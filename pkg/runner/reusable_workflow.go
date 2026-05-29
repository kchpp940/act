package runner

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sync"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/common/git"
	"github.com/nektos/act/pkg/model"
)

func newLocalReusableWorkflowExecutor(rc *RunContext) common.Executor {
	return newReusableWorkflowExecutor(rc, rc.Config.Workdir, rc.Run.Job().Uses)
}

func newRemoteReusableWorkflowExecutor(rc *RunContext) common.Executor {
	uses := rc.Run.Job().Uses
	ghctx := rc.getGithubContext(context.Background())

	ar := newReusableWorkflowRef(uses, ghctx, rc.Config)
	if ar == nil {
		return common.NewErrorExecutor(fmt.Errorf("expected format {owner}/{repo}/.github/workflows/{filename}@{ref}. Actual '%s' Input string was not in a correct format", uses))
	}

	workflowDir := fmt.Sprintf("%s/%s", rc.ActionCacheDir(), ar.ExecutionDir())

	if rc.Config.ActionCache != nil {
		return newActionCacheReusableWorkflowExecutor(rc, ar)
	}

	return common.NewPipelineExecutor(
		newMutexExecutor(cloneIfRequired(rc, ar, workflowDir)),
		newReusableWorkflowExecutor(rc, workflowDir, fmt.Sprintf("./%s", ar.Path)),
	)
}

func newActionCacheReusableWorkflowExecutor(rc *RunContext, ar *actionRef) common.Executor {
	return func(ctx context.Context) error {
		cacheDir := ar.RepoCacheKey()
		sha, err := rc.Config.ActionCache.Fetch(ctx, ar)
		if err != nil {
			return ar.FetchError(err)
		}
		filename := path.Base(ar.Path)
		archive, err := rc.Config.ActionCache.GetTarArchive(ctx, cacheDir, sha, ar.Path)
		if err != nil {
			return err
		}
		defer archive.Close()
		treader := tar.NewReader(archive)
		if _, err = treader.Next(); err != nil {
			return err
		}
		planner, err := model.NewSingleWorkflowPlanner(filename, treader)
		if err != nil {
			return err
		}
		plan, err := planner.PlanEvent("workflow_call")
		if err != nil {
			return err
		}

		runner, err := NewReusableWorkflowRunner(rc)
		if err != nil {
			return err
		}

		return runner.NewPlanExecutor(plan)(ctx)
	}
}

var (
	executorLock sync.Mutex
)

func newMutexExecutor(executor common.Executor) common.Executor {
	return func(ctx context.Context) error {
		executorLock.Lock()
		defer executorLock.Unlock()

		return executor(ctx)
	}
}

func cloneIfRequired(rc *RunContext, ar *actionRef, targetDirectory string) common.Executor {
	return common.NewConditionalExecutor(
		func(_ context.Context) bool {
			_, err := os.Stat(targetDirectory)
			notExists := errors.Is(err, fs.ErrNotExist)
			return notExists
		},
		func(ctx context.Context) error {
			return git.NewGitCloneExecutor(ar.GitCloneInput(targetDirectory))(ctx)
		},
		nil,
	)
}

func newReusableWorkflowExecutor(rc *RunContext, directory string, workflow string) common.Executor {
	return func(ctx context.Context) error {
		planner, err := model.NewWorkflowPlanner(path.Join(directory, workflow), true, false)
		if err != nil {
			return err
		}

		plan, err := planner.PlanEvent("workflow_call")
		if err != nil {
			return err
		}

		runner, err := NewReusableWorkflowRunner(rc)
		if err != nil {
			return err
		}

		return runner.NewPlanExecutor(plan)(ctx)
	}
}

func NewReusableWorkflowRunner(rc *RunContext) (Runner, error) {
	runner := &runnerImpl{
		config:    rc.Config,
		eventJSON: rc.EventJSON,
		caller: &caller{
			runContext: rc,
		},
	}

	return runner.configure()
}
