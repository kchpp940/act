package runner

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/container"
	"github.com/nektos/act/pkg/exprparser"
	"github.com/nektos/act/pkg/model"
	"github.com/opencontainers/selinux/go-selinux"
)

// RunContext contains info about current job
type RunContext struct {
	Name                string
	Config              *Config
	Matrix              map[string]interface{}
	Run                 *model.Run
	EventJSON           string
	Env                 map[string]string
	GlobalEnv           map[string]string // to pass env changes of GITHUB_ENV and set-env correctly, due to dirty Env field
	ExtraPath           []string
	CurrentStep         string
	StepResults         map[string]*model.StepResult
	IntraActionState    map[string]map[string]string
	ExprEval            ExpressionEvaluator
	JobContainer        container.ExecutionsEnvironment
	ServiceContainers   []container.ExecutionsEnvironment
	OutputMappings      map[MappableOutput]MappableOutput
	JobName             string
	ActionPath          string
	Parent              *RunContext
	Masks               []string
	cleanUpJobContainer common.Executor
	caller              *caller // job calling this RunContext (reusable workflows)
	Cancelled           bool
	nodeToolFullPath    string
}

func (rc *RunContext) AddMask(mask string) {
	rc.Masks = append(rc.Masks, mask)
}

type MappableOutput struct {
	StepID     string
	OutputName string
}

func (rc *RunContext) String() string {
	name := fmt.Sprintf("%s/%s", rc.Run.Workflow.Name, rc.Name)
	if rc.caller != nil {
		// prefix the reusable workflow with the caller job
		// this is required to create unique container names
		name = fmt.Sprintf("%s/%s", rc.caller.runContext.Name, name)
	}
	return name
}

// GetEnv returns the env for the context
func (rc *RunContext) GetEnv() map[string]string {
	if rc.Env == nil {
		rc.Env = map[string]string{}
		if rc.Run != nil && rc.Run.Workflow != nil && rc.Config != nil {
			job := rc.Run.Job()
			if job != nil {
				rc.Env = mergeMaps(rc.Run.Workflow.Env, job.Environment(), rc.Config.Env)
			}
		}
	}
	rc.Env["ACT"] = "true"
	return rc.Env
}

func (rc *RunContext) jobContainerName() string {
	return createContainerName("act", rc.String())
}

// networkName return the name of the network which will be created by `act` automatically for job,
// only create network if using a service container
func (rc *RunContext) networkName() (string, bool) {
	if len(rc.Run.Job().Services) > 0 {
		return fmt.Sprintf("%s-%s-network", rc.jobContainerName(), rc.Run.JobID), true
	}
	if rc.Config.ContainerNetworkMode == "" {
		return "host", false
	}
	return string(rc.Config.ContainerNetworkMode), false
}

func getDockerDaemonSocketMountPath(daemonPath string) string {
	if protoIndex := strings.Index(daemonPath, "://"); protoIndex != -1 {
		scheme := daemonPath[:protoIndex]
		if strings.EqualFold(scheme, "npipe") {
			// linux container mount on windows, use the default socket path of the VM / wsl2
			return "/var/run/docker.sock"
		} else if strings.EqualFold(scheme, "unix") {
			return daemonPath[protoIndex+3:]
		} else if strings.IndexFunc(scheme, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
		}) == -1 {
			// unknown protocol use default
			return "/var/run/docker.sock"
		}
	}
	return daemonPath
}

// Returns the binds and mounts for the container, resolving paths as appropriate
func (rc *RunContext) GetBindsAndMounts() ([]string, map[string]string) {
	name := rc.jobContainerName()

	if rc.Config.ContainerDaemonSocket == "" {
		rc.Config.ContainerDaemonSocket = "/var/run/docker.sock"
	}

	binds := []string{}
	if rc.Config.ContainerDaemonSocket != "-" {
		daemonPath := getDockerDaemonSocketMountPath(rc.Config.ContainerDaemonSocket)
		binds = append(binds, fmt.Sprintf("%s:%s", daemonPath, "/var/run/docker.sock"))
	}

	ext := container.LinuxContainerEnvironmentExtensions{}

	if hostEnv, ok := rc.JobContainer.(*container.HostEnvironment); ok {
		mounts := map[string]string{}
		// Permission issues?
		// binds = append(binds, hostEnv.ToolCache+":/opt/hostedtoolcache")
		binds = append(binds, hostEnv.GetActPath()+":"+ext.GetActPath())
		binds = append(binds, hostEnv.ToContainerPath(rc.Config.Workdir)+":"+ext.ToContainerPath(rc.Config.Workdir))
		return binds, mounts
	}
	mounts := map[string]string{
		"act-toolcache": "/opt/hostedtoolcache",
		name + "-env":   ext.GetActPath(),
	}

	if job := rc.Run.Job(); job != nil {
		if container := job.Container(); container != nil {
			for _, v := range container.Volumes {
				if !strings.Contains(v, ":") || filepath.IsAbs(v) {
					// Bind anonymous volume or host file.
					binds = append(binds, v)
				} else {
					// Mount existing volume.
					paths := strings.SplitN(v, ":", 2)
					mounts[paths[0]] = paths[1]
				}
			}
		}
	}

	if rc.Config.BindWorkdir {
		bindModifiers := ""
		if runtime.GOOS == "darwin" {
			bindModifiers = ":delegated"
		}
		if selinux.GetEnabled() {
			bindModifiers = ":z"
		}
		binds = append(binds, fmt.Sprintf("%s:%s%s", rc.Config.Workdir, ext.ToContainerPath(rc.Config.Workdir), bindModifiers))
	} else {
		mounts[name] = ext.ToContainerPath(rc.Config.Workdir)
	}

	return binds, mounts
}

func (rc *RunContext) startHostEnvironment() common.Executor {
	return func(ctx context.Context) error {
		logger := common.Logger(ctx)
		rawLogger := logger.WithField("raw_output", true)
		logWriter := common.NewLineWriter(rc.commandHandler(ctx), func(s string) bool {
			if rc.Config.LogOutput {
				rawLogger.Infof("%s", s)
			} else {
				rawLogger.Debugf("%s", s)
			}
			return true
		})
		cacheDir := rc.ActionCacheDir()
		randBytes := make([]byte, 8)
		_, _ = rand.Read(randBytes)
		miscpath := filepath.Join(cacheDir, hex.EncodeToString(randBytes))
		actPath := filepath.Join(miscpath, "act")
		if err := os.MkdirAll(actPath, 0o777); err != nil {
			return err
		}
		path := filepath.Join(miscpath, "hostexecutor")
		if err := os.MkdirAll(path, 0o777); err != nil {
			return err
		}
		runnerTmp := filepath.Join(miscpath, "tmp")
		if err := os.MkdirAll(runnerTmp, 0o777); err != nil {
			return err
		}
		toolCache := filepath.Join(cacheDir, "tool_cache")
		rc.JobContainer = &container.HostEnvironment{
			Path:      path,
			TmpDir:    runnerTmp,
			ToolCache: toolCache,
			Workdir:   rc.Config.Workdir,
			ActPath:   actPath,
			CleanUp: func() {
				os.RemoveAll(miscpath)
			},
			StdOut: logWriter,
		}
		rc.cleanUpJobContainer = rc.JobContainer.Remove()
		for k, v := range rc.JobContainer.GetRunnerContext(ctx) {
			if v, ok := v.(string); ok {
				rc.Env[fmt.Sprintf("RUNNER_%s", strings.ToUpper(k))] = v
			}
		}
		for _, env := range os.Environ() {
			if k, v, ok := strings.Cut(env, "="); ok {
				// don't override
				if _, ok := rc.Env[k]; !ok {
					rc.Env[k] = v
				}
			}
		}

		return common.NewPipelineExecutor(
			rc.JobContainer.Copy(rc.JobContainer.GetActPath()+"/", &container.FileEntry{
				Name: "workflow/event.json",
				Mode: 0o644,
				Body: rc.EventJSON,
			}, &container.FileEntry{
				Name: "workflow/envs.txt",
				Mode: 0o666,
				Body: "",
			}),
		)(ctx)
	}
}

func (rc *RunContext) startJobContainer() common.Executor {
	return func(ctx context.Context) error {
		logger := common.Logger(ctx)
		image := rc.platformImage(ctx)
		rawLogger := logger.WithField("raw_output", true)
		logWriter := common.NewLineWriter(rc.commandHandler(ctx), func(s string) bool {
			if rc.Config.LogOutput {
				rawLogger.Infof("%s", s)
			} else {
				rawLogger.Debugf("%s", s)
			}
			return true
		})

		username, password, err := rc.handleCredentials(ctx)
		if err != nil {
			return fmt.Errorf("failed to handle credentials: %s", err)
		}

		logger.Infof("\U0001f680  Start image=%s", image)
		name := rc.jobContainerName()

		envList := make([]string, 0)

		envList = append(envList, fmt.Sprintf("%s=%s", "RUNNER_TOOL_CACHE", "/opt/hostedtoolcache"))
		envList = append(envList, fmt.Sprintf("%s=%s", "RUNNER_OS", "Linux"))
		envList = append(envList, fmt.Sprintf("%s=%s", "RUNNER_ARCH", container.RunnerArch(ctx)))
		envList = append(envList, fmt.Sprintf("%s=%s", "RUNNER_TEMP", "/tmp"))
		envList = append(envList, fmt.Sprintf("%s=%s", "LANG", "C.UTF-8")) // Use same locale as GitHub Actions

		ext := container.LinuxContainerEnvironmentExtensions{}
		binds, mounts := rc.GetBindsAndMounts()

		// specify the network to which the container will connect when `docker create` stage. (like execute command line: docker create --network <networkName> <image>)
		// if using service containers, will create a new network for the containers.
		// and it will be removed after at last.
		networkName, createAndDeleteNetwork := rc.networkName()

		// add service containers
		for serviceID, spec := range rc.Run.Job().Services {
			// interpolate env
			interpolatedEnvs := make(map[string]string, len(spec.Env))
			for k, v := range spec.Env {
				interpolatedEnvs[k] = rc.ExprEval.Interpolate(ctx, v)
			}
			envs := make([]string, 0, len(interpolatedEnvs))
			for k, v := range interpolatedEnvs {
				envs = append(envs, fmt.Sprintf("%s=%s", k, v))
			}
			username, password, err = rc.handleServiceCredentials(ctx, spec.Credentials)
			if err != nil {
				return fmt.Errorf("failed to handle service %s credentials: %w", serviceID, err)
			}

			interpolatedVolumes := make([]string, 0, len(spec.Volumes))
			for _, volume := range spec.Volumes {
				interpolatedVolumes = append(interpolatedVolumes, rc.ExprEval.Interpolate(ctx, volume))
			}
			serviceBinds, serviceMounts := rc.GetServiceBindsAndMounts(interpolatedVolumes)

			interpolatedPorts := make([]string, 0, len(spec.Ports))
			for _, port := range spec.Ports {
				interpolatedPorts = append(interpolatedPorts, rc.ExprEval.Interpolate(ctx, port))
			}
			exposedPorts, portBindings, err := nat.ParsePortSpecs(interpolatedPorts)
			if err != nil {
				return fmt.Errorf("failed to parse service %s ports: %w", serviceID, err)
			}

			imageName := rc.ExprEval.Interpolate(ctx, spec.Image)
			if imageName == "" {
				logger.Infof("The service '%s' will not be started because the container definition has an empty image.", serviceID)
				continue
			}

			serviceContainerName := createContainerName(rc.jobContainerName(), serviceID)
			c := container.NewContainer(&container.NewContainerInput{
				Name:           serviceContainerName,
				WorkingDir:     ext.ToContainerPath(rc.Config.Workdir),
				Image:          imageName,
				Username:       username,
				Password:       password,
				Env:            envs,
				Mounts:         serviceMounts,
				Binds:          serviceBinds,
				Stdout:         logWriter,
				Stderr:         logWriter,
				Privileged:     rc.Config.Privileged,
				UsernsMode:     rc.Config.UsernsMode,
				Platform:       rc.Config.ContainerArchitecture,
				Options:        rc.ExprEval.Interpolate(ctx, spec.Options),
				NetworkMode:    networkName,
				NetworkAliases: []string{serviceID},
				ExposedPorts:   exposedPorts,
				PortBindings:   portBindings,
				ServiceName:    serviceID,
			})
			rc.ServiceContainers = append(rc.ServiceContainers, c)
		}

		rc.cleanUpJobContainer = func(ctx context.Context) error {
			reuseJobContainer := func(_ context.Context) bool {
				return rc.Config.ReuseContainers
			}

			if rc.JobContainer != nil {
				return rc.JobContainer.Remove().IfNot(reuseJobContainer).
					Then(container.NewDockerVolumeRemoveExecutor(rc.jobContainerName(), false)).IfNot(reuseJobContainer).
					Then(container.NewDockerVolumeRemoveExecutor(rc.jobContainerName()+"-env", false)).IfNot(reuseJobContainer).
					Then(func(ctx context.Context) error {
						if len(rc.ServiceContainers) > 0 {
							logger.Infof("Cleaning up services for job %s", rc.JobName)
							if err := rc.stopServiceContainers()(ctx); err != nil {
								logger.Errorf("Error while cleaning services: %v", err)
							}
							if createAndDeleteNetwork {
								// clean network if it has been created by act
								// if using service containers
								// it means that the network to which containers are connecting is created by `act_runner`,
								// so, we should remove the network at last.
								logger.Infof("Cleaning up network for job %s, and network name is: %s", rc.JobName, networkName)
								if err := container.NewDockerNetworkRemoveExecutor(networkName)(ctx); err != nil {
									logger.Errorf("Error while cleaning network: %v", err)
								}
							}
						}
						return nil
					})(ctx)
			}
			return nil
		}

		jobContainerNetwork := rc.Config.ContainerNetworkMode.NetworkName()
		if rc.containerImage(ctx) != "" {
			jobContainerNetwork = networkName
		} else if jobContainerNetwork == "" {
			jobContainerNetwork = "host"
		}

		rc.JobContainer = container.NewContainer(&container.NewContainerInput{
			Cmd:            nil,
			Entrypoint:     []string{"tail", "-f", "/dev/null"},
			WorkingDir:     ext.ToContainerPath(rc.Config.Workdir),
			Image:          image,
			Username:       username,
			Password:       password,
			Name:           name,
			Env:            envList,
			Mounts:         mounts,
			NetworkMode:    jobContainerNetwork,
			NetworkAliases: []string{rc.Name},
			Binds:          binds,
			Stdout:         logWriter,
			Stderr:         logWriter,
			Privileged:     rc.Config.Privileged,
			UsernsMode:     rc.Config.UsernsMode,
			Platform:       rc.Config.ContainerArchitecture,
			Options:        rc.options(ctx),
		})
		if rc.JobContainer == nil {
			return errors.New("Failed to create job container")
		}

		return common.NewPipelineExecutor(
			rc.pullServicesImages(rc.Config.ForcePull),
			rc.JobContainer.Pull(rc.Config.ForcePull),
			rc.stopJobContainer(),
			container.NewDockerNetworkCreateExecutor(networkName).IfBool(createAndDeleteNetwork),
			rc.startServiceContainers(networkName),
			rc.JobContainer.Create(rc.Config.ContainerCapAdd, rc.Config.ContainerCapDrop),
			rc.JobContainer.Start(false),
			rc.JobContainer.Copy(rc.JobContainer.GetActPath()+"/", &container.FileEntry{
				Name: "workflow/event.json",
				Mode: 0o644,
				Body: rc.EventJSON,
			}, &container.FileEntry{
				Name: "workflow/envs.txt",
				Mode: 0o666,
				Body: "",
			}),
			rc.waitForServiceContainers(),
		)(ctx)
	}
}

func (rc *RunContext) execJobContainer(cmd []string, env map[string]string, user, workdir string) common.Executor {
	return func(ctx context.Context) error {
		return rc.JobContainer.Exec(cmd, env, user, workdir)(ctx)
	}
}

func (rc *RunContext) InitializeNodeTool() common.Executor {
	return func(ctx context.Context) error {
		ctx, cancel := common.EarlyCancelContext(ctx)
		defer cancel()
		rc.GetNodeToolFullPath(ctx)
		return nil
	}
}

func (rc *RunContext) GetNodeToolFullPath(ctx context.Context) string {
	if rc.nodeToolFullPath == "" {
		timeed, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		path := rc.JobContainer.GetPathVariableName()
		cenv := map[string]string{}
		var cpath string
		if err := rc.JobContainer.UpdateFromImageEnv(&cenv)(ctx); err == nil {
			if p, ok := cenv[path]; ok {
				cpath = p
			}
		}
		if len(cpath) == 0 {
			cpath = rc.JobContainer.DefaultPathVariable()
		}
		cenv[path] = cpath
		hout := &bytes.Buffer{}
		herr := &bytes.Buffer{}
		stdout, stderr := rc.JobContainer.ReplaceLogWriter(hout, herr)
		err := rc.execJobContainer([]string{"node", "--no-warnings", "-e", "console.log(process.execPath)"},
			cenv, "", "").
			Finally(func(context.Context) error {
				rc.JobContainer.ReplaceLogWriter(stdout, stderr)
				return nil
			})(timeed)
		rawStr := strings.Trim(hout.String(), "\r\n")
		if err == nil && !strings.ContainsAny(rawStr, "\r\n") {
			rc.nodeToolFullPath = rawStr
		} else {
			rc.nodeToolFullPath = "node"
		}
	}
	return rc.nodeToolFullPath
}

func (rc *RunContext) ApplyExtraPath(ctx context.Context, env *map[string]string) {
	if len(rc.ExtraPath) > 0 {
		path := rc.JobContainer.GetPathVariableName()
		if rc.JobContainer.IsEnvironmentCaseInsensitive() {
			// On windows system Path and PATH could also be in the map
			for k := range *env {
				if strings.EqualFold(path, k) {
					path = k
					break
				}
			}
		}
		if (*env)[path] == "" {
			cenv := map[string]string{}
			var cpath string
			if err := rc.JobContainer.UpdateFromImageEnv(&cenv)(ctx); err == nil {
				if p, ok := cenv[path]; ok {
					cpath = p
				}
			}
			if len(cpath) == 0 {
				cpath = rc.JobContainer.DefaultPathVariable()
			}
			(*env)[path] = cpath
		}
		(*env)[path] = rc.JobContainer.JoinPathVariable(append(rc.ExtraPath, (*env)[path])...)
	}
}

func (rc *RunContext) UpdateExtraPath(ctx context.Context, githubEnvPath string) error {
	if common.Dryrun(ctx) {
		return nil
	}
	pathTar, err := rc.JobContainer.GetContainerArchive(ctx, githubEnvPath)
	if err != nil {
		return err
	}
	defer pathTar.Close()

	reader := tar.NewReader(pathTar)
	_, err = reader.Next()
	if err != nil && err != io.EOF {
		return err
	}
	s := bufio.NewScanner(reader)
	s.Buffer(nil, 1024*1024*1024) // increase buffer to 1GB to avoid scanner buffer overflow
	firstLine := true
	for s.Scan() {
		line := s.Text()
		if firstLine {
			firstLine = false
			// skip utf8 bom, powershell 5 legacy uses it for utf8
			if len(line) >= 3 && line[0] == 239 && line[1] == 187 && line[2] == 191 {
				line = line[3:]
			}
		}
		if len(line) > 0 {
			rc.addPath(ctx, line)
		}
	}
	return s.Err()
}

// stopJobContainer removes the job container (if it exists) and its volume (if it exists)
func (rc *RunContext) stopJobContainer() common.Executor {
	return func(ctx context.Context) error {
		if rc.cleanUpJobContainer != nil {
			return rc.cleanUpJobContainer(ctx)
		}
		return nil
	}
}

func (rc *RunContext) pullServicesImages(forcePull bool) common.Executor {
	return func(ctx context.Context) error {
		execs := []common.Executor{}
		for _, c := range rc.ServiceContainers {
			execs = append(execs, c.Pull(forcePull))
		}
		return common.NewParallelExecutor(len(execs), execs...)(ctx)
	}
}

func (rc *RunContext) startServiceContainers(_ string) common.Executor {
	return func(ctx context.Context) error {
		execs := []common.Executor{}
		for _, c := range rc.ServiceContainers {
			execs = append(execs, common.NewPipelineExecutor(
				c.Pull(false),
				c.Create(rc.Config.ContainerCapAdd, rc.Config.ContainerCapDrop),
				c.Start(false),
			))
		}
		return common.NewParallelExecutor(len(execs), execs...)(ctx)
	}
}

type healthCheckConfig struct {
	timeout  time.Duration
	interval time.Duration
	retries  int
	hasCmd   bool
}

func parseHealthCheckOptions(options string) healthCheckConfig {
	config := healthCheckConfig{
		timeout:  5 * time.Minute,
		interval: 0,
		retries:  0,
		hasCmd:   false,
	}

	if options == "" {
		return config
	}

	parts := strings.Fields(options)
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		switch {
		case strings.HasPrefix(part, "--health-cmd="):
			config.hasCmd = true
		case part == "--health-cmd" && i+1 < len(parts):
			config.hasCmd = true
			i++
		case strings.HasPrefix(part, "--health-timeout="):
			if d, err := time.ParseDuration(strings.TrimPrefix(part, "--health-timeout=")); err == nil {
				config.timeout = d * 10
			}
		case part == "--health-timeout" && i+1 < len(parts):
			if d, err := time.ParseDuration(parts[i+1]); err == nil {
				config.timeout = d * 10
				i++
			}
		case strings.HasPrefix(part, "--health-interval="):
			if d, err := time.ParseDuration(strings.TrimPrefix(part, "--health-interval=")); err == nil {
				config.interval = d
			}
		case part == "--health-interval" && i+1 < len(parts):
			if d, err := time.ParseDuration(parts[i+1]); err == nil {
				config.interval = d
				i++
			}
		case strings.HasPrefix(part, "--health-retries="):
			if r, err := strconv.Atoi(strings.TrimPrefix(part, "--health-retries=")); err == nil {
				config.retries = r
			}
		case part == "--health-retries" && i+1 < len(parts):
			if r, err := strconv.Atoi(parts[i+1]); err == nil {
				config.retries = r
				i++
			}
		}
	}

	if config.retries > 0 && config.interval > 0 {
		config.timeout = time.Duration(config.retries+2) * config.interval
	}

	return config
}

func buildServiceWaitPlan(ctx context.Context, rc *RunContext, serviceName string) container.ServiceWaitPlan {
	plan := container.ServiceWaitPlan{
		ServiceName:  serviceName,
		ServiceAlias: serviceName,
		WaitMethod:   "none",
		Timeout:      10 * time.Second,
	}

	for serviceID, spec := range rc.Run.Job().Services {
		if serviceID == serviceName {
			interpolatedOptions := rc.ExprEval.Interpolate(ctx, spec.Options)
			hcConfig := parseHealthCheckOptions(interpolatedOptions)
			plan.NetworkName = rc.networkNameForService(serviceID)

			plan.HealthCheck.Enabled = hcConfig.hasCmd
			plan.HealthCheck.Interval = hcConfig.interval
			plan.HealthCheck.Retries = hcConfig.retries
			plan.Timeout = hcConfig.timeout

			interpolatedPorts := make([]string, 0, len(spec.Ports))
			for _, port := range spec.Ports {
				interpolatedPorts = append(interpolatedPorts, rc.ExprEval.Interpolate(ctx, port))
			}

			for _, portSpec := range interpolatedPorts {
				parts := strings.Split(portSpec, ":")
				var portAndProto string
				var hostPort string

				if len(parts) >= 3 {
					hostPort = parts[0]
					portAndProto = parts[2]
				} else if len(parts) == 2 {
					hostPort = parts[0]
					portAndProto = parts[1]
				} else {
					portAndProto = parts[0]
				}

				portParts := strings.Split(portAndProto, "/")
				containerPort := portParts[0]
				protocol := "tcp"
				if len(portParts) > 1 {
					protocol = portParts[1]
				}

				waitPort := container.ServiceWaitPort{
					ContainerPort: containerPort,
					Protocol:      protocol,
					HostPort:      hostPort,
				}
				plan.Ports = append(plan.Ports, waitPort)
			}

			if plan.HealthCheck.Enabled {
				plan.WaitMethod = "healthcheck"
			} else if len(plan.Ports) > 0 {
				plan.WaitMethod = "port-check"
				if plan.Timeout == 5*time.Minute {
					plan.Timeout = 2 * time.Minute
				}
			}

			break
		}
	}

	return plan
}

func (rc *RunContext) networkNameForService(serviceID string) string {
	networkName, _ := rc.networkName()
	return networkName
}

type serviceWaitStatus struct {
	ServiceName       string
	ServiceAlias      string
	ContainerID       string
	ContainerName     string
	Image             string
	ContainerState    string
	NetworkName       string
	WaitMethod        string
	HasHealthCheck    bool
	HealthStatus      container.Health
	HealthLog         string
	FailingStreak     int
	Ports             []container.ServiceWaitPort
	PortCheckResults  []container.PortCheckResult
	AllPortsConnected bool
}

func formatServiceWaitError(status serviceWaitStatus, timeout time.Duration) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Service '%s' failed to become ready", status.ServiceName))
	sb.WriteString(fmt.Sprintf("\n  Service Alias: %s", status.ServiceAlias))
	sb.WriteString(fmt.Sprintf("\n  Network: %s", status.NetworkName))
	sb.WriteString(fmt.Sprintf("\n  Wait Method: %s", status.WaitMethod))
	sb.WriteString(fmt.Sprintf("\n  Container ID: %s", status.ContainerID))
	sb.WriteString(fmt.Sprintf("\n  Container Name: %s", status.ContainerName))
	sb.WriteString(fmt.Sprintf("\n  Image: %s", status.Image))
	sb.WriteString(fmt.Sprintf("\n  Container State: %s", status.ContainerState))

	if status.WaitMethod == "healthcheck" {
		sb.WriteString(fmt.Sprintf("\n  Health Status: %s", status.HealthStatus.String()))
		if status.FailingStreak > 0 {
			sb.WriteString(fmt.Sprintf("\n  Failing Streak: %d", status.FailingStreak))
		}
		if status.HealthLog != "" {
			sb.WriteString(fmt.Sprintf("\n  Last Health Check: %s", status.HealthLog))
		}
	} else if status.WaitMethod == "port-check" {
		toolAbsent := false
		for _, r := range status.PortCheckResults {
			if r.ToolAbsent {
				toolAbsent = true
				break
			}
		}
		if toolAbsent {
			sb.WriteString("\n  Probe Result: NO CONNECTIVITY TOOL available in job container image")
			sb.WriteString("\n  Tried: nc, bash (/dev/tcp), python3, python, perl")
			sb.WriteString("\n  Suggestion: install one of the above tools in your job container image, or add a healthcheck to the service definition")
		}
		sb.WriteString(fmt.Sprintf("\n  Ports Checked: %d", len(status.Ports)))
		for i, port := range status.Ports {
			addr := fmt.Sprintf("%s:%s", status.ServiceAlias, port.ContainerPort)
			if i < len(status.PortCheckResults) {
				r := status.PortCheckResults[i]
				if r.Success {
					sb.WriteString(fmt.Sprintf("\n    %s (containerPort: %s, protocol: %s): connected", addr, port.ContainerPort, port.Protocol))
				} else if r.ToolAbsent {
					sb.WriteString(fmt.Sprintf("\n    %s (containerPort: %s, protocol: %s): TOOL UNAVAILABLE - cannot check", addr, port.ContainerPort, port.Protocol))
				} else {
					sb.WriteString(fmt.Sprintf("\n    %s (containerPort: %s, protocol: %s): SERVICE NOT READY - %s", addr, port.ContainerPort, port.Protocol, r.Error))
				}
			} else {
				sb.WriteString(fmt.Sprintf("\n    %s (containerPort: %s, protocol: %s): not checked", addr, port.ContainerPort, port.Protocol))
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\n  Timeout: %v", timeout))
	return sb.String()
}

const serviceProbeScriptName = "service-probe.sh"

const serviceProbeScript = `#!/bin/sh
HOST="$1"
PORT="$2"
if [ -z "$HOST" ] || [ -z "$PORT" ]; then
    echo "usage: service-probe.sh HOST PORT"
    exit 2
fi

if command -v nc >/dev/null 2>&1; then
    nc -z -w 2 "$HOST" "$PORT" 2>/dev/null && exit 0 || exit 1
fi

if command -v bash >/dev/null 2>&1; then
    timeout 2 bash -c "echo >/dev/tcp/$HOST/$PORT" 2>/dev/null && exit 0 || exit 1
fi

if command -v python3 >/dev/null 2>&1; then
    python3 -c "import socket; s=socket.socket(); s.settimeout(2); s.connect(('$HOST', $PORT)); s.close()" 2>/dev/null && exit 0 || exit 1
fi

if command -v python >/dev/null 2>&1; then
    python -c "import socket; s=socket.socket(); s.settimeout(2); s.connect(('$HOST', $PORT)); s.close()" 2>/dev/null && exit 0 || exit 1
fi

if command -v perl >/dev/null 2>&1; then
    perl -e "use IO::Socket::INET; my \\\$s = IO::Socket::INET->new(PeerAddr=>'$HOST',PeerPort=>$PORT,Timeout=>2) or exit 1; close \\\$s;" 2>/dev/null && exit 0 || exit 1
fi

echo "no connectivity tool available (tried: nc, bash, python3, python, perl)"
exit 2
`

const (
	probeExitConnected   = 0
	probeExitRefused     = 1
	probeExitNoTool      = 2
	probeExitUnknown     = -1
)

func execExitCode(err error) int {
	if err == nil {
		return 0
	}
	var code int
	n, _ := fmt.Sscanf(err.Error(), "exitcode '%d'", &code)
	if n == 1 {
		return code
	}
	return probeExitUnknown
}

func (rc *RunContext) injectServiceProbe(ctx context.Context) error {
	if rc.JobContainer == nil {
		return fmt.Errorf("job container not available for probe injection")
	}
	return rc.JobContainer.Copy(rc.JobContainer.GetActPath()+"/", &container.FileEntry{
		Name: serviceProbeScriptName,
		Mode: 0o755,
		Body: serviceProbeScript,
	})(ctx)
}

func (rc *RunContext) checkServicePortFromJobContainer(ctx context.Context, serviceAlias string, port container.ServiceWaitPort) container.PortCheckResult {
	result := container.PortCheckResult{
		Port: port.ContainerPort,
	}

	probePath := rc.JobContainer.GetActPath() + "/" + serviceProbeScriptName
	err := rc.JobContainer.Exec(
		[]string{"sh", probePath, serviceAlias, port.ContainerPort},
		map[string]string{},
		"", "",
	)(ctx)

	code := execExitCode(err)
	switch code {
	case probeExitConnected:
		result.Success = true
	case probeExitNoTool:
		result.ToolAbsent = true
		result.Error = fmt.Sprintf("no connectivity tool available in job container image (tried: nc, bash, python3, python, perl); cannot check serviceAlias=%s containerPort=%s", serviceAlias, port.ContainerPort)
	case probeExitRefused:
		result.Error = fmt.Sprintf("service %s:%s not reachable (connection refused or timeout)", serviceAlias, port.ContainerPort)
	default:
		if err != nil {
			result.Error = fmt.Sprintf("probe execution failed for %s:%s: %s", serviceAlias, port.ContainerPort, err.Error())
		} else {
			result.Success = true
		}
	}

	return result
}

func (rc *RunContext) checkAllServicePortsFromJobContainer(ctx context.Context, serviceAlias string, ports []container.ServiceWaitPort) ([]container.PortCheckResult, bool) {
	results := make([]container.PortCheckResult, 0, len(ports))
	allConnected := true

	for _, port := range ports {
		result := rc.checkServicePortFromJobContainer(ctx, serviceAlias, port)
		results = append(results, result)
		if !result.Success {
			allConnected = false
		}
	}

	return results, allConnected
}

func (rc *RunContext) waitForServiceContainer(c container.ExecutionsEnvironment) common.Executor {
	return func(ctx context.Context) error {
		serviceName := c.GetServiceName()
		plan := buildServiceWaitPlan(ctx, rc, serviceName)

		sctx, cancel := context.WithTimeout(ctx, plan.Timeout)
		defer cancel()

		logger := common.Logger(ctx)

		switch plan.WaitMethod {
		case "healthcheck":
			logger.Infof("Waiting for service '%s' (alias: %s) using healthcheck (timeout: %v)...", serviceName, plan.ServiceAlias, plan.Timeout)
		case "port-check":
			portList := make([]string, 0, len(plan.Ports))
			for _, p := range plan.Ports {
				portList = append(portList, fmt.Sprintf("%s:%s", plan.ServiceAlias, p.ContainerPort))
			}
			logger.Infof("Waiting for service '%s' (alias: %s) using port connectivity check from job container (ports: %v, timeout: %v)...", serviceName, plan.ServiceAlias, portList, plan.Timeout)
		default:
			logger.Infof("Service '%s' (alias: %s) has no healthcheck or ports, waiting briefly for container to start (timeout: %v)...", serviceName, plan.ServiceAlias, plan.Timeout)
		}

		delay := time.Second

		status := serviceWaitStatus{
			ServiceName:    plan.ServiceName,
			ServiceAlias:   plan.ServiceAlias,
			NetworkName:    plan.NetworkName,
			WaitMethod:     plan.WaitMethod,
			HasHealthCheck: plan.HealthCheck.Enabled,
			Ports:          plan.Ports,
		}

		for {
			select {
			case <-sctx.Done():
				healthStatus := c.GetHealthStatus(sctx)
				status.ContainerID = healthStatus.ContainerID
				status.ContainerName = healthStatus.ContainerName
				status.Image = healthStatus.Image
				status.ContainerState = healthStatus.State
				status.HealthStatus = healthStatus.Status
				status.HealthLog = healthStatus.HealthLog
				status.FailingStreak = healthStatus.FailingStreak

				if plan.WaitMethod == "port-check" {
					status.PortCheckResults, _ = rc.checkAllServicePortsFromJobContainer(sctx, plan.ServiceAlias, plan.Ports)
				}

				return fmt.Errorf("%s\n\nTimeout reached after %v waiting for service to become ready",
					formatServiceWaitError(status, plan.Timeout), plan.Timeout)
			default:
			}

			if plan.WaitMethod == "healthcheck" {
				healthStatus := c.GetHealthStatus(sctx)
				status.ContainerID = healthStatus.ContainerID
				status.ContainerName = healthStatus.ContainerName
				status.Image = healthStatus.Image
				status.ContainerState = healthStatus.State
				status.HealthStatus = healthStatus.Status
				status.HealthLog = healthStatus.HealthLog
				status.FailingStreak = healthStatus.FailingStreak

				if healthStatus.Status == container.HealthHealthy {
					logger.Infof("Service '%s' (alias: %s) is healthy", serviceName, plan.ServiceAlias)
					return nil
				}

				if healthStatus.Status == container.HealthUnHealthy {
					return fmt.Errorf("%s\n\nService is unhealthy and will not recover",
						formatServiceWaitError(status, plan.Timeout))
				}
			} else if plan.WaitMethod == "port-check" {
				portResults, allConnected := rc.checkAllServicePortsFromJobContainer(sctx, plan.ServiceAlias, plan.Ports)
				status.PortCheckResults = portResults
				status.AllPortsConnected = allConnected

				if allConnected {
					portList := make([]string, 0, len(plan.Ports))
					for _, p := range plan.Ports {
						portList = append(portList, fmt.Sprintf("%s:%s", plan.ServiceAlias, p.ContainerPort))
					}
					logger.Infof("Service '%s' (alias: %s) is ready - all ports connected from job container: %v", serviceName, plan.ServiceAlias, portList)
					return nil
				}

				toolAbsent := false
				for _, r := range portResults {
					if r.ToolAbsent {
						toolAbsent = true
						break
					}
				}
				if toolAbsent {
					return fmt.Errorf("%s\n\nCannot verify service readiness: no connectivity tool available in job container image",
						formatServiceWaitError(status, plan.Timeout))
				}
			} else {
				time.Sleep(5 * time.Second)
				logger.Infof("Service '%s' (alias: %s) container started (no healthcheck or ports configured)", serviceName, plan.ServiceAlias)
				return nil
			}

			time.Sleep(delay)
			delay *= 2
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}
		}
	}
}

func (rc *RunContext) waitForServiceContainers() common.Executor {
	return func(ctx context.Context) error {
		if len(rc.ServiceContainers) == 0 {
			return nil
		}

		logger := common.Logger(ctx)

		needsPortCheck := false
		for _, c := range rc.ServiceContainers {
			serviceName := c.GetServiceName()
			plan := buildServiceWaitPlan(ctx, rc, serviceName)
			if plan.WaitMethod == "port-check" {
				needsPortCheck = true
				break
			}
		}

		if needsPortCheck {
			if err := rc.injectServiceProbe(ctx); err != nil {
				logger.Warnf("Failed to inject service probe script into job container: %v", err)
			} else {
				logger.Debugf("Injected service probe script into job container")
			}
		}

		logger.Infof("Waiting for %d service containers to be ready...", len(rc.ServiceContainers))

		execs := []common.Executor{}
		for _, c := range rc.ServiceContainers {
			execs = append(execs, rc.waitForServiceContainer(c))
		}
		err := common.NewParallelExecutor(len(execs), execs...)(ctx)

		if err != nil {
			logger.Errorf("One or more service containers failed to become ready: %v", err)
			return err
		}

		logger.Infof("All service containers are ready")
		return nil
	}
}

func (rc *RunContext) stopServiceContainers() common.Executor {
	return func(ctx context.Context) error {
		execs := []common.Executor{}
		for _, c := range rc.ServiceContainers {
			execs = append(execs, c.Remove().Finally(c.Close()))
		}
		return common.NewParallelExecutor(len(execs), execs...)(ctx)
	}
}

// Prepare the mounts and binds for the worker

// ActionCacheDir is for rc
func (rc *RunContext) ActionCacheDir() string {
	if rc.Config.ActionCacheDir != "" {
		return rc.Config.ActionCacheDir
	}
	var xdgCache string
	var ok bool
	if xdgCache, ok = os.LookupEnv("XDG_CACHE_HOME"); !ok || xdgCache == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdgCache = filepath.Join(home, ".cache")
		} else if xdgCache, err = filepath.Abs("."); err != nil {
			// It's almost impossible to get here, so the temp dir is a good fallback
			xdgCache = os.TempDir()
		}
	}
	return filepath.Join(xdgCache, "act")
}

// Interpolate outputs after a job is done
func (rc *RunContext) interpolateOutputs() common.Executor {
	return func(ctx context.Context) error {
		ee := rc.NewExpressionEvaluator(ctx)
		for k, v := range rc.Run.Job().Outputs {
			interpolated := ee.Interpolate(ctx, v)
			if v != interpolated {
				rc.Run.Job().Outputs[k] = interpolated
			}
		}
		return nil
	}
}

func (rc *RunContext) startContainer() common.Executor {
	return func(ctx context.Context) error {
		ctx, cancel := common.EarlyCancelContext(ctx)
		defer cancel()
		if rc.IsHostEnv(ctx) {
			return rc.startHostEnvironment()(ctx)
		}
		return rc.startJobContainer()(ctx)
	}
}

func (rc *RunContext) IsHostEnv(ctx context.Context) bool {
	platform := rc.runsOnImage(ctx)
	image := rc.containerImage(ctx)
	return image == "" && strings.EqualFold(platform, "-self-hosted")
}

func (rc *RunContext) stopContainer() common.Executor {
	return rc.stopJobContainer()
}

func (rc *RunContext) closeContainer() common.Executor {
	return func(ctx context.Context) error {
		if rc.JobContainer != nil {
			return rc.JobContainer.Close()(ctx)
		}
		return nil
	}
}

func (rc *RunContext) matrix() map[string]interface{} {
	return rc.Matrix
}

func (rc *RunContext) result(result string) {
	rc.Run.Job().Result = result
}

func (rc *RunContext) steps() []*model.Step {
	return rc.Run.Job().Steps
}

// Executor returns a pipeline executor for all the steps in the job
func (rc *RunContext) Executor() (common.Executor, error) {
	var executor common.Executor
	var jobType, err = rc.Run.Job().Type()

	switch jobType {
	case model.JobTypeDefault:
		executor = newJobExecutor(rc, &stepFactoryImpl{}, rc)
	case model.JobTypeReusableWorkflowLocal:
		executor = newLocalReusableWorkflowExecutor(rc)
	case model.JobTypeReusableWorkflowRemote:
		executor = newRemoteReusableWorkflowExecutor(rc)
	case model.JobTypeInvalid:
		return nil, err
	}

	return func(ctx context.Context) error {
		res, err := rc.isEnabled(ctx)
		if err != nil {
			return err
		}
		if res {
			return executor(ctx)
		}
		return nil
	}, nil
}

func (rc *RunContext) containerImage(ctx context.Context) string {
	job := rc.Run.Job()

	c := job.Container()
	if c != nil {
		return rc.ExprEval.Interpolate(ctx, c.Image)
	}

	return ""
}

func (rc *RunContext) runsOnImage(ctx context.Context) string {
	if rc.Run.Job().RunsOn() == nil {
		common.Logger(ctx).Errorf("'runs-on' key not defined in %s", rc.String())
	}

	for _, platformName := range rc.runsOnPlatformNames(ctx) {
		image := rc.Config.Platforms[strings.ToLower(platformName)]
		if image != "" {
			return image
		}
	}

	return ""
}

func (rc *RunContext) runsOnPlatformNames(ctx context.Context) []string {
	job := rc.Run.Job()

	if job.RunsOn() == nil {
		return []string{}
	}

	if err := rc.ExprEval.EvaluateYamlNode(ctx, &job.RawRunsOn); err != nil {
		common.Logger(ctx).Errorf("Error while evaluating runs-on: %v", err)
		return []string{}
	}

	return job.RunsOn()
}

func (rc *RunContext) platformImage(ctx context.Context) string {
	if containerImage := rc.containerImage(ctx); containerImage != "" {
		return containerImage
	}

	return rc.runsOnImage(ctx)
}

func (rc *RunContext) options(ctx context.Context) string {
	job := rc.Run.Job()
	c := job.Container()
	if c != nil {
		return rc.ExprEval.Interpolate(ctx, c.Options)
	}

	return rc.Config.ContainerOptions
}

func (rc *RunContext) isEnabled(ctx context.Context) (bool, error) {
	job := rc.Run.Job()
	l := common.Logger(ctx)
	runJob, runJobErr := EvalBool(ctx, rc.ExprEval, job.If.Value, exprparser.DefaultStatusCheckSuccess)
	jobType, jobTypeErr := job.Type()

	if runJobErr != nil {
		return false, fmt.Errorf("  \u274C  Error in if-expression: \"if: %s\" (%s)", job.If.Value, runJobErr)
	}

	if jobType == model.JobTypeInvalid {
		return false, jobTypeErr
	}

	if !runJob {
		rc.result("skipped")
		l.WithField("jobResult", "skipped").Debugf("Skipping job '%s' due to '%s'", job.Name, job.If.Value)
		return false, nil
	}

	if jobType != model.JobTypeDefault {
		return true, nil
	}

	img := rc.platformImage(ctx)
	if img == "" {
		for _, platformName := range rc.runsOnPlatformNames(ctx) {
			l.Infof("\U0001F6A7  Skipping unsupported platform -- Try running with `-P %+v=...`", platformName)
		}
		return false, nil
	}
	return true, nil
}

func mergeMaps(maps ...map[string]string) map[string]string {
	rtnMap := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			rtnMap[k] = v
		}
	}
	return rtnMap
}

func createContainerName(parts ...string) string {
	name := strings.Join(parts, "-")
	pattern := regexp.MustCompile("[^a-zA-Z0-9]")
	name = pattern.ReplaceAllString(name, "-")
	name = strings.ReplaceAll(name, "--", "-")
	hash := sha256.Sum256([]byte(name))

	// SHA256 is 64 hex characters. So trim name to 63 characters to make room for the hash and separator
	trimmedName := strings.Trim(trimToLen(name, 63), "-")

	return fmt.Sprintf("%s-%x", trimmedName, hash)
}

func trimToLen(s string, l int) string {
	if l < 0 {
		l = 0
	}
	if len(s) > l {
		return s[:l]
	}
	return s
}

func (rc *RunContext) getJobContext() *model.JobContext {
	jobStatus := "success"
	if rc.Cancelled {
		jobStatus = "cancelled"
	} else {
		for _, stepStatus := range rc.StepResults {
			if stepStatus.Conclusion == model.StepStatusFailure {
				jobStatus = "failure"
				break
			}
		}
	}
	return &model.JobContext{
		Status: jobStatus,
	}
}

func (rc *RunContext) getStepsContext() map[string]*model.StepResult {
	return rc.StepResults
}

func (rc *RunContext) getGithubContext(ctx context.Context) *model.GithubContext {
	logger := common.Logger(ctx)
	ghc := &model.GithubContext{
		Event:            make(map[string]interface{}),
		Workflow:         rc.Run.Workflow.Name,
		RunAttempt:       rc.Config.Env["GITHUB_RUN_ATTEMPT"],
		RunID:            rc.Config.Env["GITHUB_RUN_ID"],
		RunNumber:        rc.Config.Env["GITHUB_RUN_NUMBER"],
		Actor:            rc.Config.Actor,
		EventName:        rc.Config.EventName,
		Action:           rc.CurrentStep,
		Token:            rc.Config.Token,
		Job:              rc.Run.JobID,
		ActionPath:       rc.ActionPath,
		ActionRepository: rc.Env["GITHUB_ACTION_REPOSITORY"],
		ActionRef:        rc.Env["GITHUB_ACTION_REF"],
		RepositoryOwner:  rc.Config.Env["GITHUB_REPOSITORY_OWNER"],
		RetentionDays:    rc.Config.Env["GITHUB_RETENTION_DAYS"],
		RunnerPerflog:    rc.Config.Env["RUNNER_PERFLOG"],
		RunnerTrackingID: rc.Config.Env["RUNNER_TRACKING_ID"],
		Repository:       rc.Config.Env["GITHUB_REPOSITORY"],
		Ref:              rc.Config.Env["GITHUB_REF"],
		Sha:              rc.Config.Env["SHA_REF"],
		RefName:          rc.Config.Env["GITHUB_REF_NAME"],
		RefType:          rc.Config.Env["GITHUB_REF_TYPE"],
		BaseRef:          rc.Config.Env["GITHUB_BASE_REF"],
		HeadRef:          rc.Config.Env["GITHUB_HEAD_REF"],
		Workspace:        rc.Config.Env["GITHUB_WORKSPACE"],
	}
	if rc.JobContainer != nil {
		ghc.EventPath = rc.JobContainer.GetActPath() + "/workflow/event.json"
		ghc.Workspace = rc.JobContainer.ToContainerPath(rc.Config.Workdir)
	}

	if ghc.RunAttempt == "" {
		ghc.RunAttempt = "1"
	}

	if ghc.RunID == "" {
		ghc.RunID = "1"
	}

	if ghc.RunNumber == "" {
		ghc.RunNumber = "1"
	}

	if ghc.RetentionDays == "" {
		ghc.RetentionDays = "0"
	}

	if ghc.RunnerPerflog == "" {
		ghc.RunnerPerflog = "/dev/null"
	}

	// Backwards compatibility for configs that require
	// a default rather than being run as a cmd
	if ghc.Actor == "" {
		ghc.Actor = "nektos/act"
	}

	if rc.EventJSON != "" {
		err := json.Unmarshal([]byte(rc.EventJSON), &ghc.Event)
		if err != nil {
			logger.Errorf("Unable to Unmarshal event '%s': %v", rc.EventJSON, err)
		}
	}

	ghc.SetBaseAndHeadRef()
	repoPath := rc.Config.Workdir
	ghc.SetRepositoryAndOwner(ctx, rc.Config.GitHubInstance, rc.Config.RemoteName, repoPath)
	if ghc.Ref == "" {
		ghc.SetRef(ctx, rc.Config.DefaultBranch, repoPath)
	}
	if ghc.Sha == "" {
		ghc.SetSha(ctx, repoPath)
	}

	ghc.SetRefTypeAndName()

	// defaults
	ghc.ServerURL = "https://github.com"
	ghc.APIURL = "https://api.github.com"
	ghc.GraphQLURL = "https://api.github.com/graphql"
	// per GHES
	if rc.Config.GitHubInstance != "github.com" {
		ghc.ServerURL = fmt.Sprintf("https://%s", rc.Config.GitHubInstance)
		ghc.APIURL = fmt.Sprintf("https://%s/api/v3", rc.Config.GitHubInstance)
		ghc.GraphQLURL = fmt.Sprintf("https://%s/api/graphql", rc.Config.GitHubInstance)
	}
	// allow to be overridden by user
	if rc.Config.Env["GITHUB_SERVER_URL"] != "" {
		ghc.ServerURL = rc.Config.Env["GITHUB_SERVER_URL"]
	}
	if rc.Config.Env["GITHUB_API_URL"] != "" {
		ghc.APIURL = rc.Config.Env["GITHUB_API_URL"]
	}
	if rc.Config.Env["GITHUB_GRAPHQL_URL"] != "" {
		ghc.GraphQLURL = rc.Config.Env["GITHUB_GRAPHQL_URL"]
	}

	return ghc
}

func isLocalCheckout(ghc *model.GithubContext, step *model.Step) bool {
	if step.Type() == model.StepTypeInvalid {
		// This will be errored out by the executor later, we need this here to avoid a null panic though
		return false
	}
	if step.Type() != model.StepTypeUsesActionRemote {
		return false
	}
	remoteAction := newRemoteAction(step.Uses)
	if remoteAction == nil {
		// IsCheckout() will nil panic if we dont bail out early
		return false
	}
	if !remoteAction.IsCheckout() {
		return false
	}

	if repository, ok := step.With["repository"]; ok && repository != ghc.Repository {
		return false
	}
	if repository, ok := step.With["ref"]; ok && repository != ghc.Ref {
		return false
	}
	return true
}

func nestedMapLookup(m map[string]interface{}, ks ...string) (rval interface{}) {
	var ok bool

	if len(ks) == 0 { // degenerate input
		return nil
	}
	if rval, ok = m[ks[0]]; !ok {
		return nil
	} else if len(ks) == 1 { // we've reached the final key
		return rval
	} else if m, ok = rval.(map[string]interface{}); !ok {
		return nil
	}
	// 1+ more keys
	return nestedMapLookup(m, ks[1:]...)
}

func (rc *RunContext) withGithubEnv(ctx context.Context, github *model.GithubContext, env map[string]string) map[string]string {
	env["CI"] = "true"
	env["GITHUB_WORKFLOW"] = github.Workflow
	env["GITHUB_RUN_ATTEMPT"] = github.RunAttempt
	env["GITHUB_RUN_ID"] = github.RunID
	env["GITHUB_RUN_NUMBER"] = github.RunNumber
	env["GITHUB_ACTION"] = github.Action
	env["GITHUB_ACTION_PATH"] = github.ActionPath
	env["GITHUB_ACTION_REPOSITORY"] = github.ActionRepository
	env["GITHUB_ACTION_REF"] = github.ActionRef
	env["GITHUB_ACTIONS"] = "true"
	env["GITHUB_ACTOR"] = github.Actor
	env["GITHUB_REPOSITORY"] = github.Repository
	env["GITHUB_EVENT_NAME"] = github.EventName
	env["GITHUB_EVENT_PATH"] = github.EventPath
	env["GITHUB_WORKSPACE"] = github.Workspace
	env["GITHUB_SHA"] = github.Sha
	env["GITHUB_REF"] = github.Ref
	env["GITHUB_REF_NAME"] = github.RefName
	env["GITHUB_REF_TYPE"] = github.RefType
	env["GITHUB_JOB"] = github.Job
	env["GITHUB_REPOSITORY_OWNER"] = github.RepositoryOwner
	env["GITHUB_RETENTION_DAYS"] = github.RetentionDays
	env["RUNNER_PERFLOG"] = github.RunnerPerflog
	env["RUNNER_TRACKING_ID"] = github.RunnerTrackingID
	env["GITHUB_BASE_REF"] = github.BaseRef
	env["GITHUB_HEAD_REF"] = github.HeadRef
	env["GITHUB_SERVER_URL"] = github.ServerURL
	env["GITHUB_API_URL"] = github.APIURL
	env["GITHUB_GRAPHQL_URL"] = github.GraphQLURL

	if rc.Config.ArtifactServerPath != "" {
		setActionRuntimeVars(rc, env)
	}

	for _, platformName := range rc.runsOnPlatformNames(ctx) {
		if platformName != "" {
			if platformName == "ubuntu-latest" {
				// hardcode current ubuntu-latest since we have no way to check that 'on the fly'
				env["ImageOS"] = "ubuntu20"
			} else {
				platformName = strings.SplitN(strings.Replace(platformName, `-`, ``, 1), `.`, 2)[0]
				env["ImageOS"] = platformName
			}
		}
	}

	return env
}

func setActionRuntimeVars(rc *RunContext, env map[string]string) {
	actionsRuntimeURL := os.Getenv("ACTIONS_RUNTIME_URL")
	if actionsRuntimeURL == "" {
		actionsRuntimeURL = fmt.Sprintf("http://%s:%s/", rc.Config.ArtifactServerAddr, rc.Config.ArtifactServerPort)
	}
	env["ACTIONS_RUNTIME_URL"] = actionsRuntimeURL
	env["ACTIONS_RESULTS_URL"] = actionsRuntimeURL

	actionsRuntimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	if actionsRuntimeToken == "" {
		runID := int64(1)
		if rid, ok := rc.Config.Env["GITHUB_RUN_ID"]; ok {
			runID, _ = strconv.ParseInt(rid, 10, 64)
		}
		actionsRuntimeToken, _ = common.CreateAuthorizationToken(runID, runID, runID)
	}
	env["ACTIONS_RUNTIME_TOKEN"] = actionsRuntimeToken
}

func (rc *RunContext) handleCredentials(ctx context.Context) (string, string, error) {
	// TODO: remove below 2 lines when we can release act with breaking changes
	username := rc.Config.Secrets["DOCKER_USERNAME"]
	password := rc.Config.Secrets["DOCKER_PASSWORD"]

	container := rc.Run.Job().Container()
	if container == nil || container.Credentials == nil {
		return username, password, nil
	}

	if container.Credentials != nil && len(container.Credentials) != 2 {
		err := fmt.Errorf("invalid property count for key 'credentials:'")
		return "", "", err
	}

	ee := rc.NewExpressionEvaluator(ctx)
	if username = ee.Interpolate(ctx, container.Credentials["username"]); username == "" {
		err := fmt.Errorf("failed to interpolate container.credentials.username")
		return "", "", err
	}
	if password = ee.Interpolate(ctx, container.Credentials["password"]); password == "" {
		err := fmt.Errorf("failed to interpolate container.credentials.password")
		return "", "", err
	}

	if container.Credentials["username"] == "" || container.Credentials["password"] == "" {
		err := fmt.Errorf("container.credentials cannot be empty")
		return "", "", err
	}

	return username, password, nil
}

func (rc *RunContext) handleServiceCredentials(ctx context.Context, creds map[string]string) (username, password string, err error) {
	if creds == nil {
		return
	}
	if len(creds) != 2 {
		err = fmt.Errorf("invalid property count for key 'credentials:'")
		return
	}

	ee := rc.NewExpressionEvaluator(ctx)
	if username = ee.Interpolate(ctx, creds["username"]); username == "" {
		err = fmt.Errorf("failed to interpolate credentials.username")
		return
	}

	if password = ee.Interpolate(ctx, creds["password"]); password == "" {
		err = fmt.Errorf("failed to interpolate credentials.password")
		return
	}

	return
}

// GetServiceBindsAndMounts returns the binds and mounts for the service container, resolving paths as appropriate
func (rc *RunContext) GetServiceBindsAndMounts(svcVolumes []string) ([]string, map[string]string) {
	if rc.Config.ContainerDaemonSocket == "" {
		rc.Config.ContainerDaemonSocket = "/var/run/docker.sock"
	}
	binds := []string{}
	if rc.Config.ContainerDaemonSocket != "-" {
		daemonPath := getDockerDaemonSocketMountPath(rc.Config.ContainerDaemonSocket)
		binds = append(binds, fmt.Sprintf("%s:%s", daemonPath, "/var/run/docker.sock"))
	}

	mounts := map[string]string{}

	for _, v := range svcVolumes {
		if !strings.Contains(v, ":") || filepath.IsAbs(v) {
			// Bind anonymous volume or host file.
			binds = append(binds, v)
		} else {
			// Mount existing volume.
			paths := strings.SplitN(v, ":", 2)
			mounts[paths[0]] = paths[1]
		}
	}

	return binds, mounts
}
