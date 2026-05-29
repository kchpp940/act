package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	docker_container "github.com/moby/moby/api/types/container"
	"github.com/nektos/act/pkg/model"
	log "github.com/sirupsen/logrus"
)

type EnvSource struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Source string `json:"source"`
}

type SecretSource struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Set    bool   `json:"set"`
}

type VarSource struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Source string `json:"source"`
}

type StepPreview struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Uses     string `json:"uses,omitempty"`
	Run      string `json:"run,omitempty"`
	CacheHit *bool  `json:"cache_hit,omitempty"`
}

type ServicePreview struct {
	ID      string            `json:"id"`
	Image   string            `json:"image"`
	Env     map[string]string `json:"env,omitempty"`
	Ports   []string          `json:"ports,omitempty"`
	Volumes []string          `json:"volumes,omitempty"`
	Options string            `json:"options,omitempty"`
}

type ContainerPreview struct {
	Image       string            `json:"image"`
	Env         map[string]string `json:"env,omitempty"`
	Ports       []string          `json:"ports,omitempty"`
	Volumes     []string          `json:"volumes,omitempty"`
	Options     string            `json:"options,omitempty"`
	Credentials bool              `json:"credentials,omitempty"`
}

type MatrixPreview struct {
	Key    string        `json:"key"`
	Values []interface{} `json:"values"`
}

type JobPreview struct {
	JobID         string            `json:"job_id"`
	JobName       string            `json:"job_name"`
	Stage         int               `json:"stage"`
	WorkflowName  string            `json:"workflow_name"`
	WorkflowFile  string            `json:"workflow_file"`
	Events        []string          `json:"events"`
	RunsOn        []string          `json:"runs_on"`
	PlatformImage string            `json:"platform_image,omitempty"`
	Container     *ContainerPreview `json:"container,omitempty"`
	NetworkMode   string            `json:"network_mode"`
	NetworkName   string            `json:"network_name,omitempty"`
	Services      []ServicePreview  `json:"services,omitempty"`
	Matrix        []MatrixPreview   `json:"matrix,omitempty"`
	MatrixCount   int               `json:"matrix_count"`
	Steps         []StepPreview     `json:"steps"`
	IfCondition   string            `json:"if_condition,omitempty"`
	Timeout       string            `json:"timeout,omitempty"`
	Needs         []string          `json:"needs,omitempty"`
	Env           []EnvSource       `json:"env,omitempty"`
}

type ActionCachePreview struct {
	Enabled        bool              `json:"enabled"`
	CacheDir       string            `json:"cache_dir"`
	OfflineMode    bool              `json:"offline_mode"`
	UseNewCache    bool              `json:"use_new_cache"`
	LocalRepos     map[string]string `json:"local_repositories,omitempty"`
}

type Selection struct {
	EventName string `json:"event_name"`
	JobID     string `json:"job_id,omitempty"`
	Workflow  string `json:"workflow,omitempty"`
}

type PreviewPlan struct {
	Selection           Selection                        `json:"selection"`
	EventPath           string                           `json:"event_path,omitempty"`
	Actor               string                           `json:"actor"`
	Workdir             string                           `json:"workdir"`
	WorkflowsPath       string                           `json:"workflows_path"`
	DefaultBranch       string                           `json:"default_branch,omitempty"`
	ContainerNetworkMode docker_container.NetworkMode    `json:"container_network_mode"`
	ContainerArch       string                           `json:"container_architecture,omitempty"`
	Platforms           map[string]string                `json:"platforms"`
	ActionCache         ActionCachePreview               `json:"action_cache"`
	Env                 []EnvSource                      `json:"env,omitempty"`
	Secrets             []SecretSource                   `json:"secrets,omitempty"`
	Vars                []VarSource                      `json:"vars,omitempty"`
	Jobs                []JobPreview                     `json:"jobs"`
	Plan                *model.Plan                      `json:"-"`
	RunnerConfig        *Config                          `json:"-"`
	TotalJobs           int                              `json:"total_jobs"`
	TotalStages         int                              `json:"total_stages"`
}

type PreviewInput struct {
	Ctx                context.Context
	Config             *Config
	Plan               *model.Plan
	WorkflowsPath      string
	Selection          Selection
	EnvFile            string
	SecretFile         string
	VarFile            string
	EnvCLI             map[string]string
	SecretCLI          map[string]string
	VarCLI             map[string]string
	InputFile          string
	InputCLI           map[string]string
	ActionCacheDir     string
	ActionOfflineMode  bool
	UseNewActionCache  bool
	LocalRepositories  []string
}

func NewPreviewPlan(input *PreviewInput) (*PreviewPlan, error) {
	pp := &PreviewPlan{
		Selection:             input.Selection,
		EventPath:             input.Config.EventPath,
		Actor:                 input.Config.Actor,
		Workdir:               input.Config.Workdir,
		WorkflowsPath:         input.WorkflowsPath,
		DefaultBranch:         input.Config.DefaultBranch,
		ContainerNetworkMode:  input.Config.ContainerNetworkMode,
		ContainerArch:         input.Config.ContainerArchitecture,
		Platforms:             input.Config.Platforms,
		ActionCache: ActionCachePreview{
			Enabled:     input.Config.ActionCache != nil,
			CacheDir:    input.ActionCacheDir,
			OfflineMode: input.ActionOfflineMode,
			UseNewCache: input.UseNewActionCache,
		},
		Plan:         input.Plan,
		RunnerConfig: input.Config,
		TotalStages:  len(input.Plan.Stages),
	}

	if len(input.LocalRepositories) > 0 {
		pp.ActionCache.LocalRepos = make(map[string]string)
		for _, l := range input.LocalRepositories {
			k, v, _ := strings.Cut(l, "=")
			pp.ActionCache.LocalRepos[k] = v
		}
	}

	pp.Env = collectEnvSources(input.EnvCLI, input.EnvFile)
	pp.Secrets = collectSecretSources(input.SecretCLI, input.SecretFile, input.Config.Secrets)
	pp.Vars = collectVarSources(input.VarCLI, input.VarFile, input.Config.Vars)

	for stageIdx, stage := range input.Plan.Stages {
		for _, run := range stage.Runs {
			jobPreview, err := buildJobPreview(input, run, stageIdx)
			if err != nil {
				log.Warnf("Failed to build preview for job %s: %v", run.JobID, err)
				continue
			}
			pp.Jobs = append(pp.Jobs, *jobPreview)
		}
	}

	pp.TotalJobs = len(pp.Jobs)

	return pp, nil
}

func collectEnvSources(cli map[string]string, envFile string) []EnvSource {
	sources := []EnvSource{}
	added := make(map[string]bool)

	for k, v := range cli {
		sources = append(sources, EnvSource{
			Key:    k,
			Value:  v,
			Source: "cli",
		})
		added[k] = true
	}

	if envFile != "" {
		if _, err := os.Stat(envFile); err == nil {
			content, _ := os.ReadFile(envFile)
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 && !added[parts[0]] {
					sources = append(sources, EnvSource{
						Key:    parts[0],
						Value:  parts[1],
						Source: "file",
					})
					added[parts[0]] = true
				}
			}
		}
	}

	return sources
}

func collectSecretSources(cli map[string]string, secretFile string, configSecrets map[string]string) []SecretSource {
	sources := []SecretSource{}
	added := make(map[string]bool)

	for k, v := range cli {
		sources = append(sources, SecretSource{
			Key:    k,
			Source: "cli",
			Set:    v != "",
		})
		added[k] = true
	}

	if secretFile != "" {
		if _, err := os.Stat(secretFile); err == nil {
			content, _ := os.ReadFile(secretFile)
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) >= 1 && !added[parts[0]] {
					sources = append(sources, SecretSource{
						Key:    parts[0],
						Source: "file",
						Set:    len(parts) == 2 && parts[1] != "",
					})
					added[parts[0]] = true
				}
			}
		}
	}

	for k, v := range configSecrets {
		if !added[k] {
			sources = append(sources, SecretSource{
				Key:    k,
				Source: "config",
				Set:    v != "",
			})
			added[k] = true
		}
	}

	return sources
}

func collectVarSources(cli map[string]string, varFile string, configVars map[string]string) []VarSource {
	sources := []VarSource{}
	added := make(map[string]bool)

	for k, v := range cli {
		sources = append(sources, VarSource{
			Key:    k,
			Value:  v,
			Source: "cli",
		})
		added[k] = true
	}

	if varFile != "" {
		if _, err := os.Stat(varFile); err == nil {
			content, _ := os.ReadFile(varFile)
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 && !added[parts[0]] {
					sources = append(sources, VarSource{
						Key:    parts[0],
						Value:  parts[1],
						Source: "file",
					})
					added[parts[0]] = true
				}
			}
		}
	}

	for k, v := range configVars {
		if !added[k] {
			sources = append(sources, VarSource{
				Key:    k,
				Value:  v,
				Source: "config",
			})
			added[k] = true
		}
	}

	return sources
}

func buildJobPreview(input *PreviewInput, run *model.Run, stageIdx int) (*JobPreview, error) {
	job := run.Job()
	if job == nil {
		return nil, fmt.Errorf("job not found: %s", run.JobID)
	}

	jobPreview := &JobPreview{
		JobID:        run.JobID,
		JobName:      run.String(),
		Stage:        stageIdx,
		WorkflowName: run.Workflow.Name,
		WorkflowFile: run.Workflow.File,
		Events:       run.Workflow.On(),
		RunsOn:       job.RunsOn(),
		IfCondition:  job.If.Value,
		Timeout:      job.TimeoutMinutes,
		Needs:        job.Needs(),
		NetworkMode:  string(input.Config.ContainerNetworkMode),
	}

	platformImage := resolvePlatformImage(job, input.Config.Platforms)
	jobPreview.PlatformImage = platformImage

	if container := job.Container(); container != nil {
		jobPreview.Container = &ContainerPreview{
			Image:       container.Image,
			Env:         container.Env,
			Ports:       container.Ports,
			Volumes:     container.Volumes,
			Options:     container.Options,
			Credentials: container.Credentials != nil && len(container.Credentials) > 0,
		}
		if jobPreview.Container.Image != "" {
			jobPreview.PlatformImage = jobPreview.Container.Image
		}
	}

	if len(job.Services) > 0 {
		networkName := fmt.Sprintf("act-%s-%s-network", run.Workflow.Name, run.JobID)
		jobPreview.NetworkName = networkName
		for svcID, spec := range job.Services {
			jobPreview.Services = append(jobPreview.Services, ServicePreview{
				ID:      svcID,
				Image:   spec.Image,
				Env:     spec.Env,
				Ports:   spec.Ports,
				Volumes: spec.Volumes,
				Options: spec.Options,
			})
		}
	}

	if job.Strategy != nil {
		if matrix := job.Matrix(); matrix != nil {
			for k, v := range matrix {
				if k == "include" || k == "exclude" {
					continue
				}
				jobPreview.Matrix = append(jobPreview.Matrix, MatrixPreview{
					Key:    k,
					Values: v,
				})
			}
		}
		matrixes, err := job.GetMatrixes()
		if err == nil {
			jobPreview.MatrixCount = len(matrixes)
		} else {
			jobPreview.MatrixCount = 1
		}
	} else {
		jobPreview.MatrixCount = 1
	}

	workflowEnv := run.Workflow.Env
	jobEnv := job.Environment()
	for k, v := range workflowEnv {
		jobPreview.Env = append(jobPreview.Env, EnvSource{
			Key:    k,
			Value:  v,
			Source: "workflow",
		})
	}
	for k, v := range jobEnv {
		jobPreview.Env = append(jobPreview.Env, EnvSource{
			Key:    k,
			Value:  v,
			Source: "job",
		})
	}

	for i, step := range job.Steps {
		if step == nil {
			continue
		}
		if step.ID == "" {
			step.ID = fmt.Sprintf("%d", i)
		}
		stepPreview := StepPreview{
			ID:   step.ID,
			Name: step.String(),
			Type: step.Type().String(),
			Uses: step.Uses,
			Run:  step.Run,
		}

		if step.Type() == model.StepTypeUsesActionRemote && input.Config.ActionCache != nil {
			cacheHit := checkActionCacheHit(input, step.Uses)
			stepPreview.CacheHit = &cacheHit
		}

		jobPreview.Steps = append(jobPreview.Steps, stepPreview)
	}

	return jobPreview, nil
}

func resolvePlatformImage(job *model.Job, platforms map[string]string) string {
	if job.RunsOn() == nil {
		return ""
	}

	for _, platformName := range job.RunsOn() {
		image := platforms[strings.ToLower(platformName)]
		if image != "" {
			return image
		}
	}

	return ""
}

func checkActionCacheHit(input *PreviewInput, uses string) bool {
	if input.ActionCacheDir == "" || uses == "" {
		return false
	}

	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '@' || r == ':' {
			return '-'
		}
		return r
	}, uses)

	gitPath := filepath.Join(input.ActionCacheDir, safeName+".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

func (pp *PreviewPlan) JSON() (string, error) {
	data, err := json.MarshalIndent(pp, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (pp *PreviewPlan) ListTable() string {
	headerJobID := "Job ID"
	headerJobName := "Job name"
	headerStage := "Stage"
	headerWfName := "Workflow name"
	headerWfFile := "Workflow file"
	headerEvents := "Events"

	jobIDMaxWidth := len(headerJobID)
	jobNameMaxWidth := len(headerJobName)
	stageMaxWidth := len(headerStage)
	wfNameMaxWidth := len(headerWfName)
	wfFileMaxWidth := len(headerWfFile)
	eventsMaxWidth := len(headerEvents)

	for _, job := range pp.Jobs {
		stageStr := fmt.Sprintf("%d", job.Stage)
		eventsStr := strings.Join(job.Events, ",")
		if jobIDMaxWidth < len(job.JobID) {
			jobIDMaxWidth = len(job.JobID)
		}
		if jobNameMaxWidth < len(job.JobName) {
			jobNameMaxWidth = len(job.JobName)
		}
		if stageMaxWidth < len(stageStr) {
			stageMaxWidth = len(stageStr)
		}
		if wfNameMaxWidth < len(job.WorkflowName) {
			wfNameMaxWidth = len(job.WorkflowName)
		}
		if wfFileMaxWidth < len(job.WorkflowFile) {
			wfFileMaxWidth = len(job.WorkflowFile)
		}
		if eventsMaxWidth < len(eventsStr) {
			eventsMaxWidth = len(eventsStr)
		}
	}

	jobIDMaxWidth += 2
	jobNameMaxWidth += 2
	stageMaxWidth += 2
	wfNameMaxWidth += 2
	wfFileMaxWidth += 2

	var sb strings.Builder
	fmt.Fprintf(&sb, "%*s%*s%*s%*s%*s%*s\n",
		-stageMaxWidth, headerStage,
		-jobIDMaxWidth, headerJobID,
		-jobNameMaxWidth, headerJobName,
		-wfNameMaxWidth, headerWfName,
		-wfFileMaxWidth, headerWfFile,
		-eventsMaxWidth, headerEvents,
	)

	for _, job := range pp.Jobs {
		stageStr := fmt.Sprintf("%d", job.Stage)
		eventsStr := strings.Join(job.Events, ",")
		fmt.Fprintf(&sb, "%*s%*s%*s%*s%*s%*s\n",
			-stageMaxWidth, stageStr,
			-jobIDMaxWidth, job.JobID,
			-jobNameMaxWidth, job.JobName,
			-wfNameMaxWidth, job.WorkflowName,
			-wfFileMaxWidth, job.WorkflowFile,
			-eventsMaxWidth, eventsStr,
		)
	}

	return sb.String()
}

func (pp *PreviewPlan) GraphDrawing() []string {
	lines := make([]string, 0)
	maxJobLen := 0
	for _, job := range pp.Jobs {
		if len(job.JobName) > maxJobLen {
			maxJobLen = len(job.JobName)
		}
	}
	boxWidth := maxJobLen + 4

	stages := make(map[int][]string)
	for _, job := range pp.Jobs {
		stages[job.Stage] = append(stages[job.Stage], job.JobName)
	}

	for i := 0; i < pp.TotalStages; i++ {
		if i > 0 {
			arrowPad := (boxWidth - 4) / 2
			lines = append(lines, fmt.Sprintf("%s%s", strings.Repeat(" ", arrowPad), "⬇"))
		}
		jobs := stages[i]
		if len(jobs) == 0 {
			continue
		}

		jobName := jobs[0]
		padding := boxWidth - len(jobName) - 2
		leftPad := padding / 2
		rightPad := padding - leftPad

		lines = append(lines, fmt.Sprintf("╭%s╮", strings.Repeat("─", boxWidth-2)))
		lines = append(lines, fmt.Sprintf("│%s%s%s│", strings.Repeat(" ", leftPad), jobName, strings.Repeat(" ", rightPad)))
		lines = append(lines, fmt.Sprintf("╰%s╯", strings.Repeat("─", boxWidth-2)))
	}

	return lines
}

func (pp *PreviewPlan) Table() string {
	var sb strings.Builder

	sb.WriteString("=== Execution Preview ===\n\n")
	sb.WriteString(fmt.Sprintf("Event:       %s\n", pp.Selection.EventName))
	if pp.EventPath != "" {
		sb.WriteString(fmt.Sprintf("Event File:  %s\n", pp.EventPath))
	}
	sb.WriteString(fmt.Sprintf("Actor:       %s\n", pp.Actor))
	sb.WriteString(fmt.Sprintf("Workdir:     %s\n", pp.Workdir))
	sb.WriteString(fmt.Sprintf("Total Jobs:  %d\n", pp.TotalJobs))
	sb.WriteString(fmt.Sprintf("Total Stages:%d\n", pp.TotalStages))
	sb.WriteString(fmt.Sprintf("Network:     %s\n", string(pp.ContainerNetworkMode)))
	if pp.ContainerArch != "" {
		sb.WriteString(fmt.Sprintf("Arch:        %s\n", pp.ContainerArch))
	}
	sb.WriteString("\n")

	sb.WriteString("--- Platforms ---\n")
	for k, v := range pp.Platforms {
		sb.WriteString(fmt.Sprintf("  %-20s -> %s\n", k, v))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("--- Action Cache ---\n"))
	sb.WriteString(fmt.Sprintf("  Enabled:     %v\n", pp.ActionCache.Enabled))
	sb.WriteString(fmt.Sprintf("  Cache Dir:   %s\n", pp.ActionCache.CacheDir))
	sb.WriteString(fmt.Sprintf("  Offline Mode:%v\n", pp.ActionCache.OfflineMode))
	sb.WriteString(fmt.Sprintf("  New Cache:   %v\n", pp.ActionCache.UseNewCache))
	if len(pp.ActionCache.LocalRepos) > 0 {
		sb.WriteString("  Local Repos:\n")
		for k, v := range pp.ActionCache.LocalRepos {
			sb.WriteString(fmt.Sprintf("    %s -> %s\n", k, v))
		}
	}
	sb.WriteString("\n")

	if len(pp.Env) > 0 {
		sb.WriteString("--- Environment Variables ---\n")
		for _, e := range pp.Env {
			sb.WriteString(fmt.Sprintf("  %-30s = %-20s [source: %s]\n", e.Key, truncate(e.Value, 20), e.Source))
		}
		sb.WriteString("\n")
	}

	if len(pp.Secrets) > 0 {
		sb.WriteString("--- Secrets ---\n")
		for _, s := range pp.Secrets {
			status := "set"
			if !s.Set {
				status = "not set"
			}
			sb.WriteString(fmt.Sprintf("  %-30s [source: %s, %s]\n", s.Key, s.Source, status))
		}
		sb.WriteString("\n")
	}

	if len(pp.Vars) > 0 {
		sb.WriteString("--- Variables ---\n")
		for _, v := range pp.Vars {
			sb.WriteString(fmt.Sprintf("  %-30s = %-20s [source: %s]\n", v.Key, truncate(v.Value, 20), v.Source))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("=== Jobs ===\n\n")
	for _, job := range pp.Jobs {
		sb.WriteString(fmt.Sprintf("[Stage %d] %s (%s)\n", job.Stage, job.JobName, job.JobID))
		sb.WriteString(fmt.Sprintf("  Workflow: %s (%s)\n", job.WorkflowName, job.WorkflowFile))
		sb.WriteString(fmt.Sprintf("  Runs On:  %v\n", job.RunsOn))
		if job.PlatformImage != "" {
			sb.WriteString(fmt.Sprintf("  Image:    %s\n", job.PlatformImage))
		}
		sb.WriteString(fmt.Sprintf("  Network:  %s\n", job.NetworkMode))
		if job.NetworkName != "" {
			sb.WriteString(fmt.Sprintf("  Net Name: %s\n", job.NetworkName))
		}
		if job.IfCondition != "" {
			sb.WriteString(fmt.Sprintf("  If:       %s\n", job.IfCondition))
		}
		if len(job.Needs) > 0 {
			sb.WriteString(fmt.Sprintf("  Needs:    %v\n", job.Needs))
		}
		if job.MatrixCount > 1 {
			sb.WriteString(fmt.Sprintf("  Matrix:   %d combinations\n", job.MatrixCount))
			for _, m := range job.Matrix {
				sb.WriteString(fmt.Sprintf("    %s: %v\n", m.Key, m.Values))
			}
		}
		if len(job.Services) > 0 {
			sb.WriteString("  Services:\n")
			for _, svc := range job.Services {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", svc.ID, svc.Image))
				if len(svc.Ports) > 0 {
					sb.WriteString(fmt.Sprintf("      Ports: %v\n", svc.Ports))
				}
			}
		}
		sb.WriteString("  Steps:\n")
		for i, step := range job.Steps {
			cacheInfo := ""
			if step.CacheHit != nil {
				if *step.CacheHit {
					cacheInfo = " [CACHE HIT]"
				} else {
					cacheInfo = " [CACHE MISS]"
				}
			}
			sb.WriteString(fmt.Sprintf("    %2d. %-40s [type: %s]%s\n", i+1, truncate(step.Name, 40), step.Type, cacheInfo))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
