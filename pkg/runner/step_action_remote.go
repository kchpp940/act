package runner

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/common/git"
	"github.com/nektos/act/pkg/model"
)

type stepActionRemote struct {
	Step                *model.Step
	RunContext          *RunContext
	compositeRunContext *RunContext
	compositeSteps      *compositeSteps
	readAction          readAction
	runAction           runAction
	action              *model.Action
	env                 map[string]string
	actionSource        *ActionSource
	resolvedSha         string
}

var (
	stepActionRemoteNewCloneExecutor = git.NewGitCloneExecutor
)

func (sar *stepActionRemote) prepareActionExecutor() common.Executor {
	return func(ctx context.Context) error {
		if sar.actionSource != nil && sar.action != nil {
			return nil
		}

		resolver := NewActionSourceResolver(ActionSourceResolverConfig{
			ServerURL:                          sar.getGithubContext(ctx).ServerURL,
			Token:                              sar.RunContext.Config.Token,
			Workdir:                            sar.RunContext.Config.Workdir,
			ActionCacheDir:                     sar.RunContext.ActionCacheDir(),
			ReplaceGheActionWithGithubCom:      sar.RunContext.Config.ReplaceGheActionWithGithubCom,
			ReplaceGheActionTokenWithGithubCom: sar.RunContext.Config.ReplaceGheActionTokenWithGithubCom,
			ActionCache:                        sar.RunContext.Config.ActionCache,
			OfflineMode:                        sar.RunContext.Config.ActionOfflineMode,
		})

		actionSource, err := resolver.ResolveAction(ctx, sar.Step.Uses, sar.Step)
		if err != nil {
			return err
		}
		sar.actionSource = actionSource

		github := sar.getGithubContext(ctx)
		if actionSource.IsGheReplacement {
			github.Token = actionSource.Token
		}

		if actionSource.IsCheckout() && isLocalCheckout(github, sar.Step) && !sar.RunContext.Config.NoSkipCheckout {
			common.Logger(ctx).Debugf("Skipping local actions/checkout because workdir was already copied")
			return nil
		}

		if sar.RunContext.Config.ActionCache != nil {
			sar.resolvedSha, err = resolver.Fetch(ctx, actionSource)
			if err != nil {
				return err
			}

			remoteReader := func(ctx context.Context) actionYamlReader {
				return func(filename string) (io.Reader, io.Closer, error) {
					spath := actionSource.ActionMetadataPath(filename)
					for i := 0; i < maxSymlinkDepth; i++ {
						tars, err := resolver.GetTarArchive(ctx, actionSource, spath)
						if err != nil {
							return nil, nil, os.ErrNotExist
						}
						treader := tar.NewReader(tars)
						header, err := treader.Next()
						if err != nil {
							return nil, nil, os.ErrNotExist
						}
						if header.FileInfo().Mode()&os.ModeSymlink == os.ModeSymlink {
							spath, err = symlinkJoin(spath, header.Linkname, ".")
							if err != nil {
								return nil, nil, err
							}
						} else {
							return treader, tars, nil
						}
					}
					return nil, nil, fmt.Errorf("max depth %d of symlinks exceeded while reading %s", maxSymlinkDepth, spath)
				}
			}

			actionModel, err := sar.readAction(ctx, sar.Step, sar.resolvedSha, actionSource.ActionPath(), remoteReader(ctx), os.WriteFile)
			sar.action = actionModel
			return err
		}

		gitClone := stepActionRemoteNewCloneExecutor(git.NewGitCloneExecutorInput{
			URL:         actionSource.CloneURL,
			Ref:         actionSource.Ref,
			Dir:         actionSource.ActionDir(),
			Token:       actionSource.Token,
			OfflineMode: sar.RunContext.Config.ActionOfflineMode,
		})
		var ntErr common.Executor
		if err := gitClone(ctx); err != nil {
			if errors.Is(err, git.ErrShortRef) {
				return fmt.Errorf("Unable to resolve action `%s`, the provided ref `%s` is the shortened version of a commit SHA, which is not supported. Please use the full commit SHA `%s` instead",
					sar.Step.Uses, actionSource.Ref, err.(*git.Error).Commit())
			} else if errors.Is(err, gogit.ErrForceNeeded) {
				ntErr = common.NewInfoExecutor("Non-terminating error while running 'git clone': %v", err)
			} else {
				return err
			}
		}

		remoteReader := func(_ context.Context) actionYamlReader {
			return func(filename string) (io.Reader, io.Closer, error) {
				f, err := os.Open(filepath.Join(actionSource.ActionDir(), actionSource.ActionPath(), filename))
				return f, f, err
			}
		}

		return common.NewPipelineExecutor(
			ntErr,
			func(ctx context.Context) error {
				actionModel, err := sar.readAction(ctx, sar.Step, actionSource.ActionDir(), actionSource.ActionPath(), remoteReader(ctx), os.WriteFile)
				sar.action = actionModel
				return err
			},
		)(ctx)
	}
}

func (sar *stepActionRemote) pre() common.Executor {
	sar.env = map[string]string{}

	return common.NewPipelineExecutor(
		sar.prepareActionExecutor(),
		runStepExecutor(sar, stepStagePre, runPreStep(sar)).If(hasPreStep(sar)).If(shouldRunPreStep(sar)))
}

func (sar *stepActionRemote) main() common.Executor {
	return common.NewPipelineExecutor(
		sar.prepareActionExecutor(),
		runStepExecutor(sar, stepStageMain, func(ctx context.Context) error {
			github := sar.getGithubContext(ctx)
			if sar.actionSource.IsCheckout() && isLocalCheckout(github, sar.Step) && !sar.RunContext.Config.NoSkipCheckout {
				if sar.RunContext.Config.BindWorkdir {
					common.Logger(ctx).Debugf("Skipping local actions/checkout because you bound your workspace")
					return nil
				}
				eval := sar.RunContext.NewExpressionEvaluator(ctx)
				copyToPath := path.Join(sar.RunContext.JobContainer.ToContainerPath(sar.RunContext.Config.Workdir), eval.Interpolate(ctx, sar.Step.With["path"]))
				return sar.RunContext.JobContainer.CopyDir(copyToPath, sar.RunContext.Config.Workdir+string(filepath.Separator)+".", sar.RunContext.Config.UseGitIgnore)(ctx)
			}

			return sar.runAction(sar)(ctx)
		}),
	)
}

func (sar *stepActionRemote) post() common.Executor {
	return runStepExecutor(sar, stepStagePost, runPostStep(sar)).If(hasPostStep(sar)).If(shouldRunPostStep(sar))
}

func (sar *stepActionRemote) getRunContext() *RunContext {
	return sar.RunContext
}

func (sar *stepActionRemote) getGithubContext(ctx context.Context) *model.GithubContext {
	ghc := sar.getRunContext().getGithubContext(ctx)

	if sar.actionSource != nil {
		ghc.ActionRepository = fmt.Sprintf("%s/%s", sar.actionSource.Org, sar.actionSource.Repo)
		ghc.ActionRef = sar.actionSource.Ref
	}

	return ghc
}

func (sar *stepActionRemote) getStepModel() *model.Step {
	return sar.Step
}

func (sar *stepActionRemote) getEnv() *map[string]string {
	return &sar.env
}

func (sar *stepActionRemote) getIfExpression(ctx context.Context, stage stepStage) string {
	switch stage {
	case stepStagePre:
		github := sar.getGithubContext(ctx)
		if sar.actionSource.IsCheckout() && isLocalCheckout(github, sar.Step) && !sar.RunContext.Config.NoSkipCheckout {
			return "false"
		}
		return sar.action.Runs.PreIf
	case stepStageMain:
		return sar.Step.If.Value
	case stepStagePost:
		return sar.action.Runs.PostIf
	}
	return ""
}

func (sar *stepActionRemote) getActionModel() *model.Action {
	return sar.action
}

func (sar *stepActionRemote) getActionSource() *ActionSource {
	return sar.actionSource
}

func (sar *stepActionRemote) getCompositeRunContext(ctx context.Context) *RunContext {
	if sar.compositeRunContext == nil {
		_, containerActionDir := sar.actionSource.ContainerActionPaths(sar.RunContext)

		sar.compositeRunContext = newCompositeRunContext(ctx, sar.RunContext, sar, containerActionDir)
		sar.compositeSteps = sar.compositeRunContext.compositeExecutor(sar.action)
	} else {
		env := evaluateCompositeInputAndEnv(ctx, sar.RunContext, sar)
		sar.compositeRunContext.Env = env
		sar.compositeRunContext.ExtraPath = sar.RunContext.ExtraPath
	}
	return sar.compositeRunContext
}

func (sar *stepActionRemote) getCompositeSteps() *compositeSteps {
	return sar.compositeSteps
}

type remoteAction struct {
	URL  string
	Org  string
	Repo string
	Path string
	Ref  string
}

func (ra *remoteAction) CloneURL() string {
	return fmt.Sprintf("%s/%s/%s", ra.URL, ra.Org, ra.Repo)
}

func (ra *remoteAction) IsCheckout() bool {
	if ra.Org == "actions" && ra.Repo == "checkout" {
		return true
	}
	return false
}

func newRemoteAction(action string) *remoteAction {
	// GitHub's document[^] describes:
	// > We strongly recommend that you include the version of
	// > the action you are using by specifying a Git ref, SHA, or Docker tag number.
	// Actually, the workflow stops if there is the uses directive that hasn't @ref.
	// [^]: https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions
	r := regexp.MustCompile(`^([^/@]+)/([^/@]+)(/([^@]*))?(@(.*))?$`)
	matches := r.FindStringSubmatch(action)
	if len(matches) < 7 || matches[6] == "" {
		return nil
	}
	return &remoteAction{
		Org:  matches[1],
		Repo: matches[2],
		Path: matches[4],
		Ref:  matches[6],
		URL:  "https://github.com",
	}
}

func safeFilename(s string) string {
	return strings.NewReplacer(
		`<`, "-",
		`>`, "-",
		`:`, "-",
		`"`, "-",
		`/`, "-",
		`\`, "-",
		`|`, "-",
		`?`, "-",
		`*`, "-",
	).Replace(s)
}
