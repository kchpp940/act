package container

import (
	"io"

	"github.com/docker/go-connections/nat"
	"github.com/nektos/act/pkg/common"
)

type ImagePullPolicy string

const (
	ImagePullPolicyAlways       ImagePullPolicy = "always"
	ImagePullPolicyIfNotPresent ImagePullPolicy = "if_not_present"
	ImagePullPolicyNever        ImagePullPolicy = "never"
)

type ImageBuildPolicy string

const (
	ImageBuildPolicyAlways    ImageBuildPolicy = "always"
	ImageBuildPolicyIfMissing ImageBuildPolicy = "if_missing"
	ImageBuildPolicyNever     ImageBuildPolicy = "never"
)

type CleanupOwner string

const (
	CleanupOwnerSelf  CleanupOwner = "self"
	CleanupOwnerScope CleanupOwner = "scope"
)

type CleanupCondition string

const (
	CleanupConditionAlways    CleanupCondition = "always"
	CleanupConditionOnSuccess CleanupCondition = "on_success"
	CleanupConditionNever     CleanupCondition = "never"
)

type CleanupAction string

const (
	CleanupActionRemoveContainer CleanupAction = "remove_container"
	CleanupActionRemoveVolumes   CleanupAction = "remove_volumes"
	CleanupActionCloseClient     CleanupAction = "close_client"
	CleanupActionRemoveNetwork   CleanupAction = "remove_network"
)

type CleanupPlan struct {
	Owner      CleanupOwner
	Condition  CleanupCondition
	Order      int
	Actions    []CleanupAction
	SkipOnReuse bool
}

func NewCleanupPlanSelf(condition CleanupCondition, order int) *CleanupPlan {
	return &CleanupPlan{
		Owner:      CleanupOwnerSelf,
		Condition:  condition,
		Order:      order,
		Actions:    []CleanupAction{CleanupActionRemoveContainer, CleanupActionCloseClient},
		SkipOnReuse: true,
	}
}

func NewCleanupPlanScope(condition CleanupCondition, order int) *CleanupPlan {
	return &CleanupPlan{
		Owner:      CleanupOwnerScope,
		Condition:  condition,
		Order:      order,
		Actions:    []CleanupAction{CleanupActionRemoveContainer},
		SkipOnReuse: false,
	}
}

func (p *CleanupPlan) ShouldRemoveContainer(jobErr error, reuse bool) bool {
	if p.SkipOnReuse && reuse {
		return false
	}
	switch p.Condition {
	case CleanupConditionAlways:
		return true
	case CleanupConditionOnSuccess:
		return jobErr == nil
	case CleanupConditionNever:
		return false
	}
	return false
}

func (p *CleanupPlan) HasAction(action CleanupAction) bool {
	for _, a := range p.Actions {
		if a == action {
			return true
		}
	}
	return false
}

type NetworkLifecycle string

const (
	NetworkLifecycleCreateAndManage NetworkLifecycle = "create_and_manage"
	NetworkLifecycleUseExisting      NetworkLifecycle = "use_existing"
	NetworkLifecycleNone             NetworkLifecycle = "none"
)

const (
	NetworkModeHost    string = "host"
	NetworkModeNone    string = "none"
	NetworkModeDefault string = "default"
)

type ContainerLifecycle struct {
	Create    bool
	Start     bool
	Attach    bool
	Wait      bool
	Exec      bool
	CopyFiles []*FileEntry
}

type ImageSpec struct {
	Ref                 string
	BuildContext        *ImageBuildSpec
	PullPolicy          ImagePullPolicy
	BuildPolicy         ImageBuildPolicy
	ForcePull           bool
	ForceRebuild        bool
	Username            string
	Password            string
	Platform            string
}

type ImageBuildSpec struct {
	ContextDir   string
	Dockerfile   string
	BuildContext io.Reader
}

type NetworkSpec struct {
	Name                string
	Aliases             []string
	Mode                string
	Lifecycle           NetworkLifecycle
	CreateIfNotExists   bool
	CleanupPlan         *CleanupPlan
}

type RuntimeSpec struct {
	ScopeID          string
	ManagedByScope   bool
	Container        *NewContainerInput
	Image            *ImageSpec
	Network          *NetworkSpec
	Lifecycle        *ContainerLifecycle
	CleanupPlan      *CleanupPlan
	ReuseContainer   bool
	CapAdd           []string
	CapDrop          []string
}

func NewRuntimeSpec() *RuntimeSpec {
	return &RuntimeSpec{
		Container: &NewContainerInput{
			Mounts:       make(map[string]string),
			Env:          make([]string, 0),
			Binds:        make([]string, 0),
			ExposedPorts: make(nat.PortSet),
			PortBindings: make(nat.PortMap),
		},
		Image: &ImageSpec{
			PullPolicy:   ImagePullPolicyIfNotPresent,
			BuildPolicy:  ImageBuildPolicyIfMissing,
		},
		Network: &NetworkSpec{
			Lifecycle:         NetworkLifecycleUseExisting,
			CreateIfNotExists: false,
		},
		Lifecycle: &ContainerLifecycle{
			Create:    true,
			Start:     true,
			Attach:    false,
			Wait:      false,
			Exec:      false,
			CopyFiles: make([]*FileEntry, 0),
		},
		CleanupPlan: NewCleanupPlanSelf(CleanupConditionAlways, 300),
	}
}

func (s *RuntimeSpec) WithContainerSpec(fn func(*NewContainerInput)) *RuntimeSpec {
	fn(s.Container)
	return s
}

func (s *RuntimeSpec) WithImageSpec(fn func(*ImageSpec)) *RuntimeSpec {
	fn(s.Image)
	return s
}

func (s *RuntimeSpec) WithNetworkSpec(fn func(*NetworkSpec)) *RuntimeSpec {
	fn(s.Network)
	return s
}

func (s *RuntimeSpec) WithLifecycle(fn func(*ContainerLifecycle)) *RuntimeSpec {
	fn(s.Lifecycle)
	return s
}

func (s *RuntimeSpec) WithCleanupPlan(plan *CleanupPlan) *RuntimeSpec {
	s.CleanupPlan = plan
	return s
}

func (s *RuntimeSpec) BuildExecutor() common.Executor {
	return NewContainerExecutor(s).Execute()
}

func NewJobRuntimeSpec() *RuntimeSpec {
	spec := NewRuntimeSpec()
	spec.Container.Entrypoint = []string{"tail", "-f", "/dev/null"}
	spec.Lifecycle.Attach = false
	spec.Lifecycle.Wait = false
	spec.CleanupPlan = NewCleanupPlanSelf(CleanupConditionOnSuccess, 200)
	spec.ReuseContainer = false
	return spec
}

func NewServiceRuntimeSpec() *RuntimeSpec {
	spec := NewRuntimeSpec()
	spec.Lifecycle.Attach = false
	spec.Lifecycle.Wait = false
	spec.CleanupPlan = NewCleanupPlanScope(CleanupConditionAlways, 100)
	spec.ReuseContainer = false
	return spec
}

func NewActionRuntimeSpec() *RuntimeSpec {
	spec := NewRuntimeSpec()
	spec.Lifecycle.Attach = true
	spec.Lifecycle.Wait = true
	spec.CleanupPlan = NewCleanupPlanSelf(CleanupConditionAlways, 300)
	spec.ReuseContainer = false
	return spec
}

func NewStepRuntimeSpec() *RuntimeSpec {
	spec := NewRuntimeSpec()
	spec.Lifecycle.Attach = true
	spec.Lifecycle.Wait = true
	spec.CleanupPlan = NewCleanupPlanSelf(CleanupConditionAlways, 300)
	spec.ReuseContainer = false
	return spec
}
