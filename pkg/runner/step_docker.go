package runner

import (
	"context"
	"fmt"
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

		cfg := DockerStepConfig{
			StepID:     step.ID,
			Image:      image,
			Entrypoint: entrypoint,
			Cmd:        cmd,
			Env:        sd.env,
		}

		spec, err := rc.newDockerStepRuntimeSpec(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to create docker step spec: %w", err)
		}

		rawLogger := common.Logger(ctx).WithField("raw_output", true)
		logWriter := common.NewLineWriter(rc.commandHandler(ctx), func(s string) bool {
			if rc.Config.LogOutput {
				rawLogger.Infof("%s", s)
			} else {
				rawLogger.Debugf("%s", s)
			}
			return true
		})

		spec.WorkingDir = rc.JobContainer.ToContainerPath(rc.Config.Workdir)
		spec.Stdout = logWriter
		spec.Stderr = logWriter

		stepContainer := ContainerNewContainer(spec.ToNewContainerInput())

		shouldForcePull := !spec.ShouldSkipPull()
		forcePull := spec.ShouldForcePull()
		shouldRemove := spec.ShouldRemove()

		pullExec := stepContainer.Pull(forcePull).IfBool(shouldForcePull)

		return common.NewPipelineExecutor(
			pullExec,
			stepContainer.Remove().IfBool(shouldRemove),
			stepContainer.Create(rc.Config.ContainerCapAdd, rc.Config.ContainerCapDrop),
			stepContainer.Start(true),
		).Finally(
			stepContainer.Remove().IfBool(shouldRemove),
		).Finally(stepContainer.Close())(ctx)
	}
}

var (
	ContainerNewContainer = container.NewContainer
)
