package container

import (
	"context"
	"io"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/nektos/act/pkg/common"
)

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
	ServiceName    string
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
	GetHealthStatus(ctx context.Context) HealthStatus
	GetServiceName() string
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

type HealthStatus struct {
	Status        Health
	ContainerID   string
	ContainerName string
	ServiceName   string
	Image         string
	State         string
	HealthLog     string
	FailingStreak int
}

func (h Health) String() string {
	switch h {
	case HealthStarting:
		return "starting"
	case HealthHealthy:
		return "healthy"
	case HealthUnHealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

type PortCheckResult struct {
	Port       string
	Success    bool
	Error      string
	ToolAbsent bool
}

type ServiceDependencyStatus struct {
	ServiceName       string
	ServiceAlias      string
	ContainerID       string
	ContainerName     string
	Image             string
	ContainerState    string
	HasHealthCheck    bool
	HealthStatus      Health
	HealthLog         string
	FailingStreak     int
	Ports             []string
	PortCheckResults  []PortCheckResult
	WaitMethod        string
	AllPortsConnected bool
}

type ServiceWaitPort struct {
	HostPort      string
	ContainerPort string
	Protocol      string
}

type ServiceWaitPlan struct {
	ServiceName  string
	ServiceAlias string
	Timeout      time.Duration
	WaitMethod   string
	HealthCheck  struct {
		Enabled  bool
		Interval time.Duration
		Retries  int
	}
	Ports       []ServiceWaitPort
	NetworkName string
}
