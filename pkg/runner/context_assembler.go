package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/container"
	"github.com/nektos/act/pkg/exprparser"
	"github.com/nektos/act/pkg/model"
	"gopkg.in/yaml.v3"
)

type ContextSnapshot struct {
	Env      map[string]string
	Vars     map[string]string
	Secrets  map[string]string
	Inputs   map[string]interface{}
	Matrix   map[string]interface{}
	Github   *model.GithubContext
	Job      *model.JobContext
	Steps    map[string]*model.StepResult
	Runner   map[string]interface{}
	Strategy map[string]interface{}
	Needs    map[string]exprparser.Needs
	Jobs     *map[string]*model.WorkflowCallResult
}

func (s *ContextSnapshot) Clone() *ContextSnapshot {
	return &ContextSnapshot{
		Env:      cloneMap(s.Env),
		Vars:     cloneMap(s.Vars),
		Secrets:  cloneMap(s.Secrets),
		Inputs:   cloneInterfaceMap(s.Inputs),
		Matrix:   cloneInterfaceMap(s.Matrix),
		Github:   s.Github,
		Job:      s.Job,
		Steps:    cloneStepResults(s.Steps),
		Runner:   s.Runner,
		Strategy: cloneInterfaceMap(s.Strategy),
		Needs:    s.Needs,
		Jobs:     s.Jobs,
	}
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func cloneInterfaceMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func cloneStepResults(m map[string]*model.StepResult) map[string]*model.StepResult {
	if m == nil {
		return nil
	}
	result := make(map[string]*model.StepResult, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

type JobStateWriter interface {
	SetStepResult(stepID string, result *model.StepResult)
	SetJobOutput(name, value string)
	SetJobResult(result string)
	SetCancelled(cancelled bool)
	SetCurrentStep(stepID string)
	SetActionPath(path string)
	AddEnv(name, value string)
	SetEnv(name, value string)
	MergeEnv(env map[string]string)
	AddExtraPath(path string)
	SetExtraPath(paths []string)
	SetIntraActionState(stepID, name, value string)
	GetIntraActionState(stepID, name string) (string, bool)
	MergeGlobalEnv(env map[string]string)
	SetCallerJobOutputs(ctx context.Context, exprEval ExpressionEvaluator)
}

type ContextSnapshotReader interface {
	GetSnapshot() *ContextSnapshot
}

type ContextSnapshotProvider interface {
	CreateSnapshot(ctx context.Context) *ContextSnapshot
}

type SnapshotBasedEvaluator interface {
	UpdateSnapshot(snapshot *ContextSnapshot)
}

type ContextDataSource interface {
	GetConfig() *Config
	GetRun() *model.Run
	GetRawMatrix() map[string]interface{}
	GetEventJSON() string
	GetRawEnv() map[string]string
	GetRawStepResults() map[string]*model.StepResult
	GetJobContainer() container.ExecutionsEnvironment
	GetCurrentStep() string
	GetActionPath() string
	GetCancelled() bool
	GetCaller() *caller
	GetExprEval() ExpressionEvaluator
	GetStateVersion() uint64
	IncrementStateVersion()
}

type EnvSnapshotBuilder struct {
	source ContextDataSource
}

func NewEnvSnapshotBuilder(source ContextDataSource) *EnvSnapshotBuilder {
	return &EnvSnapshotBuilder{source: source}
}

func (b *EnvSnapshotBuilder) Build(ctx context.Context, github *model.GithubContext) map[string]string {
	env := map[string]string{}
	config := b.source.GetConfig()
	run := b.source.GetRun()

	if run != nil && run.Workflow != nil && config != nil {
		job := run.Job()
		if job != nil {
			env = mergeMaps(run.Workflow.Env, job.Environment(), config.Env)
		}
	}
	env["ACT"] = "true"

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

	if config.ArtifactServerPath != "" {
		b.SetActionRuntimeVars(env)
	}

	if run != nil {
		job := run.Job()
		if job != nil && job.RunsOn() != nil {
			for _, platformName := range job.RunsOn() {
				if platformName != "" {
					if platformName == "ubuntu-latest" {
						env["ImageOS"] = "ubuntu20"
					} else {
						platformName = strings.SplitN(strings.Replace(platformName, `-`, ``, 1), `.`, 2)[0]
						env["ImageOS"] = platformName
					}
				}
			}
		}
	}

	return env
}

func (b *EnvSnapshotBuilder) SetActionRuntimeVars(env map[string]string) {
	config := b.source.GetConfig()
	actionsRuntimeURL := os.Getenv("ACTIONS_RUNTIME_URL")
	if actionsRuntimeURL == "" {
		actionsRuntimeURL = fmt.Sprintf("http://%s:%s/", config.ArtifactServerAddr, config.ArtifactServerPort)
	}
	env["ACTIONS_RUNTIME_URL"] = actionsRuntimeURL
	env["ACTIONS_RESULTS_URL"] = actionsRuntimeURL

	actionsRuntimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	if actionsRuntimeToken == "" {
		runID := int64(1)
		if rid, ok := config.Env["GITHUB_RUN_ID"]; ok {
			runID, _ = strconv.ParseInt(rid, 10, 64)
		}
		actionsRuntimeToken, _ = common.CreateAuthorizationToken(runID, runID, runID)
	}
	env["ACTIONS_RUNTIME_TOKEN"] = actionsRuntimeToken
}

type GithubSnapshotBuilder struct {
	source ContextDataSource
}

func NewGithubSnapshotBuilder(source ContextDataSource) *GithubSnapshotBuilder {
	return &GithubSnapshotBuilder{source: source}
}

func (b *GithubSnapshotBuilder) Build(ctx context.Context) *model.GithubContext {
	logger := common.Logger(ctx)
	config := b.source.GetConfig()
	run := b.source.GetRun()
	env := b.source.GetRawEnv()

	ghc := &model.GithubContext{
		Event:            make(map[string]interface{}),
		Workflow:         run.Workflow.Name,
		RunAttempt:       config.Env["GITHUB_RUN_ATTEMPT"],
		RunID:            config.Env["GITHUB_RUN_ID"],
		RunNumber:        config.Env["GITHUB_RUN_NUMBER"],
		Actor:            config.Actor,
		EventName:        config.EventName,
		Action:           b.source.GetCurrentStep(),
		Token:            config.Token,
		Job:              run.JobID,
		ActionPath:       b.source.GetActionPath(),
		ActionRepository: env["GITHUB_ACTION_REPOSITORY"],
		ActionRef:        env["GITHUB_ACTION_REF"],
		RepositoryOwner:  config.Env["GITHUB_REPOSITORY_OWNER"],
		RetentionDays:    config.Env["GITHUB_RETENTION_DAYS"],
		RunnerPerflog:    config.Env["RUNNER_PERFLOG"],
		RunnerTrackingID: config.Env["RUNNER_TRACKING_ID"],
		Repository:       config.Env["GITHUB_REPOSITORY"],
		Ref:              config.Env["GITHUB_REF"],
		Sha:              config.Env["SHA_REF"],
		RefName:          config.Env["GITHUB_REF_NAME"],
		RefType:          config.Env["GITHUB_REF_TYPE"],
		BaseRef:          config.Env["GITHUB_BASE_REF"],
		HeadRef:          config.Env["GITHUB_HEAD_REF"],
		Workspace:        config.Env["GITHUB_WORKSPACE"],
	}

	jobContainer := b.source.GetJobContainer()
	if jobContainer != nil {
		ghc.EventPath = jobContainer.GetActPath() + "/workflow/event.json"
		ghc.Workspace = jobContainer.ToContainerPath(config.Workdir)
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
	if ghc.Actor == "" {
		ghc.Actor = "nektos/act"
	}

	eventJSON := b.source.GetEventJSON()
	if eventJSON != "" {
		err := json.Unmarshal([]byte(eventJSON), &ghc.Event)
		if err != nil {
			logger.Errorf("Unable to Unmarshal event '%s': %v", eventJSON, err)
		}
	}

	ghc.SetBaseAndHeadRef()
	repoPath := config.Workdir
	ghc.SetRepositoryAndOwner(ctx, config.GitHubInstance, config.RemoteName, repoPath)
	if ghc.Ref == "" {
		ghc.SetRef(ctx, config.DefaultBranch, repoPath)
	}
	if ghc.Sha == "" {
		ghc.SetSha(ctx, repoPath)
	}
	ghc.SetRefTypeAndName()

	ghc.ServerURL = "https://github.com"
	ghc.APIURL = "https://api.github.com"
	ghc.GraphQLURL = "https://api.github.com/graphql"
	if config.GitHubInstance != "github.com" {
		ghc.ServerURL = fmt.Sprintf("https://%s", config.GitHubInstance)
		ghc.APIURL = fmt.Sprintf("https://%s/api/v3", config.GitHubInstance)
		ghc.GraphQLURL = fmt.Sprintf("https://%s/api/graphql", config.GitHubInstance)
	}
	if config.Env["GITHUB_SERVER_URL"] != "" {
		ghc.ServerURL = config.Env["GITHUB_SERVER_URL"]
	}
	if config.Env["GITHUB_API_URL"] != "" {
		ghc.APIURL = config.Env["GITHUB_API_URL"]
	}
	if config.Env["GITHUB_GRAPHQL_URL"] != "" {
		ghc.GraphQLURL = config.Env["GITHUB_GRAPHQL_URL"]
	}

	return ghc
}

type SecretsSnapshotBuilder struct {
	source ContextDataSource
}

func NewSecretsSnapshotBuilder(source ContextDataSource) *SecretsSnapshotBuilder {
	return &SecretsSnapshotBuilder{source: source}
}

func (b *SecretsSnapshotBuilder) Build(ctx context.Context) map[string]string {
	caller := b.source.GetCaller()
	if caller != nil {
		job := caller.runContext.Run.Job()
		secrets := job.Secrets()

		if secrets == nil && job.InheritSecrets() {
			secrets = caller.runContext.Config.Secrets
		}

		if secrets == nil {
			secrets = map[string]string{}
		}

		result := make(map[string]string, len(secrets))
		for k, v := range secrets {
			result[k] = caller.runContext.ExprEval.Interpolate(ctx, v)
		}

		return result
	}

	return cloneMap(b.source.GetConfig().Secrets)
}

type VarsSnapshotBuilder struct {
	source ContextDataSource
}

func NewVarsSnapshotBuilder(source ContextDataSource) *VarsSnapshotBuilder {
	return &VarsSnapshotBuilder{source: source}
}

func (b *VarsSnapshotBuilder) Build() map[string]string {
	return cloneMap(b.source.GetConfig().Vars)
}

type InputsSnapshotBuilder struct {
	source ContextDataSource
}

func NewInputsSnapshotBuilder(source ContextDataSource) *InputsSnapshotBuilder {
	return &InputsSnapshotBuilder{source: source}
}

func (b *InputsSnapshotBuilder) Build(ctx context.Context, stepEnv map[string]string, ghc *model.GithubContext) map[string]interface{} {
	inputs := map[string]interface{}{}

	b.setupWorkflowInputs(ctx, &inputs)

	var env map[string]string
	if stepEnv != nil {
		env = stepEnv
	} else {
		env = b.source.GetRawEnv()
	}

	for k, v := range env {
		if strings.HasPrefix(k, "INPUT_") {
			inputs[strings.ToLower(strings.TrimPrefix(k, "INPUT_"))] = v
		}
	}

	caller := b.source.GetCaller()
	if caller == nil && ghc.EventName == "workflow_dispatch" {
		run := b.source.GetRun()
		config := run.Workflow.WorkflowDispatchConfig()
		if config != nil && config.Inputs != nil {
			for k, v := range config.Inputs {
				value := nestedMapLookup(ghc.Event, "inputs", k)
				if value == nil {
					value = v.Default
				}
				if v.Type == "boolean" {
					inputs[k] = value == "true"
				} else {
					inputs[k] = value
				}
			}
		}
	}

	if ghc.EventName == "workflow_call" {
		run := b.source.GetRun()
		config := run.Workflow.WorkflowCallConfig()
		if config != nil && config.Inputs != nil {
			for k, v := range config.Inputs {
				value := nestedMapLookup(ghc.Event, "inputs", k)
				if value == nil {
					if err := v.Default.Decode(&value); err != nil {
						common.Logger(ctx).Debugf("error decoding default value for %s: %v", k, err)
					}
				}
				if v.Type == "boolean" {
					inputs[k] = value == "true"
				} else {
					inputs[k] = value
				}
			}
		}
	}
	return inputs
}

func (b *InputsSnapshotBuilder) setupWorkflowInputs(ctx context.Context, inputs *map[string]interface{}) {
	caller := b.source.GetCaller()
	if caller != nil {
		run := b.source.GetRun()
		config := run.Workflow.WorkflowCallConfig()

		for name, input := range config.Inputs {
			value := caller.runContext.Run.Job().With[name]

			if value != nil {
				node := yaml.Node{}
				_ = node.Encode(value)
				if caller.runContext.ExprEval != nil {
					_ = caller.runContext.ExprEval.EvaluateYamlNode(ctx, &node)
				}
				_ = node.Decode(&value)
			}

			if value == nil && config != nil && config.Inputs != nil {
				def := input.Default
				exprEval := b.source.GetExprEval()
				if exprEval != nil {
					_ = exprEval.EvaluateYamlNode(ctx, &def)
				}
				_ = def.Decode(&value)
			}

			(*inputs)[name] = value
		}
	}
}

type MatrixSnapshotBuilder struct {
	source ContextDataSource
}

func NewMatrixSnapshotBuilder(source ContextDataSource) *MatrixSnapshotBuilder {
	return &MatrixSnapshotBuilder{source: source}
}

func (b *MatrixSnapshotBuilder) Build() map[string]interface{} {
	return cloneInterfaceMap(b.source.GetRawMatrix())
}

type JobContextSnapshotBuilder struct {
	source ContextDataSource
}

func NewJobContextSnapshotBuilder(source ContextDataSource) *JobContextSnapshotBuilder {
	return &JobContextSnapshotBuilder{source: source}
}

func (b *JobContextSnapshotBuilder) Build() *model.JobContext {
	jobStatus := "success"
	if b.source.GetCancelled() {
		jobStatus = "cancelled"
	} else {
		for _, stepStatus := range b.source.GetRawStepResults() {
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

type StepsSnapshotBuilder struct {
	source ContextDataSource
}

func NewStepsSnapshotBuilder(source ContextDataSource) *StepsSnapshotBuilder {
	return &StepsSnapshotBuilder{source: source}
}

func (b *StepsSnapshotBuilder) Build() map[string]*model.StepResult {
	return cloneStepResults(b.source.GetRawStepResults())
}

type ContextSnapshotFactory struct {
	source            ContextDataSource
	envBuilder        *EnvSnapshotBuilder
	githubBuilder     *GithubSnapshotBuilder
	secretsBuilder    *SecretsSnapshotBuilder
	varsBuilder       *VarsSnapshotBuilder
	inputsBuilder     *InputsSnapshotBuilder
	matrixBuilder     *MatrixSnapshotBuilder
	jobContextBuilder *JobContextSnapshotBuilder
	stepsBuilder      *StepsSnapshotBuilder
}

func NewContextSnapshotFactory(source ContextDataSource) *ContextSnapshotFactory {
	return &ContextSnapshotFactory{
		source:            source,
		envBuilder:        NewEnvSnapshotBuilder(source),
		githubBuilder:     NewGithubSnapshotBuilder(source),
		secretsBuilder:    NewSecretsSnapshotBuilder(source),
		varsBuilder:       NewVarsSnapshotBuilder(source),
		inputsBuilder:     NewInputsSnapshotBuilder(source),
		matrixBuilder:     NewMatrixSnapshotBuilder(source),
		jobContextBuilder: NewJobContextSnapshotBuilder(source),
		stepsBuilder:      NewStepsSnapshotBuilder(source),
	}
}

func (f *ContextSnapshotFactory) CreateSnapshot(ctx context.Context) *ContextSnapshot {
	github := f.githubBuilder.Build(ctx)
	env := f.envBuilder.Build(ctx, github)
	secrets := f.secretsBuilder.Build(ctx)
	vars := f.varsBuilder.Build()
	inputs := f.inputsBuilder.Build(ctx, nil, github)
	matrix := f.matrixBuilder.Build()
	job := f.jobContextBuilder.Build()
	steps := f.stepsBuilder.Build()

	strategy := make(map[string]interface{})
	run := f.source.GetRun()
	if run != nil {
		jobModel := run.Job()
		if jobModel != nil && jobModel.Strategy != nil {
			strategy["fail-fast"] = jobModel.Strategy.FailFast
			strategy["max-parallel"] = jobModel.Strategy.MaxParallel
		}
	}

	var needs map[string]exprparser.Needs
	var jobs *map[string]*model.WorkflowCallResult
	if run != nil {
		jobsMap := run.Workflow.Jobs
		job := run.Job()

		if job != nil {
			jobNeeds := job.Needs()

			needs = make(map[string]exprparser.Needs)
			for _, need := range jobNeeds {
				needs[need] = exprparser.Needs{
					Outputs: jobsMap[need].Outputs,
					Result:  jobsMap[need].Result,
				}
			}
		}

		caller := f.source.GetCaller()
		if caller != nil && f.source.GetExprEval() != nil && job != nil {
			workflowCallResult := map[string]*model.WorkflowCallResult{}
			for jobName, job := range jobsMap {
				result := model.WorkflowCallResult{
					Outputs: map[string]string{},
				}
				for k, v := range job.Outputs {
					result.Outputs[k] = v
				}
				workflowCallResult[jobName] = &result
			}
			jobs = &workflowCallResult
		}
	}

	var runner map[string]interface{}
	jobContainer := f.source.GetJobContainer()
	if jobContainer != nil {
		runner = jobContainer.GetRunnerContext(ctx)
	}

	return &ContextSnapshot{
		Env:      env,
		Vars:     vars,
		Secrets:  secrets,
		Inputs:   inputs,
		Matrix:   matrix,
		Github:   github,
		Job:      job,
		Steps:    steps,
		Runner:   runner,
		Strategy: strategy,
		Needs:    needs,
		Jobs:     jobs,
	}
}

func (f *ContextSnapshotFactory) CreateStepSnapshot(ctx context.Context, step step) *ContextSnapshot {
	baseSnapshot := f.CreateSnapshot(ctx)
	stepSnapshot := baseSnapshot.Clone()
	stepSnapshot.Env = *step.getEnv()

	ghc := *stepSnapshot.Github
	if sar, ok := step.(*stepActionRemote); ok && sar.remoteAction != nil {
		ghc.ActionRepository = fmt.Sprintf("%s/%s", sar.remoteAction.Org, sar.remoteAction.Repo)
		ghc.ActionRef = sar.remoteAction.Ref
	}
	stepSnapshot.Github = &ghc

	stepSnapshot.Inputs = f.inputsBuilder.Build(ctx, *step.getEnv(), stepSnapshot.Github)
	return stepSnapshot
}

func (f *ContextSnapshotFactory) EnvBuilder() *EnvSnapshotBuilder {
	return f.envBuilder
}

type RunContextStateWriter struct {
	rc *RunContext
}

func NewRunContextStateWriter(rc *RunContext) *RunContextStateWriter {
	return &RunContextStateWriter{rc: rc}
}

func (w *RunContextStateWriter) SetStepResult(stepID string, result *model.StepResult) {
	w.rc.StepResults[stepID] = result
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetJobOutput(name, value string) {
	w.rc.Run.Job().Outputs[name] = value
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetJobResult(result string) {
	w.rc.Run.Job().Result = result
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetCancelled(cancelled bool) {
	w.rc.Cancelled = cancelled
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetCurrentStep(stepID string) {
	w.rc.CurrentStep = stepID
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetActionPath(path string) {
	w.rc.ActionPath = path
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) AddEnv(name, value string) {
	w.rc.Env[name] = value
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetEnv(name, value string) {
	w.rc.Env[name] = value
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) MergeEnv(env map[string]string) {
	mergeIntoMap := mergeIntoMapCaseSensitive
	if w.rc.JobContainer != nil && w.rc.JobContainer.IsEnvironmentCaseInsensitive() {
		mergeIntoMap = mergeIntoMapCaseInsensitive
	}
	mergeIntoMap(w.rc.Env, env)
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) AddExtraPath(path string) {
	extraPath := []string{path}
	for _, v := range w.rc.ExtraPath {
		if v != path {
			extraPath = append(extraPath, v)
		}
	}
	w.rc.ExtraPath = extraPath
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetExtraPath(paths []string) {
	w.rc.ExtraPath = paths
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetIntraActionState(stepID, name, value string) {
	if w.rc.IntraActionState == nil {
		w.rc.IntraActionState = map[string]map[string]string{}
	}
	state, ok := w.rc.IntraActionState[stepID]
	if !ok {
		state = map[string]string{}
		w.rc.IntraActionState[stepID] = state
	}
	state[name] = value
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) GetIntraActionState(stepID, name string) (string, bool) {
	if w.rc.IntraActionState == nil {
		return "", false
	}
	state, ok := w.rc.IntraActionState[stepID]
	if !ok {
		return "", false
	}
	value, ok := state[name]
	return value, ok
}

func (w *RunContextStateWriter) MergeGlobalEnv(env map[string]string) {
	if w.rc.GlobalEnv == nil {
		w.rc.GlobalEnv = map[string]string{}
	}
	mergeIntoMap := mergeIntoMapCaseSensitive
	if w.rc.JobContainer != nil && w.rc.JobContainer.IsEnvironmentCaseInsensitive() {
		mergeIntoMap = mergeIntoMapCaseInsensitive
	}
	mergeIntoMap(w.rc.GlobalEnv, env)
	w.rc.IncrementStateVersion()
}

func (w *RunContextStateWriter) SetCallerJobOutputs(ctx context.Context, exprEval ExpressionEvaluator) {
	if w.rc.caller != nil {
		callerOutputs := make(map[string]string)
		for k, v := range w.rc.Run.Workflow.WorkflowCallConfig().Outputs {
			callerOutputs[k] = exprEval.Interpolate(ctx, exprEval.Interpolate(ctx, v.Value))
		}
		w.rc.caller.runContext.Run.Job().Outputs = callerOutputs
	}
}

type ContextAssembler struct {
	snapshotFactory *ContextSnapshotFactory
	stateWriter     *RunContextStateWriter
}

func NewContextAssembler(rc *RunContext) *ContextAssembler {
	return &ContextAssembler{
		snapshotFactory: NewContextSnapshotFactory(rc),
		stateWriter:     NewRunContextStateWriter(rc),
	}
}

func (ca *ContextAssembler) CreateSnapshot(ctx context.Context) *ContextSnapshot {
	return ca.snapshotFactory.CreateSnapshot(ctx)
}

func (ca *ContextAssembler) CreateStepSnapshot(ctx context.Context, step step) *ContextSnapshot {
	return ca.snapshotFactory.CreateStepSnapshot(ctx, step)
}

func (ca *ContextAssembler) StateWriter() *RunContextStateWriter {
	return ca.stateWriter
}

func (ca *ContextAssembler) EnvBuilder() *EnvSnapshotBuilder {
	return ca.snapshotFactory.envBuilder
}

func (ca *ContextAssembler) SnapshotFactory() *ContextSnapshotFactory {
	return ca.snapshotFactory
}
