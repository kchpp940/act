package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ptrString(v string) *string { return &v }
func ptrBool(v bool) *bool     { return &v }
func ptrSlice(v []string) *[]string { return &v }

func TestLoader_LoadDefaults(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)
	pipeline.LoadDefaults()

	raw := pipeline.RawConfig()
	assert.Equal(t, "nektos/act", raw.Actor.Value)
	assert.Equal(t, "./.github/workflows/", raw.WorkflowsPath.Value)
	assert.Equal(t, "origin", raw.RemoteName.Value)
	assert.True(t, raw.ForcePull.Value)
	assert.True(t, raw.ForceRebuild.Value)
	assert.True(t, raw.UseGitIgnore.Value)
	assert.Equal(t, "github.com", raw.GitHubInstance.Value)
	assert.Equal(t, ".env", raw.Envfile.Value)
	assert.Equal(t, ".secrets", raw.Secretfile.Value)
	assert.Equal(t, ".vars", raw.Varfile.Value)
	assert.Equal(t, ".input", raw.Inputfile.Value)
	assert.Equal(t, "host", raw.NetworkName.Value)
}

func TestLoader_LoadFromInput(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)
	pipeline.LoadDefaults()

	adapter := &InputAdapter{
		Actor:         ptrString("test-user"),
		Workdir:       ptrString("/tmp/test"),
		WorkflowsPath: ptrString("./custom/workflows"),
		ForcePull:     ptrBool(false),
		Dryrun:        ptrBool(true),
	}

	pipeline.LoadFromInput(adapter)
	raw := pipeline.RawConfig()

	assert.Equal(t, "test-user", raw.Actor.Value)
	assert.Equal(t, "/tmp/test", raw.Workdir.Value)
	assert.Equal(t, "./custom/workflows", raw.WorkflowsPath.Value)
	assert.False(t, raw.ForcePull.Value)
	assert.True(t, raw.Dryrun.Value)
}

func TestNormalizer_Normalize(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)
	pipeline.LoadDefaults()

	adapter := &InputAdapter{
		Workdir: ptrString("/tmp/test"),
		Envs:    ptrSlice([]string{"KEY1=value1", "KEY2=value2"}),
		Secrets: ptrSlice([]string{"SECRET1=secret1"}),
		Vars:    ptrSlice([]string{"VAR1=var1"}),
		Inputs:  ptrSlice([]string{"INPUT1=input1"}),
		Matrix:  ptrSlice([]string{"java:11", "java:17"}),
	}

	pipeline.LoadFromInput(adapter)
	pipeline.LoadFiles()

	execConfig, err := pipeline.Normalize()
	assert.NoError(t, err)
	assert.NotNil(t, execConfig)

	assert.Equal(t, "/tmp/test", execConfig.Workdir)
	assert.Equal(t, "value1", execConfig.Env["KEY1"])
	assert.Equal(t, "value2", execConfig.Env["KEY2"])
	assert.Equal(t, "secret1", execConfig.Secrets["SECRET1"])
	assert.Equal(t, "var1", execConfig.Vars["VAR1"])
	assert.Equal(t, "input1", execConfig.Inputs["INPUT1"])
	assert.NotNil(t, execConfig.Matrix["java"])
	assert.True(t, execConfig.Matrix["java"]["11"])
	assert.True(t, execConfig.Matrix["java"]["17"])
}

func TestExecutionConfig_NormalizedFields(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)
	pipeline.LoadDefaults()

	adapter := &InputAdapter{
		Workdir:        ptrString("/tmp/test"),
		Actor:          ptrString("test-actor"),
		ForcePull:      ptrBool(true),
		Dryrun:         ptrBool(true),
		GitHubInstance: ptrString("github.com"),
		RemoteName:     ptrString("origin"),
	}

	pipeline.LoadFromInput(adapter)
	execConfig, err := pipeline.Normalize()
	assert.NoError(t, err)

	assert.Equal(t, "test-actor", execConfig.Actor)
	assert.Equal(t, "/tmp/test", execConfig.Workdir)
	assert.True(t, execConfig.ForcePull)
	assert.True(t, execConfig.Dryrun)
	assert.Equal(t, "github.com", execConfig.GitHubInstance)
	assert.Equal(t, "origin", execConfig.RemoteName)
}

func TestConfigSource_Priority(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)

	pipeline.LoadDefaults()
	raw := pipeline.RawConfig()
	assert.Equal(t, SourceDefault, raw.Actor.Source)
	assert.Equal(t, "nektos/act", raw.Actor.Value)

	adapter := &InputAdapter{
		Actor: ptrString("flags-user"),
	}
	pipeline.LoadFromInput(adapter)
	raw = pipeline.RawConfig()
	assert.Equal(t, SourceFlags, raw.Actor.Source)
	assert.Equal(t, "flags-user", raw.Actor.Value)
}

func TestLoader_ZeroValuesDontOverrideDefaults(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)

	pipeline.LoadDefaults()
	raw := pipeline.RawConfig()
	assert.True(t, raw.ForcePull.Value)

	adapter := &InputAdapter{
		Actor: ptrString("test-user"),
	}
	pipeline.LoadFromInput(adapter)

	raw = pipeline.RawConfig()
	assert.True(t, raw.ForcePull.Value)
}

func TestLoader_ExplicitEmptyValuesOverrideDefaults(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)

	pipeline.LoadDefaults()
	raw := pipeline.RawConfig()
	assert.Equal(t, "nektos/act", raw.Actor.Value)
	assert.True(t, raw.ForcePull.Value)
	assert.Equal(t, ".env", raw.Envfile.Value)
	assert.Equal(t, "host", raw.NetworkName.Value)

	adapter := &InputAdapter{
		Actor:     ptrString(""),
		ForcePull: ptrBool(false),
		Envfile:   ptrString(""),
		NetworkName: ptrString(""),
	}
	pipeline.LoadFromInput(adapter)

	raw = pipeline.RawConfig()
	assert.Equal(t, SourceFlags, raw.Actor.Source)
	assert.Equal(t, "", raw.Actor.Value)
	assert.Equal(t, SourceFlags, raw.ForcePull.Source)
	assert.False(t, raw.ForcePull.Value)
	assert.Equal(t, SourceFlags, raw.Envfile.Source)
	assert.Equal(t, "", raw.Envfile.Value)
	assert.Equal(t, SourceFlags, raw.NetworkName.Source)
	assert.Equal(t, "", raw.NetworkName.Value)
}

func TestLoader_ExplicitEmptySliceOverrideDefaults(t *testing.T) {
	ctx := context.Background()
	pipeline := NewPipeline(ctx)

	pipeline.LoadDefaults()

	adapter := &InputAdapter{
		Platforms: ptrSlice([]string{}),
	}
	pipeline.LoadFromInput(adapter)

	raw := pipeline.RawConfig()
	assert.Equal(t, SourceFlags, raw.Platforms.Source)
	assert.Equal(t, []string{}, raw.Platforms.Value)
}

func TestParseEnvs(t *testing.T) {
	envs := parseEnvs([]string{"KEY1=value1", "KEY2=value2", "KEY3="})
	assert.Equal(t, "value1", envs["KEY1"])
	assert.Equal(t, "value2", envs["KEY2"])
	assert.Equal(t, "", envs["KEY3"])
}

func TestParseMatrix(t *testing.T) {
	matrix := parseMatrix([]string{"java:11", "java:17", "node:16", "node:18"})
	assert.NotNil(t, matrix["java"])
	assert.True(t, matrix["java"]["11"])
	assert.True(t, matrix["java"]["17"])
	assert.NotNil(t, matrix["node"])
	assert.True(t, matrix["node"]["16"])
	assert.True(t, matrix["node"]["18"])
}
