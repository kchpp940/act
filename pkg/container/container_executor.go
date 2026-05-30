package container

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/nektos/act/pkg/common"
)

type RuntimeSpecBuilder struct {
	spec *RuntimeSpec
}

func NewRuntimeSpecBuilder(base *RuntimeSpec) *RuntimeSpecBuilder {
	if base == nil {
		base = NewRuntimeSpec()
	}
	return &RuntimeSpecBuilder{spec: base}
}

func (b *RuntimeSpecBuilder) WithImage(image string) *RuntimeSpecBuilder {
	b.spec.Container.Image = image
	b.spec.Image.Ref = image
	return b
}

func (b *RuntimeSpecBuilder) WithName(name string) *RuntimeSpecBuilder {
	b.spec.Container.Name = name
	return b
}

func (b *RuntimeSpecBuilder) WithCmd(cmd []string) *RuntimeSpecBuilder {
	b.spec.Container.Cmd = cmd
	return b
}

func (b *RuntimeSpecBuilder) WithEntrypoint(entrypoint []string) *RuntimeSpecBuilder {
	b.spec.Container.Entrypoint = entrypoint
	return b
}

func (b *RuntimeSpecBuilder) WithWorkingDir(workingDir string) *RuntimeSpecBuilder {
	b.spec.Container.WorkingDir = workingDir
	return b
}

func (b *RuntimeSpecBuilder) WithEnv(env []string) *RuntimeSpecBuilder {
	b.spec.Container.Env = append(b.spec.Container.Env, env...)
	return b
}

func (b *RuntimeSpecBuilder) WithEnvMap(env map[string]string) *RuntimeSpecBuilder {
	for k, v := range env {
		b.spec.Container.Env = append(b.spec.Container.Env, fmt.Sprintf("%s=%s", k, v))
	}
	return b
}

func (b *RuntimeSpecBuilder) WithBinds(binds []string) *RuntimeSpecBuilder {
	b.spec.Container.Binds = append(b.spec.Container.Binds, binds...)
	return b
}

func (b *RuntimeSpecBuilder) WithMounts(mounts map[string]string) *RuntimeSpecBuilder {
	for k, v := range mounts {
		b.spec.Container.Mounts[k] = v
	}
	return b
}

func (b *RuntimeSpecBuilder) WithNetworkMode(networkMode string) *RuntimeSpecBuilder {
	b.spec.Container.NetworkMode = networkMode
	b.spec.Network.Mode = networkMode
	return b
}

func (b *RuntimeSpecBuilder) WithNetworkName(networkName string) *RuntimeSpecBuilder {
	b.spec.Network.Name = networkName
	return b
}

func (b *RuntimeSpecBuilder) WithNetworkAliases(aliases []string) *RuntimeSpecBuilder {
	b.spec.Container.NetworkAliases = aliases
	b.spec.Network.Aliases = aliases
	return b
}

func (b *RuntimeSpecBuilder) WithNetworkLifecycle(lifecycle NetworkLifecycle) *RuntimeSpecBuilder {
	b.spec.Network.Lifecycle = lifecycle
	return b
}

func (b *RuntimeSpecBuilder) WithPrivileged(privileged bool) *RuntimeSpecBuilder {
	b.spec.Container.Privileged = privileged
	return b
}

func (b *RuntimeSpecBuilder) WithUsernsMode(usernsMode string) *RuntimeSpecBuilder {
	b.spec.Container.UsernsMode = usernsMode
	return b
}

func (b *RuntimeSpecBuilder) WithPlatform(platform string) *RuntimeSpecBuilder {
	b.spec.Container.Platform = platform
	b.spec.Image.Platform = platform
	return b
}

func (b *RuntimeSpecBuilder) WithOptions(options string) *RuntimeSpecBuilder {
	b.spec.Container.Options = options
	return b
}

func (b *RuntimeSpecBuilder) WithCredentials(username, password string) *RuntimeSpecBuilder {
	b.spec.Container.Username = username
	b.spec.Container.Password = password
	b.spec.Image.Username = username
	b.spec.Image.Password = password
	return b
}

func (b *RuntimeSpecBuilder) WithStdout(stdout io.Writer) *RuntimeSpecBuilder {
	b.spec.Container.Stdout = stdout
	return b
}

func (b *RuntimeSpecBuilder) WithStderr(stderr io.Writer) *RuntimeSpecBuilder {
	b.spec.Container.Stderr = stderr
	return b
}

func (b *RuntimeSpecBuilder) WithExposedPorts(ports nat.PortSet) *RuntimeSpecBuilder {
	for p := range ports {
		b.spec.Container.ExposedPorts[p] = struct{}{}
	}
	return b
}

func (b *RuntimeSpecBuilder) WithPortBindings(bindings nat.PortMap) *RuntimeSpecBuilder {
	for p, bnd := range bindings {
		b.spec.Container.PortBindings[p] = bnd
	}
	return b
}

func (b *RuntimeSpecBuilder) WithForcePull(forcePull bool) *RuntimeSpecBuilder {
	b.spec.Image.ForcePull = forcePull
	if forcePull {
		b.spec.Image.PullPolicy = ImagePullPolicyAlways
	}
	return b
}

func (b *RuntimeSpecBuilder) WithForceRebuild(forceRebuild bool) *RuntimeSpecBuilder {
	b.spec.Image.ForceRebuild = forceRebuild
	if forceRebuild {
		b.spec.Image.BuildPolicy = ImageBuildPolicyAlways
	}
	return b
}

func (b *RuntimeSpecBuilder) WithPullPolicy(policy ImagePullPolicy) *RuntimeSpecBuilder {
	b.spec.Image.PullPolicy = policy
	return b
}

func (b *RuntimeSpecBuilder) WithBuildPolicy(policy ImageBuildPolicy) *RuntimeSpecBuilder {
	b.spec.Image.BuildPolicy = policy
	return b
}

func (b *RuntimeSpecBuilder) WithBuildSpec(buildSpec *ImageBuildSpec) *RuntimeSpecBuilder {
	b.spec.Image.BuildContext = buildSpec
	return b
}

func (b *RuntimeSpecBuilder) WithReuseContainer(reuse bool) *RuntimeSpecBuilder {
	b.spec.ReuseContainer = reuse
	if reuse && b.spec.CleanupPlan != nil {
		b.spec.CleanupPlan.Condition = CleanupConditionNever
	}
	return b
}

func (b *RuntimeSpecBuilder) WithCleanupPolicy(policy CleanupCondition) *RuntimeSpecBuilder {
	if b.spec.CleanupPlan != nil {
		b.spec.CleanupPlan.Condition = policy
	}
	return b
}

func (b *RuntimeSpecBuilder) WithRemoveVolumes(remove bool) *RuntimeSpecBuilder {
	if b.spec.CleanupPlan == nil {
		return b
	}
	hasAction := false
	for _, a := range b.spec.CleanupPlan.Actions {
		if a == CleanupActionRemoveVolumes {
			hasAction = true
			break
		}
	}
	if remove && !hasAction {
		b.spec.CleanupPlan.Actions = append(b.spec.CleanupPlan.Actions, CleanupActionRemoveVolumes)
	} else if !remove && hasAction {
		newActions := make([]CleanupAction, 0, len(b.spec.CleanupPlan.Actions))
		for _, a := range b.spec.CleanupPlan.Actions {
			if a != CleanupActionRemoveVolumes {
				newActions = append(newActions, a)
			}
		}
		b.spec.CleanupPlan.Actions = newActions
	}
	return b
}

func (b *RuntimeSpecBuilder) WithAttach(attach bool) *RuntimeSpecBuilder {
	b.spec.Lifecycle.Attach = attach
	if attach {
		b.spec.Lifecycle.Wait = true
	}
	return b
}

func (b *RuntimeSpecBuilder) WithWait(wait bool) *RuntimeSpecBuilder {
	b.spec.Lifecycle.Wait = wait
	return b
}

func (b *RuntimeSpecBuilder) WithCopyFiles(files ...*FileEntry) *RuntimeSpecBuilder {
	b.spec.Lifecycle.CopyFiles = append(b.spec.Lifecycle.CopyFiles, files...)
	return b
}

func (b *RuntimeSpecBuilder) WithCapabilities(capAdd, capDrop []string) *RuntimeSpecBuilder {
	b.spec.CapAdd = capAdd
	b.spec.CapDrop = capDrop
	return b
}

func (b *RuntimeSpecBuilder) WithScopeID(scopeID string) *RuntimeSpecBuilder {
	b.spec.ScopeID = scopeID
	return b
}

func (b *RuntimeSpecBuilder) WithManagedByScope(managed bool) *RuntimeSpecBuilder {
	b.spec.ManagedByScope = managed
	return b
}

func (b *RuntimeSpecBuilder) WithDefaultRunnerEnv(ctx context.Context) *RuntimeSpecBuilder {
	env := []string{
		fmt.Sprintf("%s=%s", "RUNNER_TOOL_CACHE", "/opt/hostedtoolcache"),
		fmt.Sprintf("%s=%s", "RUNNER_OS", "Linux"),
		fmt.Sprintf("%s=%s", "RUNNER_ARCH", RunnerArch(ctx)),
		fmt.Sprintf("%s=%s", "RUNNER_TEMP", "/tmp"),
	}
	return b.WithEnv(env)
}

func (b *RuntimeSpecBuilder) Build() *RuntimeSpec {
	return b.spec
}

func (b *RuntimeSpecBuilder) BuildContainer() Container {
	return NewContainer(b.spec.Container)
}

func (b *RuntimeSpecBuilder) BuildExecEnvironment() ExecutionsEnvironment {
	return NewContainer(b.spec.Container)
}

func (b *RuntimeSpecBuilder) Execute(factories ...ContainerFactory) common.Executor {
	return NewContainerExecutor(b.spec, factories...).Execute()
}

type ContainerExecutor struct {
	spec        *RuntimeSpec
	container   Container
	environment ExecutionsEnvironment
}

type ContainerFactory func(*NewContainerInput) ExecutionsEnvironment

func NewContainerExecutor(spec *RuntimeSpec, factories ...ContainerFactory) *ContainerExecutor {
	var env ExecutionsEnvironment
	if len(factories) > 0 && factories[0] != nil {
		env = factories[0](spec.Container)
	} else {
		env = NewContainer(spec.Container)
	}
	return &ContainerExecutor{
		spec:        spec,
		container:   env,
		environment: env,
	}
}

func (e *ContainerExecutor) Container() Container {
	return e.container
}

func (e *ContainerExecutor) Environment() ExecutionsEnvironment {
	return e.environment
}

func (e *ContainerExecutor) Execute() common.Executor {
	return func(ctx context.Context) error {
		return e.buildPipeline()(ctx)
	}
}

func (e *ContainerExecutor) buildPipeline() common.Executor {
	pipeline := common.NewPipelineExecutor()

	pipeline = pipeline.Then(e.setupNetwork())
	pipeline = pipeline.Then(e.prepareImage())
	pipeline = pipeline.Then(e.removeExisting())
	pipeline = pipeline.Then(e.createContainer())
	pipeline = pipeline.Then(e.copyFiles())
	pipeline = pipeline.Then(e.startContainer())

	return pipeline.Finally(e.cleanup())
}

func (e *ContainerExecutor) setupNetwork() common.Executor {
	return func(ctx context.Context) error {
		if e.spec.ScopeID != "" {
			return nil
		}
		if e.spec.Network.Lifecycle == NetworkLifecycleCreateAndManage && e.spec.Network.CreateIfNotExists {
			return NewDockerNetworkCreateExecutor(e.spec.Network.Name).IfNot(common.Dryrun)(ctx)
		}
		return nil
	}
}

func (e *ContainerExecutor) prepareImage() common.Executor {
	return func(ctx context.Context) error {
		if e.spec.Image.BuildContext != nil {
			return e.buildImage()(ctx)
		}
		if e.spec.Image.PullPolicy != ImagePullPolicyNever {
			forcePull := e.spec.Image.ForcePull || e.spec.Image.PullPolicy == ImagePullPolicyAlways
			return e.container.Pull(forcePull)(ctx)
		}
		return nil
	}
}

func (e *ContainerExecutor) buildImage() common.Executor {
	return func(ctx context.Context) error {
		if e.spec.Image.BuildContext == nil {
			return nil
		}

		image := e.spec.Image.Ref
		correctArchExists, err := ImageExistsLocally(ctx, image, e.spec.Image.Platform)
		if err != nil {
			return err
		}

		needsBuild := e.spec.Image.ForceRebuild || !correctArchExists
		if !needsBuild && e.spec.Image.BuildPolicy == ImageBuildPolicyIfMissing {
			return nil
		}
		if e.spec.Image.BuildPolicy == ImageBuildPolicyNever {
			return nil
		}

		if correctArchExists && e.spec.Image.ForceRebuild {
			_, err := RemoveImage(ctx, image, true, true)
			if err != nil {
				return err
			}
		}

		return NewDockerBuildExecutor(NewDockerBuildExecutorInput{
			ContextDir:   e.spec.Image.BuildContext.ContextDir,
			Dockerfile:   e.spec.Image.BuildContext.Dockerfile,
			ImageTag:     image,
			BuildContext: e.spec.Image.BuildContext.BuildContext,
			Platform:     e.spec.Image.Platform,
		})(ctx)
	}
}

func (e *ContainerExecutor) removeExisting() common.Executor {
	return func(ctx context.Context) error {
		if e.spec.ReuseContainer {
			return nil
		}
		return e.container.Remove()(ctx)
	}
}

func (e *ContainerExecutor) createContainer() common.Executor {
	return func(ctx context.Context) error {
		if !e.spec.Lifecycle.Create {
			return nil
		}
		return e.container.Create(e.spec.CapAdd, e.spec.CapDrop)(ctx)
	}
}

func (e *ContainerExecutor) copyFiles() common.Executor {
	return func(ctx context.Context) error {
		if len(e.spec.Lifecycle.CopyFiles) == 0 {
			return nil
		}
		destPath := e.environment.GetActPath() + "/"
		return e.container.Copy(destPath, e.spec.Lifecycle.CopyFiles...)(ctx)
	}
}

func (e *ContainerExecutor) startContainer() common.Executor {
	return func(ctx context.Context) error {
		if !e.spec.Lifecycle.Start {
			return nil
		}
		return e.container.Start(e.spec.Lifecycle.Attach)(ctx)
	}
}

func (e *ContainerExecutor) cleanup() common.Executor {
	return func(ctx context.Context) error {
		jobErr := common.JobError(ctx)

		plan := e.spec.CleanupPlan
		if plan == nil {
			return nil
		}

		if plan.Owner == CleanupOwnerSelf {
			if plan.ShouldRemoveContainer(jobErr, e.spec.ReuseContainer) {
				if plan.HasAction(CleanupActionRemoveContainer) {
					_ = e.container.Remove()(ctx)
				}
			}
			if plan.HasAction(CleanupActionCloseClient) {
				_ = e.container.Close()(ctx)
			}
		}

		if e.spec.ScopeID == "" {
			if e.spec.Network.CleanupPlan != nil && e.spec.Network.CleanupPlan.ShouldRemoveContainer(jobErr, false) {
				_ = NewDockerNetworkRemoveExecutor(e.spec.Network.Name)(ctx)
			}
		}

		return nil
	}
}

func NetworkModeContainer(containerName string) string {
	return fmt.Sprintf("container:%s", containerName)
}

func ParseVolumeSpec(spec string) (isBind bool, source, target string) {
	if !strings.Contains(spec, ":") {
		return true, spec, spec
	}
	parts := strings.SplitN(spec, ":", 2)
	if strings.HasPrefix(parts[0], "/") || strings.HasPrefix(parts[0], "./") || strings.HasPrefix(parts[0], "../") {
		return true, parts[0], parts[1]
	}
	return false, parts[0], parts[1]
}
