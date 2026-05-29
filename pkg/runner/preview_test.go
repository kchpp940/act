package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	docker_container "github.com/moby/moby/api/types/container"
	"github.com/nektos/act/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPreviewPlan(t *testing.T) {
	ctx := context.Background()

	workflowContent := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    env:
      WORKFLOW_ENV: test
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: echo "test"
`

	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, ".github", "workflows")
	err := os.MkdirAll(workflowPath, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(workflowPath, "ci.yml"), []byte(workflowContent), 0644)
	require.NoError(t, err)

	planner, err := model.NewWorkflowPlanner(workflowPath, false, false)
	require.NoError(t, err)

	plan, err := planner.PlanEvent("push")
	require.NoError(t, err)

	cacheDir := filepath.Join(tmpDir, "cache")
	err = os.MkdirAll(cacheDir, 0755)
	require.NoError(t, err)

	platforms := map[string]string{
		"ubuntu-latest": "catthehacker/ubuntu:act-latest",
	}

	config := &Config{
		Workdir:              tmpDir,
		EventPath:            "",
		EventName:            "push",
		Actor:                "nektos/act",
		DefaultBranch:        "main",
		Platforms:            platforms,
		ContainerNetworkMode: docker_container.NetworkMode("host"),
		ActionCache:          &GoGitActionCache{Path: cacheDir},
		Secrets:              map[string]string{"TOKEN": "secret"},
		Vars:                 map[string]string{"DEPLOY_ENV": "prod"},
	}

	envCLI := map[string]string{"CI": "true"}

	input := &PreviewInput{
		Ctx:           ctx,
		Config:        config,
		Plan:          plan,
		WorkflowsPath: workflowPath,
		Selection: Selection{
			EventName: "push",
		},
		EnvCLI:            envCLI,
		SecretCLI:         map[string]string{"GITHUB_TOKEN": "abc"},
		VarCLI:            map[string]string{"MY_VAR": "value"},
		ActionCacheDir:    cacheDir,
		ActionOfflineMode: false,
		UseNewActionCache: true,
	}

	pp, err := NewPreviewPlan(input)
	require.NoError(t, err)

	assert.Equal(t, "push", pp.Selection.EventName)
	assert.Equal(t, "nektos/act", pp.Actor)
	assert.Equal(t, tmpDir, pp.Workdir)
	assert.Equal(t, 1, pp.TotalJobs)
	assert.Equal(t, 1, pp.TotalStages)
	assert.Equal(t, docker_container.NetworkMode("host"), pp.ContainerNetworkMode)
	assert.True(t, pp.ActionCache.Enabled)
	assert.Equal(t, cacheDir, pp.ActionCache.CacheDir)
	assert.True(t, pp.ActionCache.UseNewCache)

	assert.Len(t, pp.Env, 1)
	assert.Equal(t, "CI", pp.Env[0].Key)
	assert.Equal(t, "true", pp.Env[0].Value)
	assert.Equal(t, "cli", pp.Env[0].Source)

	assert.Len(t, pp.Secrets, 2)

	assert.Len(t, pp.Vars, 2)

	assert.Len(t, pp.Jobs, 1)
	job := pp.Jobs[0]
	assert.Equal(t, "build", job.JobID)
	assert.Equal(t, "CI", job.WorkflowName)
	assert.Equal(t, "ci.yml", job.WorkflowFile)
	assert.Equal(t, []string{"ubuntu-latest"}, job.RunsOn)
	assert.Equal(t, "catthehacker/ubuntu:act-latest", job.PlatformImage)
	assert.Equal(t, "host", job.NetworkMode)
	assert.Equal(t, 1, job.MatrixCount)
	assert.Len(t, job.Steps, 2)

	step1 := job.Steps[0]
	assert.Contains(t, step1.Name, "actions/checkout")
	assert.Equal(t, "remote-action", step1.Type)

	step2 := job.Steps[1]
	assert.Equal(t, "Run tests", step2.Name)
	assert.Equal(t, "run", step2.Type)
	assert.Equal(t, "echo \"test\"", step2.Run)
}

func TestPreviewPlanJSON(t *testing.T) {
	pp := &PreviewPlan{
		Selection: Selection{
			EventName: "push",
		},
		Actor:     "test",
		TotalJobs: 1,
		Platforms: map[string]string{
			"ubuntu-latest": "test-image",
		},
	}

	jsonStr, err := pp.JSON()
	require.NoError(t, err)
	assert.Contains(t, jsonStr, "push")
	assert.Contains(t, jsonStr, "test")
	assert.Contains(t, jsonStr, "ubuntu-latest")
}

func TestPreviewPlanTable(t *testing.T) {
	pp := &PreviewPlan{
		Selection: Selection{
			EventName: "push",
		},
		Actor:     "test",
		Workdir:   "/tmp/test",
		TotalJobs: 1,
		Platforms: map[string]string{
			"ubuntu-latest": "test-image",
		},
	}

	table := pp.Table()
	assert.Contains(t, table, "Execution Preview")
	assert.Contains(t, table, "Event:")
	assert.Contains(t, table, "push")
	assert.Contains(t, table, "Platforms")
	assert.Contains(t, table, "ubuntu-latest")
}

func TestCollectEnvSources(t *testing.T) {
	cli := map[string]string{"A": "1", "B": "2"}

	result := collectEnvSources(cli, "")
	assert.Len(t, result, 2)
}

func TestCollectSecretSources(t *testing.T) {
	cli := map[string]string{"TOKEN": "abc"}
	config := map[string]string{"GITHUB_TOKEN": "xyz"}

	result := collectSecretSources(cli, "", config)
	assert.Len(t, result, 2)
}

func TestCollectVarSources(t *testing.T) {
	cli := map[string]string{"VAR1": "val1"}
	config := map[string]string{"VAR2": "val2"}

	result := collectVarSources(cli, "", config)
	assert.Len(t, result, 2)
}

func TestCheckActionCacheHit(t *testing.T) {
	tmpDir := t.TempDir()

	input := &PreviewInput{
		ActionCacheDir: tmpDir,
	}

	hit := checkActionCacheHit(input, "actions/checkout@v4")
	assert.False(t, hit)

	gitPath := filepath.Join(tmpDir, "actions-checkout-v4.git")
	err := os.MkdirAll(gitPath, 0755)
	require.NoError(t, err)

	hit = checkActionCacheHit(input, "actions/checkout@v4")
	assert.True(t, hit)
}

func TestResolvePlatformImage(t *testing.T) {
	workflowContent := `
name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-22.04
    steps:
      - run: echo test
`

	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, ".github", "workflows")
	err := os.MkdirAll(workflowPath, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(workflowPath, "test.yml"), []byte(workflowContent), 0644)
	require.NoError(t, err)

	planner, err := model.NewWorkflowPlanner(workflowPath, false, false)
	require.NoError(t, err)

	plan, err := planner.PlanEvent("push")
	require.NoError(t, err)

	job := plan.Stages[0].Runs[0].Job()

	platforms := map[string]string{
		"ubuntu-22.04": "catthehacker/ubuntu:act-22.04",
		"ubuntu-latest": "catthehacker/ubuntu:act-latest",
	}

	image := resolvePlatformImage(job, platforms)
	assert.Equal(t, "catthehacker/ubuntu:act-22.04", image)
}
