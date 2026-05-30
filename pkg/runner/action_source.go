package runner

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/model"
)

type ActionSourceType string

const (
	ActionSourceTypeLocal                ActionSourceType = "local"
	ActionSourceTypeRemoteAction         ActionSourceType = "remote-action"
	ActionSourceTypeRemoteWorkflow       ActionSourceType = "remote-workflow"
	ActionSourceTypeLocalReusableWorkflow ActionSourceType = "local-reusable-workflow"
)

type ActionSource struct {
	Type             ActionSourceType
	Uses             string
	URL              string
	Org              string
	Repo             string
	Path             string
	Ref              string
	Filename         string
	CacheDir         string
	CloneURL         string
	Token            string
	ExecutionDir     string
	IsGheReplacement bool
	OfflineMode      bool
	Workdir          string
	ActionCacheDir   string
	resolvedSha      string
}

func (as *ActionSource) IsCheckout() bool {
	return as.Org == "actions" && as.Repo == "checkout"
}

func (as *ActionSource) IsLocalAction() bool {
	return as.Type == ActionSourceTypeLocal
}

func (as *ActionSource) IsRemoteAction() bool {
	return as.Type == ActionSourceTypeRemoteAction
}

func (as *ActionSource) ResolvedSha() string {
	return as.resolvedSha
}

func (as *ActionSource) SetResolvedSha(sha string) {
	as.resolvedSha = sha
}

func (as *ActionSource) ActionDir() string {
	return as.ExecutionDir
}

func (as *ActionSource) ActionPath() string {
	return as.Path
}

func (as *ActionSource) FullActionPath() string {
	if as.Path == "" {
		return as.ExecutionDir
	}
	return path.Join(as.ExecutionDir, as.Path)
}

func (as *ActionSource) ActionMetadataPath(filename string) string {
	return path.Join(as.Path, filename)
}

func (as *ActionSource) DockerBuildContextDir(action *model.Action) string {
	if action.Runs.Image == "" || strings.HasPrefix(action.Runs.Image, "docker://") {
		return ""
	}
	contextDir, _ := as.DockerBuildContextAndFile(action)
	return contextDir
}

func (as *ActionSource) DockerfilePath(action *model.Action) string {
	if action.Runs.Image == "" || strings.HasPrefix(action.Runs.Image, "docker://") {
		return ""
	}
	_, fileName := as.DockerBuildContextAndFile(action)
	return fileName
}

func (as *ActionSource) DockerBuildContextAndFile(action *model.Action) (contextDir string, dockerfile string) {
	imageSubpath := action.Runs.Image
	if as.IsLocalAction() {
		contextDir, dockerfile = path.Split(path.Join(as.Uses, imageSubpath))
	} else {
		contextDir, dockerfile = path.Split(path.Join(as.Path, imageSubpath))
	}
	return contextDir, dockerfile
}

func (as *ActionSource) DockerImageName(actionName string) string {
	image := fmt.Sprintf("%s-dockeraction:%s", regexp.MustCompile("[^a-zA-Z0-9]").ReplaceAllString(actionName, "-"), "latest")
	image = fmt.Sprintf("act-%s", strings.TrimLeft(image, "-"))
	return strings.ToLower(image)
}

func (as *ActionSource) ContainerActionPaths(rc *RunContext) (actionName string, containerActionDir string) {
	actionLocation := as.FullActionPath()
	stepType := as.stepType()

	actionName = ""
	containerActionDir = "."

	if stepType != model.StepTypeUsesActionRemote {
		actionName = getOsSafeRelativePath(actionLocation, rc.Config.Workdir)
		containerActionDir = rc.JobContainer.ToContainerPath(rc.Config.Workdir) + "/" + actionName
		actionName = "./" + actionName
	} else if stepType == model.StepTypeUsesActionRemote {
		actionName = getOsSafeRelativePath(actionLocation, rc.ActionCacheDir())
		containerActionDir = rc.JobContainer.GetActPath() + "/actions/" + actionName
	}

	if actionName == "" {
		actionName = filepath.Base(actionLocation)
		if runtime.GOOS == "windows" {
			actionName = strings.ReplaceAll(actionName, "\\", "/")
		}
	}
	return actionName, containerActionDir
}

func (as *ActionSource) stepType() model.StepType {
	switch as.Type {
	case ActionSourceTypeLocal, ActionSourceTypeLocalReusableWorkflow:
		return model.StepTypeUsesActionLocal
	case ActionSourceTypeRemoteAction, ActionSourceTypeRemoteWorkflow:
		return model.StepTypeUsesActionRemote
	default:
		return model.StepTypeInvalid
	}
}

func (as *ActionSource) WorkflowFilePath() string {
	if as.Type == ActionSourceTypeRemoteWorkflow {
		return fmt.Sprintf(".github/workflows/%s", as.Filename)
	}
	return as.Uses
}

func (as *ActionSource) formatCacheError(format string, args ...any) error {
	prefix := as.errorPrefix()
	return fmt.Errorf(prefix+format, args...)
}

func (as *ActionSource) errorPrefix() string {
	if as.IsGheReplacement {
		prefix := fmt.Sprintf("GHE-replaced action %q from %s (original: %s/%s): ", as.Uses, as.URL, as.Org, as.Repo)
		if as.OfflineMode {
			prefix += "(offline mode) "
		}
		return prefix
	}
	if as.OfflineMode {
		return fmt.Sprintf("action %q (offline mode): ", as.Uses)
	}
	return ""
}

type ActionSourceResolver interface {
	ResolveAction(ctx context.Context, uses string, step *model.Step) (*ActionSource, error)
	ResolveWorkflow(ctx context.Context, uses string) (*ActionSource, error)
	Fetch(ctx context.Context, source *ActionSource) (string, error)
	GetTarArchive(ctx context.Context, source *ActionSource, includePrefix string) (io.ReadCloser, error)
}

type ActionSourceResolverConfig struct {
	ServerURL                          string
	Token                              string
	Workdir                            string
	ActionCacheDir                     string
	ReplaceGheActionWithGithubCom      []string
	ReplaceGheActionTokenWithGithubCom string
	ActionCache                        ActionCache
	OfflineMode                        bool
}

type actionSourceResolver struct {
	config ActionSourceResolverConfig
}

func NewActionSourceResolver(config ActionSourceResolverConfig) ActionSourceResolver {
	return &actionSourceResolver{
		config: config,
	}
}

func (r *actionSourceResolver) ResolveAction(ctx context.Context, uses string, step *model.Step) (*ActionSource, error) {
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") || !strings.Contains(uses, "/") {
		return r.resolveLocalAction(uses), nil
	}

	remoteAction := parseRemoteAction(uses)
	if remoteAction == nil {
		return nil, fmt.Errorf("Expected format {org}/{repo}[/path]@ref. Actual '%s' Input string was not in a correct format", uses)
	}

	source := &ActionSource{
		Type:           ActionSourceTypeRemoteAction,
		Uses:           uses,
		URL:            r.config.ServerURL,
		Org:            remoteAction.Org,
		Repo:           remoteAction.Repo,
		Path:           remoteAction.Path,
		Ref:            remoteAction.Ref,
		CacheDir:       fmt.Sprintf("%s/%s", remoteAction.Org, remoteAction.Repo),
		Token:          r.config.Token,
		ExecutionDir:   fmt.Sprintf("%s/%s", r.config.ActionCacheDir, safeFilename(uses)),
		OfflineMode:    r.config.OfflineMode,
		Workdir:        r.config.Workdir,
		ActionCacheDir: r.config.ActionCacheDir,
	}

	r.applyGheReplacement(source)
	source.CloneURL = fmt.Sprintf("%s/%s/%s", source.URL, source.Org, source.Repo)

	return source, nil
}

func (r *actionSourceResolver) ResolveWorkflow(ctx context.Context, uses string) (*ActionSource, error) {
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") {
		return r.resolveLocalWorkflow(uses), nil
	}

	remoteWorkflow := parseRemoteReusableWorkflow(uses)
	if remoteWorkflow == nil {
		return nil, fmt.Errorf("Expected format {owner}/{repo}/.github/workflows/{filename}@{ref}. Actual '%s' Input string was not in a correct format", uses)
	}

	source := &ActionSource{
		Type:           ActionSourceTypeRemoteWorkflow,
		Uses:           uses,
		URL:            r.config.ServerURL,
		Org:            remoteWorkflow.Org,
		Repo:           remoteWorkflow.Repo,
		Filename:       remoteWorkflow.Filename,
		Ref:            remoteWorkflow.Ref,
		CacheDir:       fmt.Sprintf("%s/%s@%s", remoteWorkflow.Org, remoteWorkflow.Repo, remoteWorkflow.Ref),
		Token:          r.config.Token,
		OfflineMode:    r.config.OfflineMode,
		Workdir:        r.config.Workdir,
		ActionCacheDir: r.config.ActionCacheDir,
	}

	r.applyGheReplacement(source)
	source.CloneURL = fmt.Sprintf("%s/%s/%s", source.URL, source.Org, source.Repo)
	source.ExecutionDir = fmt.Sprintf("%s/%s", r.config.ActionCacheDir, safeFilename(source.CacheDir))

	return source, nil
}

func (r *actionSourceResolver) Fetch(ctx context.Context, source *ActionSource) (string, error) {
	if r.config.ActionCache == nil {
		return "", fmt.Errorf("action cache not configured for fetching %s", source.Uses)
	}

	logger := common.Logger(ctx)
	logger.Debugf("Fetching action source %s (cacheDir: %s, url: %s, ref: %s)",
		source.Uses, source.CacheDir, source.CloneURL, source.Ref)

	sha, err := r.config.ActionCache.Fetch(ctx, source)
	if err != nil {
		return "", err
	}

	source.resolvedSha = sha
	logger.Debugf("Successfully fetched action source %s, resolved to sha %s", source.Uses, sha)
	return sha, nil
}

func (r *actionSourceResolver) GetTarArchive(ctx context.Context, source *ActionSource, includePrefix string) (io.ReadCloser, error) {
	if r.config.ActionCache == nil {
		return nil, fmt.Errorf("action cache not configured for reading %s", source.Uses)
	}

	if source.resolvedSha == "" {
		return nil, fmt.Errorf("cannot get tar archive for %s: not yet fetched", source.Uses)
	}

	archive, err := r.config.ActionCache.GetTarArchive(ctx, source, source.resolvedSha, includePrefix)
	if err != nil {
		return nil, err
	}

	return archive, nil
}

func (r *actionSourceResolver) resolveLocalAction(uses string) *ActionSource {
	return &ActionSource{
		Type:           ActionSourceTypeLocal,
		Uses:           uses,
		ExecutionDir:   fmt.Sprintf("%s/%s", r.config.Workdir, uses),
		Workdir:        r.config.Workdir,
		ActionCacheDir: r.config.ActionCacheDir,
	}
}

func (r *actionSourceResolver) resolveLocalWorkflow(uses string) *ActionSource {
	return &ActionSource{
		Type:           ActionSourceTypeLocalReusableWorkflow,
		Uses:           uses,
		ExecutionDir:   r.config.Workdir,
		Workdir:        r.config.Workdir,
		ActionCacheDir: r.config.ActionCacheDir,
	}
}

func (r *actionSourceResolver) applyGheReplacement(source *ActionSource) {
	fullName := fmt.Sprintf("%s/%s", source.Org, source.Repo)
	for _, action := range r.config.ReplaceGheActionWithGithubCom {
		if strings.EqualFold(fullName, action) {
			source.URL = "https://github.com"
			source.Token = r.config.ReplaceGheActionTokenWithGithubCom
			source.IsGheReplacement = true
			break
		}
	}
}

func parseRemoteAction(action string) *remoteAction {
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

func parseRemoteReusableWorkflow(uses string) *remoteReusableWorkflow {
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
