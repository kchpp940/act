package config

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/adrg/xdg"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	raw *RawConfig
}

func NewLoader() *Loader {
	return &Loader{
		raw: NewRawConfig(),
	}
}

func (l *Loader) LoadDefaults() *Loader {
	l.setIfSource(SourceDefault, func() {
		l.raw.Actor = sourcedValue[string]{Value: "nektos/act", Source: SourceDefault}
		l.raw.WorkflowsPath = sourcedValue[string]{Value: "./.github/workflows/", Source: SourceDefault}
		l.raw.RemoteName = sourcedValue[string]{Value: "origin", Source: SourceDefault}
		l.raw.ForcePull = sourcedValue[bool]{Value: true, Source: SourceDefault}
		l.raw.ForceRebuild = sourcedValue[bool]{Value: true, Source: SourceDefault}
		l.raw.UseGitIgnore = sourcedValue[bool]{Value: true, Source: SourceDefault}
		l.raw.GitHubInstance = sourcedValue[string]{Value: "github.com", Source: SourceDefault}
		l.raw.ArtifactServerAddr = sourcedValue[string]{Value: GetOutboundIP(), Source: SourceDefault}
		l.raw.ArtifactServerPort = sourcedValue[string]{Value: "34567", Source: SourceDefault}
		l.raw.CacheServerAddr = sourcedValue[string]{Value: GetOutboundIP(), Source: SourceDefault}
		l.raw.Envfile = sourcedValue[string]{Value: ".env", Source: SourceDefault}
		l.raw.Inputfile = sourcedValue[string]{Value: ".input", Source: SourceDefault}
		l.raw.Secretfile = sourcedValue[string]{Value: ".secrets", Source: SourceDefault}
		l.raw.Varfile = sourcedValue[string]{Value: ".vars", Source: SourceDefault}
		l.raw.NetworkName = sourcedValue[string]{Value: "host", Source: SourceDefault}
		l.raw.ActionCachePath = sourcedValue[string]{Value: defaultActionCachePath(), Source: SourceDefault}
		l.raw.CacheServerPath = sourcedValue[string]{Value: defaultCacheServerPath(), Source: SourceDefault}
	})
	return l
}

type InputAdapter struct {
	Actor                              *string
	Workdir                            *string
	WorkflowsPath                      *string
	AutodetectEvent                    *bool
	EventPath                          *string
	ReuseContainers                    *bool
	BindWorkdir                        *bool
	Secrets                            *[]string
	Vars                               *[]string
	Envs                               *[]string
	Inputs                             *[]string
	Platforms                          *[]string
	Dryrun                             *bool
	ForcePull                          *bool
	ForceRebuild                       *bool
	NoOutput                           *bool
	Envfile                            *string
	Inputfile                          *string
	Secretfile                         *string
	Varfile                            *string
	InsecureSecrets                    *bool
	DefaultBranch                      *string
	Privileged                         *bool
	UsernsMode                         *string
	ContainerArchitecture              *string
	ContainerDaemonSocket              *string
	ContainerOptions                   *string
	NoWorkflowRecurse                  *bool
	UseGitIgnore                       *bool
	GitHubInstance                     *string
	ContainerCapAdd                    *[]string
	ContainerCapDrop                   *[]string
	AutoRemove                         *bool
	ArtifactServerPath                 *string
	ArtifactServerAddr                 *string
	ArtifactServerPort                 *string
	NoCacheServer                      *bool
	CacheServerPath                    *string
	CacheServerExternalURL             *string
	CacheServerAddr                    *string
	CacheServerPort                    *uint16
	JSONLogger                         *bool
	NoSkipCheckout                     *bool
	RemoteName                         *string
	ReplaceGheActionWithGithubCom      *[]string
	ReplaceGheActionTokenWithGithubCom *string
	Matrix                             *[]string
	ActionCachePath                    *string
	ActionOfflineMode                  *bool
	LogPrefixJobID                     *bool
	NetworkName                        *string
	UseNewActionCache                  *bool
	LocalRepository                    *[]string
	ConcurrentJobs                     *int
	Validate                           *bool
	Strict                             *bool
}

func (l *Loader) LoadFromInput(input *InputAdapter) *Loader {
	source := SourceFlags
	if input.Actor != nil {
		l.setIfHigherPriorityString(source, &l.raw.Actor, *input.Actor)
	}
	if input.Workdir != nil {
		l.setIfHigherPriorityString(source, &l.raw.Workdir, *input.Workdir)
	}
	if input.WorkflowsPath != nil {
		l.setIfHigherPriorityString(source, &l.raw.WorkflowsPath, *input.WorkflowsPath)
	}
	if input.AutodetectEvent != nil {
		l.setIfHigherPriorityBool(source, &l.raw.AutodetectEvent, *input.AutodetectEvent)
	}
	if input.EventPath != nil {
		l.setIfHigherPriorityString(source, &l.raw.EventPath, *input.EventPath)
	}
	if input.ReuseContainers != nil {
		l.setIfHigherPriorityBool(source, &l.raw.ReuseContainers, *input.ReuseContainers)
	}
	if input.BindWorkdir != nil {
		l.setIfHigherPriorityBool(source, &l.raw.BindWorkdir, *input.BindWorkdir)
	}
	if input.Secrets != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.Secrets, *input.Secrets)
	}
	if input.Vars != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.Vars, *input.Vars)
	}
	if input.Envs != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.Envs, *input.Envs)
	}
	if input.Inputs != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.Inputs, *input.Inputs)
	}
	if input.Platforms != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.Platforms, *input.Platforms)
	}
	if input.Dryrun != nil {
		l.setIfHigherPriorityBool(source, &l.raw.Dryrun, *input.Dryrun)
	}
	if input.ForcePull != nil {
		l.setIfHigherPriorityBool(source, &l.raw.ForcePull, *input.ForcePull)
	}
	if input.ForceRebuild != nil {
		l.setIfHigherPriorityBool(source, &l.raw.ForceRebuild, *input.ForceRebuild)
	}
	if input.NoOutput != nil {
		l.setIfHigherPriorityBool(source, &l.raw.NoOutput, *input.NoOutput)
	}
	if input.Envfile != nil {
		l.setIfHigherPriorityString(source, &l.raw.Envfile, *input.Envfile)
	}
	if input.Inputfile != nil {
		l.setIfHigherPriorityString(source, &l.raw.Inputfile, *input.Inputfile)
	}
	if input.Secretfile != nil {
		l.setIfHigherPriorityString(source, &l.raw.Secretfile, *input.Secretfile)
	}
	if input.Varfile != nil {
		l.setIfHigherPriorityString(source, &l.raw.Varfile, *input.Varfile)
	}
	if input.InsecureSecrets != nil {
		l.setIfHigherPriorityBool(source, &l.raw.InsecureSecrets, *input.InsecureSecrets)
	}
	if input.DefaultBranch != nil {
		l.setIfHigherPriorityString(source, &l.raw.DefaultBranch, *input.DefaultBranch)
	}
	if input.Privileged != nil {
		l.setIfHigherPriorityBool(source, &l.raw.Privileged, *input.Privileged)
	}
	if input.UsernsMode != nil {
		l.setIfHigherPriorityString(source, &l.raw.UsernsMode, *input.UsernsMode)
	}
	if input.ContainerArchitecture != nil {
		l.setIfHigherPriorityString(source, &l.raw.ContainerArchitecture, *input.ContainerArchitecture)
	}
	if input.ContainerDaemonSocket != nil {
		l.setIfHigherPriorityString(source, &l.raw.ContainerDaemonSocket, *input.ContainerDaemonSocket)
	}
	if input.ContainerOptions != nil {
		l.setIfHigherPriorityString(source, &l.raw.ContainerOptions, *input.ContainerOptions)
	}
	if input.NoWorkflowRecurse != nil {
		l.setIfHigherPriorityBool(source, &l.raw.NoWorkflowRecurse, *input.NoWorkflowRecurse)
	}
	if input.UseGitIgnore != nil {
		l.setIfHigherPriorityBool(source, &l.raw.UseGitIgnore, *input.UseGitIgnore)
	}
	if input.GitHubInstance != nil {
		l.setIfHigherPriorityString(source, &l.raw.GitHubInstance, *input.GitHubInstance)
	}
	if input.ContainerCapAdd != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.ContainerCapAdd, *input.ContainerCapAdd)
	}
	if input.ContainerCapDrop != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.ContainerCapDrop, *input.ContainerCapDrop)
	}
	if input.AutoRemove != nil {
		l.setIfHigherPriorityBool(source, &l.raw.AutoRemove, *input.AutoRemove)
	}
	if input.ArtifactServerPath != nil {
		l.setIfHigherPriorityString(source, &l.raw.ArtifactServerPath, *input.ArtifactServerPath)
	}
	if input.ArtifactServerAddr != nil {
		l.setIfHigherPriorityString(source, &l.raw.ArtifactServerAddr, *input.ArtifactServerAddr)
	}
	if input.ArtifactServerPort != nil {
		l.setIfHigherPriorityString(source, &l.raw.ArtifactServerPort, *input.ArtifactServerPort)
	}
	if input.NoCacheServer != nil {
		l.setIfHigherPriorityBool(source, &l.raw.NoCacheServer, *input.NoCacheServer)
	}
	if input.CacheServerPath != nil {
		l.setIfHigherPriorityString(source, &l.raw.CacheServerPath, *input.CacheServerPath)
	}
	if input.CacheServerExternalURL != nil {
		l.setIfHigherPriorityString(source, &l.raw.CacheServerExternalURL, *input.CacheServerExternalURL)
	}
	if input.CacheServerAddr != nil {
		l.setIfHigherPriorityString(source, &l.raw.CacheServerAddr, *input.CacheServerAddr)
	}
	if input.CacheServerPort != nil {
		l.setIfHigherPriorityUint16(source, &l.raw.CacheServerPort, *input.CacheServerPort)
	}
	if input.JSONLogger != nil {
		l.setIfHigherPriorityBool(source, &l.raw.JSONLogger, *input.JSONLogger)
	}
	if input.NoSkipCheckout != nil {
		l.setIfHigherPriorityBool(source, &l.raw.NoSkipCheckout, *input.NoSkipCheckout)
	}
	if input.RemoteName != nil {
		l.setIfHigherPriorityString(source, &l.raw.RemoteName, *input.RemoteName)
	}
	if input.ReplaceGheActionWithGithubCom != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.ReplaceGheActionWithGithubCom, *input.ReplaceGheActionWithGithubCom)
	}
	if input.ReplaceGheActionTokenWithGithubCom != nil {
		l.setIfHigherPriorityString(source, &l.raw.ReplaceGheActionTokenWithGithubCom, *input.ReplaceGheActionTokenWithGithubCom)
	}
	if input.Matrix != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.Matrix, *input.Matrix)
	}
	if input.ActionCachePath != nil {
		l.setIfHigherPriorityString(source, &l.raw.ActionCachePath, *input.ActionCachePath)
	}
	if input.ActionOfflineMode != nil {
		l.setIfHigherPriorityBool(source, &l.raw.ActionOfflineMode, *input.ActionOfflineMode)
	}
	if input.LogPrefixJobID != nil {
		l.setIfHigherPriorityBool(source, &l.raw.LogPrefixJobID, *input.LogPrefixJobID)
	}
	if input.NetworkName != nil {
		l.setIfHigherPriorityString(source, &l.raw.NetworkName, *input.NetworkName)
	}
	if input.UseNewActionCache != nil {
		l.setIfHigherPriorityBool(source, &l.raw.UseNewActionCache, *input.UseNewActionCache)
	}
	if input.LocalRepository != nil {
		l.setIfHigherPriorityStringSlice(source, &l.raw.LocalRepository, *input.LocalRepository)
	}
	if input.ConcurrentJobs != nil {
		l.setIfHigherPriorityInt(source, &l.raw.ConcurrentJobs, *input.ConcurrentJobs)
	}
	if input.Validate != nil {
		l.setIfHigherPriorityBool(source, &l.raw.Validate, *input.Validate)
	}
	if input.Strict != nil {
		l.setIfHigherPriorityBool(source, &l.raw.Strict, *input.Strict)
	}
	return l
}

func (l *Loader) LoadActrc() *Loader {
	locations := l.configLocations()
	sources := []ConfigSource{SourceActrcGlobal, SourceActrcHome, SourceActrcLocal}

	for i, loc := range locations {
		args := readArgsFile(loc, true)
		if len(args) > 0 {
			log.Debugf("Loaded .actrc from %s", loc)
			l.loadFromArgs(args, sources[i])
		}
	}
	return l
}

func (l *Loader) LoadFlags(args []string) *Loader {
	l.loadFromArgs(args, SourceFlags)
	return l
}

func (l *Loader) LoadEnvFile() *Loader {
	if l.raw.Envfile.Value == "" {
		return l
	}
	path := l.resolvePath(l.raw.Workdir.Value, l.raw.Envfile.Value)
	envs := readEnvsFile(path)
	if len(envs) > 0 {
		log.Debugf("Loaded environment from %s", path)
		envList := make([]string, 0, len(envs))
		for k, v := range envs {
			envList = append(envList, k+"="+v)
		}
		l.setIfHigherPriorityStringSlice(SourceEnvFile, &l.raw.Envs, envList)
	}
	return l
}

func (l *Loader) LoadSecretFile(ctx context.Context) *Loader {
	if l.raw.Secretfile.Value == "" {
		return l
	}
	path := l.resolvePath(l.raw.Workdir.Value, l.raw.Secretfile.Value)
	secrets := readEnvsFileEx(path, true)
	if len(secrets) > 0 {
		log.Debugf("Loaded secrets from %s", path)
		secretList := make([]string, 0, len(secrets))
		for k, v := range secrets {
			secretList = append(secretList, k+"="+v)
		}
		l.setIfHigherPriorityStringSlice(SourceSecretFile, &l.raw.Secrets, secretList)
	}
	return l
}

func (l *Loader) LoadVarFile() *Loader {
	if l.raw.Varfile.Value == "" {
		return l
	}
	path := l.resolvePath(l.raw.Workdir.Value, l.raw.Varfile.Value)
	vars := readEnvsFile(path)
	if len(vars) > 0 {
		log.Debugf("Loaded vars from %s", path)
		varList := make([]string, 0, len(vars))
		for k, v := range vars {
			varList = append(varList, k+"="+v)
		}
		l.setIfHigherPriorityStringSlice(SourceVarFile, &l.raw.Vars, varList)
	}
	return l
}

func (l *Loader) LoadInputFile() *Loader {
	if l.raw.Inputfile.Value == "" {
		return l
	}
	path := l.resolvePath(l.raw.Workdir.Value, l.raw.Inputfile.Value)
	inputs := readEnvsFile(path)
	if len(inputs) > 0 {
		log.Debugf("Loaded inputs from %s", path)
		inputList := make([]string, 0, len(inputs))
		for k, v := range inputs {
			inputList = append(inputList, k+"="+v)
		}
		l.setIfHigherPriorityStringSlice(SourceInputFile, &l.raw.Inputs, inputList)
	}
	return l
}

func (l *Loader) RawConfig() *RawConfig {
	return l.raw
}

func (l *Loader) configLocations() []string {
	configFileName := ".actrc"
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	homePath := filepath.Join(home, configFileName)
	invocationPath := filepath.Join(".", configFileName)

	specPath, err := xdg.ConfigFile("act/actrc")
	if err != nil {
		specPath = homePath
	}

	return []string{specPath, homePath, invocationPath}
}

func (l *Loader) loadFromArgs(args []string, source ConfigSource) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		parts := regexp.MustCompile(`\s`).Split(arg, 2)
		flag := parts[0]
		var value string
		if len(parts) > 1 {
			value = parts[1]
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			value = args[i]
		}

		l.setFlagValue(flag, value, source)
	}
}

func (l *Loader) setFlagValue(flag, value string, source ConfigSource) {
	switch flag {
	case "-a", "--actor":
		l.setString(flag, value, source, &l.raw.Actor)
	case "-C", "--directory":
		l.setString(flag, value, source, &l.raw.Workdir)
	case "-W", "--workflows":
		l.setString(flag, value, source, &l.raw.WorkflowsPath)
	case "--detect-event":
		l.setBool(flag, true, source, &l.raw.AutodetectEvent)
	case "-e", "--eventpath":
		l.setString(flag, value, source, &l.raw.EventPath)
	case "-r", "--reuse":
		l.setBool(flag, true, source, &l.raw.ReuseContainers)
	case "-b", "--bind":
		l.setBool(flag, true, source, &l.raw.BindWorkdir)
	case "-s", "--secret":
		l.setStringSlice(flag, value, source, &l.raw.Secrets)
	case "--var":
		l.setStringSlice(flag, value, source, &l.raw.Vars)
	case "--env":
		l.setStringSlice(flag, value, source, &l.raw.Envs)
	case "--input":
		l.setStringSlice(flag, value, source, &l.raw.Inputs)
	case "-P", "--platform":
		l.setStringSlice(flag, value, source, &l.raw.Platforms)
	case "-n", "--dryrun":
		l.setBool(flag, true, source, &l.raw.Dryrun)
	case "-p", "--pull":
		l.setBool(flag, value != "false", source, &l.raw.ForcePull)
	case "--rebuild":
		l.setBool(flag, value != "false", source, &l.raw.ForceRebuild)
	case "-q", "--quiet":
		l.setBool(flag, true, source, &l.raw.NoOutput)
	case "--env-file":
		l.setString(flag, value, source, &l.raw.Envfile)
	case "--input-file":
		l.setString(flag, value, source, &l.raw.Inputfile)
	case "--secret-file":
		l.setString(flag, value, source, &l.raw.Secretfile)
	case "--var-file":
		l.setString(flag, value, source, &l.raw.Varfile)
	case "--insecure-secrets":
		l.setBool(flag, true, source, &l.raw.InsecureSecrets)
	case "--defaultbranch":
		l.setString(flag, value, source, &l.raw.DefaultBranch)
	case "--privileged":
		l.setBool(flag, true, source, &l.raw.Privileged)
	case "--userns":
		l.setString(flag, value, source, &l.raw.UsernsMode)
	case "--container-architecture":
		l.setString(flag, value, source, &l.raw.ContainerArchitecture)
	case "--container-daemon-socket":
		l.setString(flag, value, source, &l.raw.ContainerDaemonSocket)
	case "--container-options":
		l.setString(flag, value, source, &l.raw.ContainerOptions)
	case "--no-recurse":
		l.setBool(flag, true, source, &l.raw.NoWorkflowRecurse)
	case "--use-gitignore":
		l.setBool(flag, value != "false", source, &l.raw.UseGitIgnore)
	case "--github-instance":
		l.setString(flag, value, source, &l.raw.GitHubInstance)
	case "--container-cap-add":
		l.setStringSlice(flag, value, source, &l.raw.ContainerCapAdd)
	case "--container-cap-drop":
		l.setStringSlice(flag, value, source, &l.raw.ContainerCapDrop)
	case "--rm":
		l.setBool(flag, true, source, &l.raw.AutoRemove)
	case "--artifact-server-path":
		l.setString(flag, value, source, &l.raw.ArtifactServerPath)
	case "--artifact-server-addr":
		l.setString(flag, value, source, &l.raw.ArtifactServerAddr)
	case "--artifact-server-port":
		l.setString(flag, value, source, &l.raw.ArtifactServerPort)
	case "--no-cache-server":
		l.setBool(flag, true, source, &l.raw.NoCacheServer)
	case "--cache-server-path":
		l.setString(flag, value, source, &l.raw.CacheServerPath)
	case "--cache-server-external-url":
		l.setString(flag, value, source, &l.raw.CacheServerExternalURL)
	case "--cache-server-addr":
		l.setString(flag, value, source, &l.raw.CacheServerAddr)
	case "--cache-server-port":
		l.setUint16(flag, value, source, &l.raw.CacheServerPort)
	case "--json":
		l.setBool(flag, true, source, &l.raw.JSONLogger)
	case "--no-skip-checkout":
		l.setBool(flag, true, source, &l.raw.NoSkipCheckout)
	case "--remote-name":
		l.setString(flag, value, source, &l.raw.RemoteName)
	case "--replace-ghe-action-with-github-com":
		l.setStringSlice(flag, value, source, &l.raw.ReplaceGheActionWithGithubCom)
	case "--replace-ghe-action-token-with-github-com":
		l.setString(flag, value, source, &l.raw.ReplaceGheActionTokenWithGithubCom)
	case "--matrix":
		l.setStringSlice(flag, value, source, &l.raw.Matrix)
	case "--action-cache-path":
		l.setString(flag, value, source, &l.raw.ActionCachePath)
	case "--action-offline-mode":
		l.setBool(flag, true, source, &l.raw.ActionOfflineMode)
	case "--log-prefix-job-id":
		l.setBool(flag, true, source, &l.raw.LogPrefixJobID)
	case "--network":
		l.setString(flag, value, source, &l.raw.NetworkName)
	case "--use-new-action-cache":
		l.setBool(flag, true, source, &l.raw.UseNewActionCache)
	case "--local-repository":
		l.setStringSlice(flag, value, source, &l.raw.LocalRepository)
	case "--concurrent-jobs":
		l.setInt(flag, value, source, &l.raw.ConcurrentJobs)
	case "--validate":
		l.setBool(flag, true, source, &l.raw.Validate)
	case "--strict":
		l.setBool(flag, true, source, &l.raw.Strict)
	}
}

func (l *Loader) setIfSource(source ConfigSource, fn func()) {
	fn()
}

func (l *Loader) setString(flag, value string, source ConfigSource, target *sourcedValue[string]) {
	if source >= target.Source {
		*target = sourcedValue[string]{Value: value, Source: source}
	}
}

func (l *Loader) setBool(flag string, value bool, source ConfigSource, target *sourcedValue[bool]) {
	if source >= target.Source {
		*target = sourcedValue[bool]{Value: value, Source: source}
	}
}

func (l *Loader) setInt(flag, value string, source ConfigSource, target *sourcedValue[int]) {
	if source >= target.Source {
		var intValue int
		if _, err := os.Stdout.WriteString(""); err != nil {
			return
		}
		*target = sourcedValue[int]{Value: intValue, Source: source}
	}
}

func (l *Loader) setUint16(flag, value string, source ConfigSource, target *sourcedValue[uint16]) {
	if source >= target.Source {
		var uintValue uint16
		*target = sourcedValue[uint16]{Value: uintValue, Source: source}
	}
}

func (l *Loader) setStringSlice(flag, value string, source ConfigSource, target *sourcedValue[[]string]) {
	if source >= target.Source {
		if target.Source != source {
			*target = sourcedValue[[]string]{Value: []string{}, Source: source}
		}
		if value != "" {
			target.Value = append(target.Value, value)
		}
	}
}

func (l *Loader) setIfHigherPriorityString(source ConfigSource, target *sourcedValue[string], value string) {
	if source >= target.Source {
		*target = sourcedValue[string]{Value: value, Source: source}
	}
}

func (l *Loader) setIfHigherPriorityBool(source ConfigSource, target *sourcedValue[bool], value bool) {
	if source >= target.Source {
		*target = sourcedValue[bool]{Value: value, Source: source}
	}
}

func (l *Loader) setIfHigherPriorityStringSlice(source ConfigSource, target *sourcedValue[[]string], value []string) {
	if source >= target.Source {
		*target = sourcedValue[[]string]{Value: value, Source: source}
	}
}

func (l *Loader) setIfHigherPriorityUint16(source ConfigSource, target *sourcedValue[uint16], value uint16) {
	if source >= target.Source {
		*target = sourcedValue[uint16]{Value: value, Source: source}
	}
}

func (l *Loader) setIfHigherPriorityInt(source ConfigSource, target *sourcedValue[int], value int) {
	if source >= target.Source {
		*target = sourcedValue[int]{Value: value, Source: source}
	}
}

func (l *Loader) resolvePath(base, path string) string {
	if path == "" {
		return path
	}
	if !filepath.IsAbs(path) {
		absBase, err := filepath.Abs(base)
		if err != nil {
			absBase = base
		}
		path = filepath.Join(absBase, path)
	}
	return path
}

func readArgsFile(file string, split bool) []string {
	args := make([]string, 0)
	f, err := os.Open(file)
	if err != nil {
		return args
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 1024*1024*1024)
	for scanner.Scan() {
		arg := os.ExpandEnv(strings.TrimSpace(scanner.Text()))
		if strings.HasPrefix(arg, "-") && split {
			args = append(args, regexp.MustCompile(`\s`).Split(arg, 2)...)
		} else if !split {
			args = append(args, arg)
		}
	}
	return args
}

func readEnvsFile(path string) map[string]string {
	return readEnvsFileEx(path, false)
}

func readEnvsFileEx(path string, caseInsensitive bool) map[string]string {
	envs := make(map[string]string)
	if _, err := os.Stat(path); err == nil {
		var env map[string]string
		var err error
		if ext := filepath.Ext(path); ext == ".yml" || ext == ".yaml" {
			env, err = readYamlFile(path)
		} else {
			env, err = godotenv.Read(path)
		}
		if err != nil {
			log.Fatalf("Error loading from %s: %v", path, err)
		}
		for k, v := range env {
			if caseInsensitive {
				k = strings.ToUpper(k)
			}
			if _, ok := envs[k]; !ok {
				envs[k] = v
			}
		}
	}
	return envs
}

func readYamlFile(file string) (map[string]string, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	ret := map[string]string{}
	if err = yaml.Unmarshal(content, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func getOutboundIP() string {
	return "127.0.0.1"
}

func defaultActionCachePath() string {
	cacheDir := defaultCacheDir()
	return filepath.Join(cacheDir, "act")
}

func defaultCacheServerPath() string {
	cacheDir := defaultCacheDir()
	return filepath.Join(cacheDir, "actcache")
}

func defaultCacheDir() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, ".cache")
}

func newLoader() *Loader {
	return NewLoader()
}

func defaultImageSurvey(actrc string) error {
	var answer string
	confirmation := &survey.Select{
		Message: "Please choose the default image you want to use with act:\n  - Large size image: ca. 17GB download + 53.1GB storage, you will need 75GB of free disk space, snapshots of GitHub Hosted Runners without snap and pulled docker images\n  - Medium size image: ~500MB, includes only necessary tools to bootstrap actions and aims to be compatible with most actions\n  - Micro size image: <200MB, contains only NodeJS required to bootstrap actions, doesn't work with all actions\n\nDefault image and other options can be changed manually in " + newLoader().configLocations()[0] + " (please refer to https://nektosact.com/usage/index.html?highlight=configur#configuration-file for additional information about file structure)",
		Help:    "If you want to know why act asks you that, please go to https://github.com/nektos/act/issues/107",
		Default: "Medium",
		Options: []string{"Large", "Medium", "Micro"},
	}

	err := survey.AskOne(confirmation, &answer)
	if err != nil {
		return err
	}

	var option string
	switch answer {
	case "Large":
		option = "-P ubuntu-latest=catthehacker/ubuntu:full-latest\n-P ubuntu-22.04=catthehacker/ubuntu:full-22.04\n-P ubuntu-20.04=catthehacker/ubuntu:full-20.04\n-P ubuntu-18.04=catthehacker/ubuntu:full-18.04\n"
	case "Medium":
		option = "-P ubuntu-latest=catthehacker/ubuntu:act-latest\n-P ubuntu-22.04=catthehacker/ubuntu:act-22.04\n-P ubuntu-20.04=catthehacker/ubuntu:act-20.04\n-P ubuntu-18.04=catthehacker/ubuntu:act-18.04\n"
	case "Micro":
		option = "-P ubuntu-latest=node:16-buster-slim\n-P ubuntu-22.04=node:16-bullseye-slim\n-P ubuntu-20.04=node:16-buster-slim\n-P ubuntu-18.04=node:16-buster-slim\n"
	}

	f, err := os.Create(actrc)
	if err != nil {
		return err
	}

	_, err = f.WriteString(option)
	if err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}

func parsePlatforms(platformList []string) map[string]string {
	platforms := map[string]string{
		"ubuntu-latest": "node:16-buster-slim",
		"ubuntu-22.04":  "node:16-bullseye-slim",
		"ubuntu-20.04":  "node:16-buster-slim",
		"ubuntu-18.04":  "node:16-buster-slim",
	}

	for _, p := range platformList {
		pParts := strings.Split(p, "=")
		if len(pParts) == 2 {
			platforms[strings.ToLower(pParts[0])] = pParts[1]
		}
	}
	return platforms
}
