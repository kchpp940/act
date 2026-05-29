package cmd

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadSecrets(t *testing.T) {
	secrets := map[string]string{}
	ret := readEnvsEx(path.Join("testdata", "secrets.yml"), secrets, true)
	assert.True(t, ret)
	assert.Equal(t, `line1
line2
line3
`, secrets["MYSECRET"])
}

func TestReadEnv(t *testing.T) {
	secrets := map[string]string{}
	ret := readEnvs(path.Join("testdata", "secrets.yml"), secrets)
	assert.True(t, ret)
	assert.Equal(t, `line1
line2
line3
`, secrets["mysecret"])
}

func TestListOptions(t *testing.T) {
	rootCmd := createRootCommand(context.Background(), &Input{}, "")
	err := newRunCommand(context.Background(), &Input{
		listOptions: true,
	})(rootCmd, []string{})
	assert.NoError(t, err)
}

func TestRun(t *testing.T) {
	rootCmd := createRootCommand(context.Background(), &Input{}, "")
	err := newRunCommand(context.Background(), &Input{
		platforms:     []string{"ubuntu-latest=node:16-buster-slim"},
		workdir:       "../pkg/runner/testdata/",
		workflowsPath: "./basic/push.yml",
	})(rootCmd, []string{})
	assert.NoError(t, err)
}

func TestRunPush(t *testing.T) {
	rootCmd := createRootCommand(context.Background(), &Input{}, "")
	err := newRunCommand(context.Background(), &Input{
		platforms:     []string{"ubuntu-latest=node:16-buster-slim"},
		workdir:       "../pkg/runner/testdata/",
		workflowsPath: "./basic/push.yml",
	})(rootCmd, []string{"push"})
	assert.NoError(t, err)
}

func TestRunPushJsonLogger(t *testing.T) {
	rootCmd := createRootCommand(context.Background(), &Input{}, "")
	err := newRunCommand(context.Background(), &Input{
		platforms:     []string{"ubuntu-latest=node:16-buster-slim"},
		workdir:       "../pkg/runner/testdata/",
		workflowsPath: "./basic/push.yml",
		jsonLogger:    true,
	})(rootCmd, []string{"push"})
	assert.NoError(t, err)
}

func TestFlags(t *testing.T) {
	for _, f := range []string{"graph", "list", "bug-report", "man-page"} {
		t.Run("TestFlag-"+f, func(t *testing.T) {
			rootCmd := createRootCommand(context.Background(), &Input{}, "")
			err := rootCmd.Flags().Set(f, "true")
			assert.NoError(t, err)
			err = newRunCommand(context.Background(), &Input{
				platforms:     []string{"ubuntu-latest=node:16-buster-slim"},
				workdir:       "../pkg/runner/testdata/",
				workflowsPath: "./basic/push.yml",
			})(rootCmd, []string{})
			assert.NoError(t, err)
		})
	}
}

func TestReadArgsFile(t *testing.T) {
	tables := []struct {
		path  string
		split bool
		args  []string
		env   map[string]string
	}{
		{
			path:  path.Join("testdata", "simple.actrc"),
			split: true,
			args:  []string{"--container-architecture=linux/amd64", "--action-offline-mode"},
		},
		{
			path:  path.Join("testdata", "env.actrc"),
			split: true,
			env: map[string]string{
				"FAKEPWD": "/fake/test/pwd",
				"FOO":     "foo",
			},
			args: []string{
				"--artifact-server-path", "/fake/test/pwd/.artifacts",
				"--env", "FOO=prefix/foo/suffix",
			},
		},
		{
			path:  path.Join("testdata", "split.actrc"),
			split: true,
			args:  []string{"--container-options", "--volume /foo:/bar --volume /baz:/qux --volume /tmp:/tmp"},
		},
	}
	for _, table := range tables {
		t.Run(table.path, func(t *testing.T) {
			for k, v := range table.env {
				t.Setenv(k, v)
			}
			args := readArgsFile(table.path, table.split, false)
			assert.Equal(t, table.args, args)
		})
	}
}

func TestParseWorkdir(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no workdir flag",
			args: []string{"--verbose", "--env-file", ".env"},
			want: ".",
		},
		{
			name: "short flag with space",
			args: []string{"-C", "/path/to/workdir", "--verbose"},
			want: "/path/to/workdir",
		},
		{
			name: "short flag with equals",
			args: []string{"-C=/path/to/workdir", "--verbose"},
			want: "/path/to/workdir",
		},
		{
			name: "long flag with space",
			args: []string{"--directory", "/path/to/workdir", "--verbose"},
			want: "/path/to/workdir",
		},
		{
			name: "long flag with equals",
			args: []string{"--directory=/path/to/workdir", "--verbose"},
			want: "/path/to/workdir",
		},
		{
			name: "relative path",
			args: []string{"-C", "./subdir"},
			want: "./subdir",
		},
		{
			name: "workdir at end",
			args: []string{"--verbose", "-C", "/last"},
			want: "/last",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWorkdir(tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigLocations(t *testing.T) {
	workdir := "/custom/workdir"
	locations := configLocations(workdir)

	assert.Len(t, locations, 3)
	assert.Equal(t, filepath.Join(workdir, ".actrc"), locations[2])
	assert.Contains(t, locations[0], "actrc")
}

func TestInputResolve(t *testing.T) {
	tests := []struct {
		name    string
		workdir string
		path    string
		want    string
	}{
		{
			name:    "absolute path unchanged",
			workdir: "/workdir",
			path:    "/absolute/path",
			want:    "/absolute/path",
		},
		{
			name:    "relative path resolved against workdir",
			workdir: "/workdir",
			path:    "./relative",
			want:    "/workdir/relative",
		},
		{
			name:    "empty path returns empty",
			workdir: "/workdir",
			path:    "",
			want:    "",
		},
		{
			name:    "relative without dot",
			workdir: "/workdir",
			path:    "subdir/file",
			want:    "/workdir/subdir/file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &Input{workdir: tt.workdir}
			got := input.resolve(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadArgsFileWithPathResolution(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		resolve       bool
		checkArgs     map[string]string
		checkAbsPaths map[string]bool
	}{
		{
			name:    "paths resolved relative to config file dir as absolute paths",
			path:    path.Join("testdata", "paths.actrc"),
			resolve: true,
			checkAbsPaths: map[string]bool{
				"--env-file":    true,
				"--secret-file": true,
				"--var-file":    true,
				"--eventpath":   true,
				"-W":            true,
				"-C":            true,
			},
		},
		{
			name:    "paths not resolved when resolvePaths is false",
			path:    path.Join("testdata", "paths.actrc"),
			resolve: false,
			checkArgs: map[string]string{
				"--env-file":    ".env",
				"--secret-file": "./secrets",
				"--var-file":    "subdir/.vars",
				"--eventpath":   "event.json",
				"-W":            "./workflows",
				"-C":            "..",
			},
			checkAbsPaths: map[string]bool{
				"--env-file":    false,
				"--secret-file": false,
				"--var-file":    false,
				"--eventpath":   false,
				"-W":            false,
				"-C":            false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := readArgsFile(tt.path, true, tt.resolve)
			argMap := make(map[string]string)
			for i := 0; i < len(args); i++ {
				arg := args[i]
				if strings.HasPrefix(arg, "-") {
					if strings.Contains(arg, "=") {
						parts := strings.SplitN(arg, "=", 2)
						argMap[parts[0]] = parts[1]
					} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
						argMap[arg] = args[i+1]
						i++
					}
				}
			}
			for key, expectedAbs := range tt.checkAbsPaths {
				actual, ok := argMap[key]
				assert.True(t, ok, "Expected arg %s not found", key)
				if expectedAbs {
					assert.True(t, filepath.IsAbs(actual), "Expected arg %s to be absolute path, got %s", key, actual)
				} else {
					assert.False(t, filepath.IsAbs(actual), "Expected arg %s to be relative path, got %s", key, actual)
				}
			}
			for key, expected := range tt.checkArgs {
				actual, ok := argMap[key]
				assert.True(t, ok, "Expected arg %s not found", key)
				assert.Equal(t, expected, actual, "For arg %s", key)
			}
		})
	}
}

func TestResolvePathArg(t *testing.T) {
	tests := []struct {
		name       string
		argName    string
		value      string
		baseDir    string
		wantAbs    bool
		wantPrefix string
	}{
		{
			name:       "path arg resolved as absolute path",
			argName:    "env-file",
			value:      ".env",
			baseDir:    "/config",
			wantAbs:    true,
			wantPrefix: "/",
		},
		{
			name:       "absolute path unchanged",
			argName:    "env-file",
			value:      "/abs/.env",
			baseDir:    "/config",
			wantAbs:    true,
			wantPrefix: "/abs/.env",
		},
		{
			name:       "non-path arg unchanged",
			argName:    "actor",
			value:      "test",
			baseDir:    "/config",
			wantAbs:    false,
			wantPrefix: "test",
		},
		{
			name:       "empty base dir no resolution",
			argName:    "env-file",
			value:      ".env",
			baseDir:    "",
			wantAbs:    false,
			wantPrefix: ".env",
		},
		{
			name:       "empty value unchanged",
			argName:    "env-file",
			value:      "",
			baseDir:    "/config",
			wantAbs:    false,
			wantPrefix: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePathArg(tt.argName, tt.value, tt.baseDir)
			if tt.wantAbs {
				assert.True(t, filepath.IsAbs(got), "Expected absolute path, got %s", got)
			}
			if tt.wantPrefix != "" && !filepath.IsAbs(tt.wantPrefix) {
				assert.Equal(t, tt.wantPrefix, got)
			} else if tt.wantPrefix == "/abs/.env" {
				assert.Equal(t, tt.wantPrefix, got)
			}
		})
	}
}

func TestCLIOverrideConfig(t *testing.T) {
	configPath := path.Join("testdata", "override.actrc")

	tests := []struct {
		name           string
		cliArgs        []string
		expectedValues map[string]string
	}{
		{
			name:    "CLI --env-file overrides config",
			cliArgs: []string{"--env-file", ".env.from-cli"},
			expectedValues: map[string]string{
				"env-file": ".env.from-cli",
			},
		},
		{
			name:    "CLI -e overrides config eventpath",
			cliArgs: []string{"-e", "event.from-cli.json"},
			expectedValues: map[string]string{
				"eventpath": "event.from-cli.json",
			},
		},
		{
			name:    "CLI --secret-file overrides config",
			cliArgs: []string{"--secret-file", ".secrets.from-cli"},
			expectedValues: map[string]string{
				"secret-file": ".secrets.from-cli",
			},
		},
		{
			name:    "CLI --var-file overrides config",
			cliArgs: []string{"--var-file", ".vars.from-cli"},
			expectedValues: map[string]string{
				"var-file": ".vars.from-cli",
			},
		},
		{
			name:    "CLI -W overrides config workflows",
			cliArgs: []string{"-W", "./workflows-from-cli"},
			expectedValues: map[string]string{
				"workflows": "./workflows-from-cli",
			},
		},
		{
			name:    "CLI -C overrides config directory",
			cliArgs: []string{"-C", "./cli-workdir"},
			expectedValues: map[string]string{
				"directory": "./cli-workdir",
			},
		},
		{
			name:    "multiple CLI overrides",
			cliArgs: []string{
				"--env-file", ".env.from-cli",
				"--secret-file", ".secrets.from-cli",
				"-e", "event.from-cli.json",
			},
			expectedValues: map[string]string{
				"env-file":    ".env.from-cli",
				"secret-file": ".secrets.from-cli",
				"eventpath":   "event.from-cli.json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configArgs := readArgsFile(configPath, true, false)

			mergedArgs := append(configArgs, tt.cliArgs...)

			input := &Input{}
			rootCmd := createRootCommand(context.Background(), input, "")
			err := rootCmd.ParseFlags(mergedArgs)
			assert.NoError(t, err)

			for flagName, expected := range tt.expectedValues {
				actual, err := rootCmd.Flags().GetString(flagName)
				assert.NoError(t, err)
				assert.Equal(t, expected, actual, "Flag %s should be overridden by CLI", flagName)
			}
		})
	}
}

func TestConfigFilePathsAsAbsolute(t *testing.T) {
	configPath := path.Join("testdata", "override.actrc")
	configArgs := readArgsFile(configPath, true, true)

	pathFlags := []string{"--env-file", "--secret-file", "--var-file", "--input-file", "--eventpath", "--workflows", "-C", "-W", "-e"}

	argMap := make(map[string]string)
	for i := 0; i < len(configArgs); i++ {
		arg := configArgs[i]
		for _, flag := range pathFlags {
			if strings.HasPrefix(arg, flag) {
				if strings.HasPrefix(arg, flag+"=") {
					value := strings.TrimPrefix(arg, flag+"=")
					argMap[flag] = value
				} else if i+1 < len(configArgs) {
					argMap[flag] = configArgs[i+1]
					i++
				}
				break
			}
		}
	}

	for flag, value := range argMap {
		assert.True(t, filepath.IsAbs(value),
			"Config file path arg %s should be resolved to absolute path, got %s", flag, value)
	}
}

func TestInputResolveMethods(t *testing.T) {
	tests := []struct {
		name      string
		workdir   string
		field     string
		value     string
		getMethod func(*Input) string
	}{
		{
			name:    "Envfile resolves against workdir",
			workdir: "/workdir",
			field:   "envfile",
			value:   ".env",
			getMethod: func(i *Input) string { return i.Envfile() },
		},
		{
			name:    "Secretfile resolves against workdir",
			workdir: "/workdir",
			field:   "secretfile",
			value:   ".secrets",
			getMethod: func(i *Input) string { return i.Secretfile() },
		},
		{
			name:    "Varfile resolves against workdir",
			workdir: "/workdir",
			field:   "varfile",
			value:   ".vars",
			getMethod: func(i *Input) string { return i.Varfile() },
		},
		{
			name:    "EventPath resolves against workdir",
			workdir: "/workdir",
			field:   "eventPath",
			value:   "event.json",
			getMethod: func(i *Input) string { return i.EventPath() },
		},
		{
			name:    "WorkflowsPath resolves against workdir",
			workdir: "/workdir",
			field:   "workflowsPath",
			value:   "./workflows",
			getMethod: func(i *Input) string { return i.WorkflowsPath() },
		},
		{
			name:    "ActionCachePath resolves against workdir",
			workdir: "/workdir",
			field:   "actionCachePath",
			value:   ".cache/act",
			getMethod: func(i *Input) string { return i.ActionCachePath() },
		},
		{
			name:    "ArtifactServerPath resolves against workdir",
			workdir: "/workdir",
			field:   "artifactServerPath",
			value:   "./artifacts",
			getMethod: func(i *Input) string { return i.ArtifactServerPath() },
		},
		{
			name:    "CacheServerPath resolves against workdir",
			workdir: "/workdir",
			field:   "cacheServerPath",
			value:   "./cache",
			getMethod: func(i *Input) string { return i.CacheServerPath() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &Input{workdir: tt.workdir}
			switch tt.field {
			case "envfile":
				input.envfile = tt.value
			case "secretfile":
				input.secretfile = tt.value
			case "varfile":
				input.varfile = tt.value
			case "eventPath":
				input.eventPath = tt.value
			case "workflowsPath":
				input.workflowsPath = tt.value
			case "actionCachePath":
				input.actionCachePath = tt.value
			case "artifactServerPath":
				input.artifactServerPath = tt.value
			case "cacheServerPath":
				input.cacheServerPath = tt.value
			}

			got := tt.getMethod(input)
			assert.True(t, filepath.IsAbs(got),
				"Expected absolute path for %s, got %s", tt.field, got)
			assert.Contains(t, got, tt.workdir,
				"Expected path to contain workdir %s, got %s", tt.workdir, got)
		})
	}
}
