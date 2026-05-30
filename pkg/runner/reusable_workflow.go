package runner

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sync"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/common/git"
	"github.com/nektos/act/pkg/model"
)

func newLocalReusableWorkflowExecutor(rc *RunContext) common.Executor {
	return newReusableWorkflowExecutor(rc, rc.Config.Workdir, rc.Run.Job().Uses)
}

func newRemoteReusableWorkflowExecutor(rc *RunContext) common.Executor {
	return func(ctx context.Context) error {
		uses := rc.Run.Job().Uses

		resolver := NewActionSourceResolver(ActionSourceResolverConfig{
			ServerURL:                          rc.getGithubContext(ctx).ServerURL,
			Token:                              rc.Config.Token,
			Workdir:                            rc.Config.Workdir,
			ActionCacheDir:                     rc.ActionCacheDir(),
			ReplaceGheActionWithGithubCom:      rc.Config.ReplaceGheActionWithGithubCom,
			ReplaceGheActionTokenWithGithubCom: rc.Config.ReplaceGheActionTokenWithGithubCom,
			ActionCache:                        rc.Config.ActionCache,
			OfflineMode:                        rc.Config.ActionOfflineMode,
		})

		actionSource, err := resolver.ResolveWorkflow(ctx, uses)
		if err != nil {
			return err
		}

		if rc.Config.ActionCache != nil {
			return newActionCacheReusableWorkflowExecutor(resolver, actionSource, rc)(ctx)
		}

		return common.NewPipelineExecutor(
			newMutexExecutor(cloneIfRequired(rc, actionSource)),
			newReusableWorkflowExecutor(rc, actionSource.ActionDir(), actionSource.WorkflowFilePath()),
		)(ctx)
	}
}

func newActionCacheReusableWorkflowExecutor(resolver ActionSourceResolver, actionSource *ActionSource, rc *RunContext) common.Executor {
	return func(ctx context.Context) error {
		_, err := resolver.Fetch(ctx, actionSource)
		if err != nil {
			return err
		}

		includePrefix := actionSource.WorkflowFilePath()
		archive, err := resolver.GetTarArchive(ctx, actionSource, includePrefix)
		if err != nil {
			return err
		}
		defer archive.Close()

		treader := tar.NewReader(archive)
		if _, err = treader.Next(); err != nil {
			return actionSource.formatCacheError("failed to read workflow file: %w", err)
		}

		planner, err := model.NewSingleWorkflowPlanner(actionSource.Filename, treader)
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

func cloneIfRequired(rc *RunContext, actionSource *ActionSource) common.Executor {
	return common.NewConditionalExecutor(
		func(_ context.Context) bool {
			_, err := os.Stat(actionSource.ActionDir())
			notExists := errors.Is(err, fs.ErrNotExist)
			return notExists
		},
		func(ctx context.Context) error {
			return git.NewGitCloneExecutor(git.NewGitCloneExecutorInput{
				URL:         actionSource.CloneURL,
				Ref:         actionSource.Ref,
				Dir:         actionSource.ActionDir(),
				Token:       actionSource.Token,
				OfflineMode: rc.Config.ActionOfflineMode,
			})(ctx)
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

type remoteReusableWorkflow struct {
	URL      string
	Org      string
	Repo     string
	Filename string
	Ref      string
}

func (r *remoteReusableWorkflow) CloneURL() string {
	return fmt.Sprintf("%s/%s/%s", r.URL, r.Org, r.Repo)
}

func newRemoteReusableWorkflow(uses string) *remoteReusableWorkflow {
	// GitHub docs:
	// https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#jobsjob_iduses
	r := regexp.MustCompile(`^([^/]+)/([^/]+)/.github/workflows/([^@]+)@(.*)$`)
	matches := r.FindStringSubmatch(uses)
	if len(matches) != 5 {
		return nil
	}
	return &remoteReusableWorkflow{
		Org:      matches[1],
		Repo:     matches[2],
		Filename: matches[3],
		Ref:      matches[4],
		URL:      "https://github.com",
	}
}
