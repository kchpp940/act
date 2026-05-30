package runner

import (
	"context"
	"strings"

	"github.com/kballard/go-shellquote"
	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/container"
	"github.com/nektos/act/pkg/model"
)

type stepDocker struct {
	Step       *model.Step
	RunContext *RunContext
	env        map[string]string
}

func (sd *stepDocker) pre() common.Executor {
	return func(_ context.Context) error {
		return nil
	}
}

func (sd *stepDocker) main() common.Executor {
	sd.env = map[string]string{}

	return runStepExecutor(sd, stepStageMain, sd.runUsesContainer())
}

func (sd *stepDocker) post() common.Executor {
	return func(_ context.Context) error {
		return nil
	}
}

func (sd *stepDocker) getRunContext() *RunContext {
	return sd.RunContext
}

func (sd *stepDocker) getGithubContext(ctx context.Context) *model.GithubContext {
	return sd.getRunContext().getGithubContext(ctx)
}

func (sd *stepDocker) getStepModel() *model.Step {
	return sd.Step
}

func (sd *stepDocker) getEnv() *map[string]string {
	return &sd.env
}

func (sd *stepDocker) getIfExpression(_ context.Context, _ stepStage) string {
	return sd.Step.If.Value
}

func (sd *stepDocker) runUsesContainer() common.Executor {
	rc := sd.RunContext
	step := sd.Step

	return func(ctx context.Context) error {
		image := strings.TrimPrefix(step.Uses, "docker://")
		eval := rc.NewExpressionEvaluator(ctx)
		cmd, err := shellquote.Split(eval.Interpolate(ctx, step.With["args"]))
		if err != nil {
			return err
		}

		var entrypoint []string
		if entry := eval.Interpolate(ctx, step.With["entrypoint"]); entry != "" {
			entrypoint = []string{entry}
		}

		builder := sd.createStepRuntimeSpecBuilder(ctx, image, cmd, entrypoint)

		if rc.JobRuntimeScope != nil {
			builder = builder.WithScopeID(rc.JobRuntimeScope.ID)
		}
		return builder.Execute(func(input *container.NewContainerInput) container.ExecutionsEnvironment {
			return ContainerNewContainer(input)
		})(ctx)
	}
}

var (
	ContainerNewContainer = container.NewContainer
)

func (sd *stepDocker) createStepRuntimeSpecBuilder(ctx context.Context, image string, cmd []string, entrypoint []string) *container.RuntimeSpecBuilder {
	rc := sd.RunContext
	step := sd.Step

	rawLogger := common.Logger(ctx).WithField("raw_output", true)
	logWriter := common.NewLineWriter(rc.commandHandler(ctx), func(s string) bool {
		if rc.Config.LogOutput {
			rawLogger.Infof("%s", s)
		} else {
			rawLogger.Debugf("%s", s)
		}
		return true
	})

	binds, mounts := rc.GetBindsAndMounts()
	builder := container.NewRuntimeSpecBuilder(container.NewStepRuntimeSpec()).
		WithName(createContainerName(rc.jobContainerName(), step.ID)).
		WithImage(image).
		WithCmd(cmd).
		WithEntrypoint(entrypoint).
		WithWorkingDir(rc.JobContainer.ToContainerPath(rc.Config.Workdir)).
		WithCredentials(rc.Config.Secrets["DOCKER_USERNAME"], rc.Config.Secrets["DOCKER_PASSWORD"]).
		WithEnvMap(sd.env).
		WithDefaultRunnerEnv(ctx).
		WithBinds(binds).
		WithMounts(mounts).
		WithNetworkMode(container.NetworkModeContainer(rc.jobContainerName())).
		WithStdout(logWriter).
		WithStderr(logWriter).
		WithPrivileged(rc.Config.Privileged).
		WithUsernsMode(rc.Config.UsernsMode).
		WithPlatform(rc.Config.ContainerArchitecture).
		WithForcePull(rc.Config.ForcePull).
		WithReuseContainer(rc.Config.ReuseContainers).
		WithCapabilities(rc.Config.ContainerCapAdd, rc.Config.ContainerCapDrop).
		WithAttach(true).
		WithWait(true)

	return builder
}
