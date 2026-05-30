package container

import (
	"context"
	"fmt"
	"sort"

	"github.com/nektos/act/pkg/common"
)

type managedContainer struct {
	name        string
	container   ExecutionsEnvironment
	cleanupPlan *CleanupPlan
	reuse       bool
}

type RuntimeScope struct {
	ID           string
	Name         string
	Network      *NetworkSpec
	containers   map[string]*managedContainer
	subScopes    map[string]*RuntimeScope
	parentScope  *RuntimeScope
	isDisposed   bool
}

func NewRuntimeScope(id string, name string) *RuntimeScope {
	return &RuntimeScope{
		ID:         id,
		Name:       name,
		containers: make(map[string]*managedContainer),
		subScopes:  make(map[string]*RuntimeScope),
	}
}

func (s *RuntimeScope) WithNetwork(network *NetworkSpec) *RuntimeScope {
	s.Network = network
	if s.Network != nil {
		s.Network.CleanupPlan = &CleanupPlan{
			Owner:      CleanupOwnerScope,
			Condition:  CleanupConditionAlways,
			Order:      1000,
			Actions:    []CleanupAction{CleanupActionRemoveNetwork},
			SkipOnReuse: false,
		}
	}
	return s
}

func (s *RuntimeScope) RegisterContainer(name string, container ExecutionsEnvironment, cleanupPlan *CleanupPlan, reuse bool) {
	s.containers[name] = &managedContainer{
		name:        name,
		container:   container,
		cleanupPlan: cleanupPlan,
		reuse:       reuse,
	}
}

func (s *RuntimeScope) UnregisterContainer(name string) {
	delete(s.containers, name)
}

func (s *RuntimeScope) GetContainer(name string) ExecutionsEnvironment {
	if mc, ok := s.containers[name]; ok {
		return mc.container
	}
	return nil
}

func (s *RuntimeScope) ListContainers() []ExecutionsEnvironment {
	containers := make([]ExecutionsEnvironment, 0, len(s.containers))
	for _, mc := range s.containers {
		containers = append(containers, mc.container)
	}
	return containers
}

func (s *RuntimeScope) AddSubScope(subScope *RuntimeScope) {
	subScope.parentScope = s
	s.subScopes[subScope.ID] = subScope
}

func (s *RuntimeScope) RemoveSubScope(subScopeID string) {
	delete(s.subScopes, subScopeID)
}

func (s *RuntimeScope) CreateNetwork() common.Executor {
	return func(ctx context.Context) error {
		if s.Network == nil {
			return nil
		}
		if s.Network.Lifecycle != NetworkLifecycleCreateAndManage {
			return nil
		}
		if !s.Network.CreateIfNotExists {
			return nil
		}
		return NewDockerNetworkCreateExecutor(s.Network.Name).IfNot(common.Dryrun)(ctx)
	}
}

func (s *RuntimeScope) Setup() common.Executor {
	return s.CreateNetwork()
}

func (s *RuntimeScope) sortedCleanupItems() []*managedContainer {
	items := make([]*managedContainer, 0, len(s.containers))
	for _, mc := range s.containers {
		items = append(items, mc)
	}
	sort.Slice(items, func(i, j int) bool {
		orderI := 0
		if items[i].cleanupPlan != nil {
			orderI = items[i].cleanupPlan.Order
		}
		orderJ := 0
		if items[j].cleanupPlan != nil {
			orderJ = items[j].cleanupPlan.Order
		}
		return orderI < orderJ
	})
	return items
}

func (s *RuntimeScope) CleanupContainers(ctx context.Context) error {
	logger := common.Logger(ctx)
	jobErr := common.JobError(ctx)

	items := s.sortedCleanupItems()
	for _, mc := range items {
		if mc.cleanupPlan == nil {
			continue
		}
		if mc.cleanupPlan.Owner != CleanupOwnerScope {
			continue
		}
		if !mc.cleanupPlan.ShouldRemoveContainer(jobErr, mc.reuse) {
			continue
		}
		if mc.cleanupPlan.HasAction(CleanupActionRemoveContainer) {
			logger.Debugf("Scope '%s' cleaning up container '%s' (order=%d)", s.Name, mc.name, mc.cleanupPlan.Order)
			_ = mc.container.Remove()(ctx)
		}
	}

	s.containers = make(map[string]*managedContainer)
	return nil
}

func (s *RuntimeScope) CleanupNetwork(ctx context.Context) error {
	if s.Network == nil {
		return nil
	}
	if s.Network.CleanupPlan == nil {
		return nil
	}

	jobErr := common.JobError(ctx)
	if !s.Network.CleanupPlan.ShouldRemoveContainer(jobErr, false) {
		return nil
	}
	if !s.Network.CleanupPlan.HasAction(CleanupActionRemoveNetwork) {
		return nil
	}

	logger := common.Logger(ctx)
	logger.Debugf("Scope '%s' cleaning up network '%s' (order=%d)", s.Name, s.Network.Name, s.Network.CleanupPlan.Order)
	_ = NewDockerNetworkRemoveExecutor(s.Network.Name)(ctx)
	return nil
}

func (s *RuntimeScope) Cleanup() common.Executor {
	return func(ctx context.Context) error {
		if s.isDisposed {
			return nil
		}
		s.isDisposed = true

		logger := common.Logger(ctx)
		logger.Debugf("Starting cleanup for scope '%s'", s.Name)

		for _, subScope := range s.subScopes {
			_ = subScope.Cleanup()(ctx)
		}

		_ = s.CleanupContainers(ctx)
		_ = s.CleanupNetwork(ctx)

		logger.Debugf("Completed cleanup for scope '%s'", s.Name)
		return nil
	}
}

func (s *RuntimeScope) ContainerExecutor(spec *RuntimeSpec, factories ...ContainerFactory) *ScopedContainerExecutor {
	return &ScopedContainerExecutor{
		scope:    s,
		executor: NewContainerExecutor(spec, factories...),
	}
}

type ScopedContainerExecutor struct {
	scope    *RuntimeScope
	executor *ContainerExecutor
}

func (e *ScopedContainerExecutor) Execute() common.Executor {
	return func(ctx context.Context) error {
		spec := e.executor.spec

		if spec.ManagedByScope {
			e.scope.RegisterContainer(
				spec.Container.Name,
				e.executor.Environment(),
				spec.CleanupPlan,
				spec.ReuseContainer,
			)
		}

		err := e.executor.Execute()(ctx)

		if err != nil && spec.ManagedByScope {
			e.scope.UnregisterContainer(spec.Container.Name)
		}

		return err
	}
}

func (e *ScopedContainerExecutor) Container() Container {
	return e.executor.Container()
}

func (e *ScopedContainerExecutor) Environment() ExecutionsEnvironment {
	return e.executor.Environment()
}

func (e *ScopedContainerExecutor) CleanupSelf() common.Executor {
	return func(ctx context.Context) error {
		spec := e.executor.spec
		container := e.executor.Container()

		if spec.ManagedByScope {
			e.scope.UnregisterContainer(spec.Container.Name)
		}

		if !spec.ReuseContainer {
			return container.Remove()(ctx)
		}
		return nil
	}
}

type RuntimeScopeManager struct {
	scopes map[string]*RuntimeScope
}

func NewRuntimeScopeManager() *RuntimeScopeManager {
	return &RuntimeScopeManager{
		scopes: make(map[string]*RuntimeScope),
	}
}

func (m *RuntimeScopeManager) CreateScope(id string, name string) *RuntimeScope {
	scope := NewRuntimeScope(id, name)
	m.scopes[id] = scope
	return scope
}

func (m *RuntimeScopeManager) GetScope(id string) *RuntimeScope {
	return m.scopes[id]
}

func (m *RuntimeScopeManager) RemoveScope(id string) {
	delete(m.scopes, id)
}

func (m *RuntimeScopeManager) CleanupAll(ctx context.Context) error {
	for _, scope := range m.scopes {
		_ = scope.Cleanup()(ctx)
	}
	m.scopes = make(map[string]*RuntimeScope)
	return nil
}

const (
	ScopeIDJob     = "job"
	ScopeIDService = "service"
	ScopeIDAction  = "action"
	ScopeIDStep    = "step"
)

func JobScopeID(jobName string) string {
	return fmt.Sprintf("%s:%s", ScopeIDJob, jobName)
}

func ServiceScopeID(jobName, serviceID string) string {
	return fmt.Sprintf("%s:%s:%s", ScopeIDService, jobName, serviceID)
}
