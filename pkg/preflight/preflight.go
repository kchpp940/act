package preflight

import (
	"context"
	"fmt"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type CheckResult struct {
	Name        string
	Severity    Severity
	Message     string
	Suggestion  string
	Err         error
}

func (cr CheckResult) IsBlocking() bool {
	return cr.Severity == SeverityError
}

func (cr CheckResult) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s", cr.Severity, cr.Name))
	if cr.Message != "" {
		sb.WriteString(fmt.Sprintf(": %s", cr.Message))
	}
	if cr.Err != nil {
		sb.WriteString(fmt.Sprintf(" (%v)", cr.Err))
	}
	if cr.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("\n  Suggestion: %s", cr.Suggestion))
	}
	return sb.String()
}

type Checker interface {
	Name() string
	Check(ctx context.Context) CheckResult
}

type CheckConfig struct {
	DockerDaemonSocket    string
	ContainerArchitecture string
	Workdir               string
	EventPath             string
	EnvFile               string
	SecretFile            string
	VarFile               string
	WorkflowsPath         string
	ActionCachePath       string
	Platforms             map[string]string
	Plan                  PlanInfo
	EventName             string
}

type PlanInfo struct {
	Stages   int
	TotalJobs int
	JobIDs   []string
	IsEmpty  bool
}

func RunBasicChecks(ctx context.Context, config CheckConfig) []CheckResult {
	checkers := []Checker{
		NewWorkdirChecker(config.Workdir),
		NewWorkflowsPathChecker(config.WorkflowsPath),
		NewEventFileChecker(config.EventPath),
		NewEnvFilesChecker(config.EnvFile, config.SecretFile, config.VarFile),
		NewPlannerResultChecker(config.Plan, config.EventName),
	}

	results := make([]CheckResult, 0, len(checkers))
	for _, checker := range checkers {
		results = append(results, checker.Check(ctx))
	}

	return results
}

func RunExecutionChecks(ctx context.Context, config CheckConfig) []CheckResult {
	checkers := []Checker{
		NewDockerDaemonChecker(config.DockerDaemonSocket, config.ContainerArchitecture),
		NewPlatformChecker(config.Platforms, config.ContainerArchitecture),
		NewActionCacheChecker(config.ActionCachePath),
	}

	results := make([]CheckResult, 0, len(checkers))
	for _, checker := range checkers {
		results = append(results, checker.Check(ctx))
	}

	return results
}

func RunChecks(ctx context.Context, config CheckConfig) []CheckResult {
	results := RunBasicChecks(ctx, config)
	results = append(results, RunExecutionChecks(ctx, config)...)
	return results
}

func HasBlockingErrors(results []CheckResult) bool {
	for _, r := range results {
		if r.IsBlocking() {
			return true
		}
	}
	return false
}

func FormatResults(results []CheckResult) string {
	var sb strings.Builder

	errors := []CheckResult{}
	warnings := []CheckResult{}

	for _, r := range results {
		if r.IsBlocking() {
			errors = append(errors, r)
		} else {
			warnings = append(warnings, r)
		}
	}

	if len(errors) > 0 {
		sb.WriteString(fmt.Sprintf("\n❌ Preflight check found %d blocking error(s):\n\n", len(errors)))
		for i, e := range errors {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, e.String()))
		}
		sb.WriteString("\n")
	}

	if len(warnings) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️  Preflight check found %d warning(s):\n\n", len(warnings)))
		for i, w := range warnings {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, w.String()))
		}
		sb.WriteString("\n")
	}

	if len(errors) == 0 && len(warnings) == 0 {
		sb.WriteString("\n✅ Preflight check passed\n\n")
	}

	return sb.String()
}
