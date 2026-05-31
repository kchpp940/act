package preflight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckResult_IsBlocking(t *testing.T) {
	tests := []struct {
		name     string
		result   CheckResult
		expected bool
	}{
		{
			name: "error is blocking",
			result: CheckResult{
				Severity: SeverityError,
			},
			expected: true,
		},
		{
			name: "warning is not blocking",
			result: CheckResult{
				Severity: SeverityWarning,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.IsBlocking())
		})
	}
}

func TestCheckResult_String(t *testing.T) {
	result := CheckResult{
		Name:       "Test Check",
		Severity:   SeverityError,
		Message:    "something went wrong",
		Suggestion: "try again",
	}

	str := result.String()
	assert.Contains(t, str, "Test Check")
	assert.Contains(t, str, "error")
	assert.Contains(t, str, "something went wrong")
	assert.Contains(t, str, "try again")
}

func TestHasBlockingErrors(t *testing.T) {
	tests := []struct {
		name     string
		results  []CheckResult
		expected bool
	}{
		{
			name:     "no results",
			results:  []CheckResult{},
			expected: false,
		},
		{
			name: "only warnings",
			results: []CheckResult{
				{Severity: SeverityWarning},
				{Severity: SeverityWarning},
			},
			expected: false,
		},
		{
			name: "has error",
			results: []CheckResult{
				{Severity: SeverityWarning},
				{Severity: SeverityError},
				{Severity: SeverityWarning},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasBlockingErrors(tt.results))
		})
	}
}

func TestFormatResults(t *testing.T) {
	tests := []struct {
		name     string
		results  []CheckResult
		contains []string
	}{
		{
			name:     "no results",
			results:  []CheckResult{},
			contains: []string{"✅", "passed"},
		},
		{
			name: "has error",
			results: []CheckResult{
				{Name: "Test", Severity: SeverityError, Message: "failed"},
			},
			contains: []string{"❌", "blocking error", "Test", "failed"},
		},
		{
			name: "has warning",
			results: []CheckResult{
				{Name: "Test", Severity: SeverityWarning, Message: "warning"},
			},
			contains: []string{"⚠️", "warning", "Test", "warning"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatResults(tt.results)
			for _, s := range tt.contains {
				assert.Contains(t, output, s)
			}
		})
	}
}

func TestWorkdirChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("valid directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		checker := NewWorkdirChecker(tmpDir)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "valid")
	})

	t.Run("non-existent directory", func(t *testing.T) {
		checker := NewWorkdirChecker("/nonexistent/path/that/should/not/exist")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "does not exist")
	})

	t.Run("file instead of directory", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "testfile")
		os.WriteFile(tmpFile, []byte("test"), 0o644)
		checker := NewWorkdirChecker(tmpFile)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "not a directory")
	})
}

func TestEventFileChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("no event file", func(t *testing.T) {
		checker := NewEventFileChecker("")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "default")
	})

	t.Run("valid JSON file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "event.json")
		os.WriteFile(tmpFile, []byte(`{"action": "push"}`), 0o644)
		checker := NewEventFileChecker(tmpFile)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "valid")
	})

	t.Run("invalid JSON file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "event.json")
		os.WriteFile(tmpFile, []byte(`not valid json`), 0o644)
		checker := NewEventFileChecker(tmpFile)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "not valid JSON")
	})

	t.Run("non-existent file", func(t *testing.T) {
		checker := NewEventFileChecker("/nonexistent/path/event.json")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "does not exist")
	})
}

func TestWorkflowsPathChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("directory with workflows", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "workflow1.yml"), []byte("name: test"), 0o644)
		os.WriteFile(filepath.Join(tmpDir, "workflow2.yaml"), []byte("name: test2"), 0o644)
		checker := NewWorkflowsPathChecker(tmpDir)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "2 workflow file(s)")
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		checker := NewWorkflowsPathChecker(tmpDir)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "No workflow files")
	})

	t.Run("non-existent directory", func(t *testing.T) {
		checker := NewWorkflowsPathChecker("/nonexistent/path")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "does not exist")
	})
}

func TestActionCacheChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("valid directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		checker := NewActionCacheChecker(tmpDir)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "valid")
	})

	t.Run("create non-existent directory", func(t *testing.T) {
		tmpDir := filepath.Join(t.TempDir(), "new", "nested", "dir")
		checker := NewActionCacheChecker(tmpDir)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "Created")
	})

	t.Run("no path specified", func(t *testing.T) {
		checker := NewActionCacheChecker("")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "default")
	})
}

func TestEnvFilesChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("all files accessible", func(t *testing.T) {
		tmpDir := t.TempDir()
		envFile := filepath.Join(tmpDir, ".env")
		secretFile := filepath.Join(tmpDir, ".secrets")
		varFile := filepath.Join(tmpDir, ".vars")

		os.WriteFile(envFile, []byte("KEY=value"), 0o644)
		os.WriteFile(secretFile, []byte("SECRET=value"), 0o644)
		os.WriteFile(varFile, []byte("VAR=value"), 0o644)

		checker := NewEnvFilesChecker(envFile, secretFile, varFile)
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "accessible")
	})

	t.Run("some files missing", func(t *testing.T) {
		checker := NewEnvFilesChecker("/nonexistent/.env", "", "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "file not found")
	})

	t.Run("no files specified", func(t *testing.T) {
		checker := NewEnvFilesChecker("", "", "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "accessible")
	})
}

func TestPlatformChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("no platforms is error", func(t *testing.T) {
		checker := NewPlatformChecker(map[string]string{}, "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "No platforms")
	})

	t.Run("with platforms and tag", func(t *testing.T) {
		platforms := map[string]string{
			"ubuntu-latest": "catthehacker/ubuntu:act-latest",
		}
		checker := NewPlatformChecker(platforms, "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
	})

	t.Run("image without tag warns", func(t *testing.T) {
		platforms := map[string]string{
			"ubuntu-latest": "catthehacker/ubuntu",
		}
		checker := NewPlatformChecker(platforms, "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "no tag")
	})

	t.Run("empty image is error", func(t *testing.T) {
		platforms := map[string]string{
			"ubuntu-latest": "",
		}
		checker := NewPlatformChecker(platforms, "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "no image configured")
	})

	t.Run("invalid image name is error", func(t *testing.T) {
		platforms := map[string]string{
			"ubuntu-latest": "invalid image name:tag",
		}
		checker := NewPlatformChecker(platforms, "")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "invalid image name")
	})

	t.Run("with architecture", func(t *testing.T) {
		platforms := map[string]string{
			"ubuntu-latest": "catthehacker/ubuntu:act-latest",
		}
		checker := NewPlatformChecker(platforms, "linux/amd64")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "linux/amd64")
	})

	t.Run("invalid architecture is error", func(t *testing.T) {
		platforms := map[string]string{
			"ubuntu-latest": "catthehacker/ubuntu:act-latest",
		}
		checker := NewPlatformChecker(platforms, "invalid/arch")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "Unknown container architecture")
	})
}

func TestDockerDaemonChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("socket disabled", func(t *testing.T) {
		checker := NewDockerDaemonChecker("-")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "disabled")
	})
}

func TestRunBasicChecks(t *testing.T) {
	ctx := context.Background()

	t.Run("all basic checks pass", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowDir := t.TempDir()
		os.WriteFile(filepath.Join(workflowDir, "test.yml"), []byte("name: test"), 0o644)
		eventFile := filepath.Join(tmpDir, "event.json")
		os.WriteFile(eventFile, []byte(`{"test": true}`), 0o644)

		config := CheckConfig{
			Workdir:       tmpDir,
			WorkflowsPath: workflowDir,
			EventPath:     eventFile,
			Plan: PlanInfo{
				Stages:    1,
				TotalJobs: 1,
				JobIDs:    []string{"build"},
				IsEmpty:   false,
			},
			EventName: "push",
		}

		results := RunBasicChecks(ctx, config)
		assert.Len(t, results, 5) // workdir, workflows, event, env files, planner result

		for _, r := range results {
			assert.Equal(t, SeverityWarning, r.Severity, "expected no errors for check: %s, got: %s", r.Name, r.Message)
		}
	})

	t.Run("basic checks with errors", func(t *testing.T) {
		config := CheckConfig{
			Workdir:       "/nonexistent/path",
			WorkflowsPath: "/nonexistent/workflows",
			EventPath:     "/nonexistent/event.json",
			Plan:          PlanInfo{IsEmpty: true},
			EventName:     "push",
		}

		results := RunBasicChecks(ctx, config)
		assert.Len(t, results, 5)

		errors := 0
		for _, r := range results {
			if r.Severity == SeverityError {
				errors++
			}
		}
		assert.Greater(t, errors, 0, "expected at least one error")
	})
}

func TestRunExecutionChecks(t *testing.T) {
	ctx := context.Background()

	t.Run("execution checks with socket disabled", func(t *testing.T) {
		config := CheckConfig{
			DockerDaemonSocket: "-",
			ActionCachePath:    t.TempDir(),
			Platforms: map[string]string{
				"ubuntu-latest": "catthehacker/ubuntu:act-latest",
			},
		}

		results := RunExecutionChecks(ctx, config)
		assert.Len(t, results, 3) // docker, platform, action cache

		hasBlocking := false
		for _, r := range results {
			if r.IsBlocking() {
				hasBlocking = true
			}
		}
		assert.False(t, hasBlocking, "expected no blocking errors when socket is disabled")
	})

	t.Run("execution checks with missing cache dir", func(t *testing.T) {
		config := CheckConfig{
			DockerDaemonSocket: "-",
			ActionCachePath:    filepath.Join(t.TempDir(), "new", "cache", "dir"),
			Platforms:          map[string]string{},
		}

		results := RunExecutionChecks(ctx, config)
		assert.Len(t, results, 3)
	})
}

func TestRunChecks(t *testing.T) {
	ctx := context.Background()
	config := CheckConfig{
		Workdir:       t.TempDir(),
		WorkflowsPath: t.TempDir(),
		ActionCachePath: t.TempDir(),
		Platforms: map[string]string{
			"ubuntu-latest": "catthehacker/ubuntu:act-latest",
		},
		Plan: PlanInfo{
			Stages:    1,
			TotalJobs: 1,
			JobIDs:    []string{"build"},
			IsEmpty:   false,
		},
		EventName: "push",
	}

	basicResults := RunBasicChecks(ctx, config)
	execResults := RunExecutionChecks(ctx, config)
	allResults := RunChecks(ctx, config)

	assert.Equal(t, len(basicResults)+len(execResults), len(allResults),
		"RunChecks should return combined results of basic and execution checks")
}

func TestPlannerResultChecker(t *testing.T) {
	ctx := context.Background()

	t.Run("empty plan is error", func(t *testing.T) {
		checker := NewPlannerResultChecker(PlanInfo{IsEmpty: true}, "push")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "No jobs to run")
	})

	t.Run("plan with zero jobs is error", func(t *testing.T) {
		checker := NewPlannerResultChecker(PlanInfo{Stages: 1, TotalJobs: 0, JobIDs: nil, IsEmpty: false}, "push")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityError, result.Severity)
		assert.Contains(t, result.Message, "no jobs")
	})

	t.Run("valid plan with jobs", func(t *testing.T) {
		checker := NewPlannerResultChecker(PlanInfo{
			Stages:    2,
			TotalJobs: 3,
			JobIDs:    []string{"build", "test", "deploy"},
			IsEmpty:   false,
		}, "push")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
		assert.Contains(t, result.Message, "2 stage(s)")
		assert.Contains(t, result.Message, "3 job(s)")
		assert.Contains(t, result.Message, "build")
	})

	t.Run("long job list is truncated", func(t *testing.T) {
		jobIDs := make([]string, 50)
		for i := range jobIDs {
			jobIDs[i] = fmt.Sprintf("job-%d", i)
		}
		checker := NewPlannerResultChecker(PlanInfo{
			Stages:    1,
			TotalJobs: 50,
			JobIDs:    jobIDs,
			IsEmpty:   false,
		}, "push")
		result := checker.Check(ctx)
		assert.Equal(t, SeverityWarning, result.Severity)
	})
}
