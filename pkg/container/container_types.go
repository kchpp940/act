package container

import (
	"context"
	"io"

	"github.com/docker/go-connections/nat"
	"github.com/nektos/act/pkg/common"
)

// ContainerPullPolicy defines the image pull policy
type ContainerPullPolicy string

const (
	// PullAlways always pull the image
	PullAlways ContainerPullPolicy = "always"
	// PullIfNotPresent pull the image if not present locally
	PullIfNotPresent ContainerPullPolicy = "if-not-present"
	// PullNever never pull the image
	PullNever ContainerPullPolicy = "never"
)

// ContainerRemovePolicy defines the container remove policy
type ContainerRemovePolicy string

const (
	// RemoveAlways always remove the container
	RemoveAlways ContainerRemovePolicy = "always"
	// RemoveOnSuccess remove only on success
	RemoveOnSuccess ContainerRemovePolicy = "on-success"
	// RemoveNever never remove the container (reuse)
	RemoveNever ContainerRemovePolicy = "never"
)

// ContainerRuntimeSpec defines the runtime specification for all container types
type ContainerRuntimeSpec struct {
	// Image configuration
	Image     string
	Platform  string
	ForcePull bool
	PullPolicy ContainerPullPolicy

	// Build configuration (for Dockerfile-based actions)
	ForceRebuild   bool
	BuildContext   string
	DockerfilePath string

	// Network configuration
	NetworkMode    string
	NetworkAliases []string
	ExposedPorts   nat.PortSet
	PortBindings   nat.PortMap

	// Lifecycle configuration
	RemovePolicy ContainerRemovePolicy
	Reuse        bool
	AutoRemove   bool

	// Security configuration
	Privileged bool
	UsernsMode string
	CapAdd     []string
	CapDrop    []string

	// Credentials
	Username string
	Password string

	// Runtime options
	Options    string
	Entrypoint []string
	Cmd        []string
	WorkingDir string
	Env        []string
	Binds      []string
	Mounts     map[string]string
	Name       string
	Stdout     io.Writer
	Stderr     io.Writer
}

// NewContainerRuntimeSpec creates a base ContainerRuntimeSpec with common defaults
func NewContainerRuntimeSpec() *ContainerRuntimeSpec {
	return &ContainerRuntimeSpec{
		PullPolicy:   PullIfNotPresent,
		RemovePolicy: RemoveAlways,
	}
}

// ToNewContainerInput converts ContainerRuntimeSpec to NewContainerInput
func (spec *ContainerRuntimeSpec) ToNewContainerInput() *NewContainerInput {
	return &NewContainerInput{
		Image:          spec.Image,
		Username:       spec.Username,
		Password:       spec.Password,
		Entrypoint:     spec.Entrypoint,
		Cmd:            spec.Cmd,
		WorkingDir:     spec.WorkingDir,
		Env:            spec.Env,
		Binds:          spec.Binds,
		Mounts:         spec.Mounts,
		Name:           spec.Name,
		Stdout:         spec.Stdout,
		Stderr:         spec.Stderr,
		NetworkMode:    spec.NetworkMode,
		Privileged:     spec.Privileged,
		UsernsMode:     spec.UsernsMode,
		Platform:       spec.Platform,
		Options:        spec.Options,
		NetworkAliases: spec.NetworkAliases,
		ExposedPorts:   spec.ExposedPorts,
		PortBindings:   spec.PortBindings,
	}
}

// ShouldForcePull returns true if the image must be force-pulled from registry
// This maps to the forcePull parameter of Container.Pull(forcePull bool)
func (spec *ContainerRuntimeSpec) ShouldForcePull() bool {
	if spec.ForcePull {
		return true
	}
	return spec.PullPolicy == PullAlways
}

// ShouldSkipPull returns true if pulling should be skipped entirely
// When false, Pull should still be called with ShouldForcePull() as the argument
func (spec *ContainerRuntimeSpec) ShouldSkipPull() bool {
	return spec.PullPolicy == PullNever
}

// ShouldRemove determines if the container should be removed
func (spec *ContainerRuntimeSpec) ShouldRemove() bool {
	if spec.Reuse {
		return false
	}
	switch spec.RemovePolicy {
	case RemoveNever:
		return false
	case RemoveOnSuccess:
		return true
	case RemoveAlways:
		fallthrough
	default:
		return true
	}
}

// ShouldBuild determines if the image should be built
func (spec *ContainerRuntimeSpec) ShouldBuild(imageExists bool) bool {
	if spec.ForceRebuild {
		return true
	}
	return !imageExists
}

// NewContainerInput the input for the New function
type NewContainerInput struct {
	Image          string
	Username       string
	Password       string
	Entrypoint     []string
	Cmd            []string
	WorkingDir     string
	Env            []string
	Binds          []string
	Mounts         map[string]string
	Name           string
	Stdout         io.Writer
	Stderr         io.Writer
	NetworkMode    string
	Privileged     bool
	UsernsMode     string
	Platform       string
	Options        string
	NetworkAliases []string
	ExposedPorts   nat.PortSet
	PortBindings   nat.PortMap
}

// FileEntry is a file to copy to a container
type FileEntry struct {
	Name string
	Mode uint32
	Body string
}

// Container for managing docker run containers
type Container interface {
	Create(capAdd []string, capDrop []string) common.Executor
	Copy(destPath string, files ...*FileEntry) common.Executor
	CopyTarStream(ctx context.Context, destPath string, tarStream io.Reader) error
	CopyDir(destPath string, srcPath string, useGitIgnore bool) common.Executor
	GetContainerArchive(ctx context.Context, srcPath string) (io.ReadCloser, error)
	Pull(forcePull bool) common.Executor
	Start(attach bool) common.Executor
	Exec(command []string, env map[string]string, user, workdir string) common.Executor
	UpdateFromEnv(srcPath string, env *map[string]string) common.Executor
	UpdateFromImageEnv(env *map[string]string) common.Executor
	Remove() common.Executor
	Close() common.Executor
	ReplaceLogWriter(io.Writer, io.Writer) (io.Writer, io.Writer)
	GetHealth(ctx context.Context) Health
}

// NewDockerBuildExecutorInput the input for the NewDockerBuildExecutor function
type NewDockerBuildExecutorInput struct {
	ContextDir   string
	Dockerfile   string
	BuildContext io.Reader
	ImageTag     string
	Platform     string
}

// NewDockerPullExecutorInput the input for the NewDockerPullExecutor function
type NewDockerPullExecutorInput struct {
	Image     string
	ForcePull bool
	Platform  string
	Username  string
	Password  string
}

type Health int

const (
	HealthStarting Health = iota
	HealthHealthy
	HealthUnHealthy
)
