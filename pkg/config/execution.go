package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	docker_container "github.com/moby/moby/api/types/container"
	log "github.com/sirupsen/logrus"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/container"
	"github.com/nektos/act/pkg/gh"
)

type ExecutionConfig struct {
	Actor                              string
	Workdir                            string
	WorkflowsPath                      string
	AutodetectEvent                    bool
	EventName                          string
	EventPath                          string
	EventJSON                          string
	DefaultBranch                      string
	ForcePull                          bool
	ForceRebuild                       bool
	ReuseContainers                    bool
	BindWorkdir                        bool
	LogOutput                          bool
	JSONLogger                         bool
	LogPrefixJobID                     bool
	Dryrun                             bool
	Env                                map[string]string
	Inputs                             map[string]string
	Secrets                            map[string]string
	Vars                               map[string]string
	Token                              string
	InsecureSecrets                    bool
	Platforms                          map[string]string
	Privileged                         bool
	UsernsMode                         string
	ContainerArchitecture              string
	ContainerDaemonSocket              string
	ContainerOptions                   string
	UseGitIgnore                       bool
	GitHubInstance                     string
	ContainerCapAdd                    []string
	ContainerCapDrop                   []string
	AutoRemove                         bool
	ArtifactServerPath                 string
	ArtifactServerAddr                 string
	ArtifactServerPort                 string
	NoSkipCheckout                     bool
	RemoteName                         string
	ReplaceGheActionWithGithubCom      []string
	ReplaceGheActionTokenWithGithubCom string
	Matrix                             map[string]map[string]bool
	ContainerNetworkMode               docker_container.NetworkMode
	ConcurrentJobs                     int
	ActionCacheDir                     string
	ActionOfflineMode                  bool
	NoCacheServer                      bool
	CacheServerPath                    string
	CacheServerExternalURL             string
	CacheServerAddr                    string
	CacheServerPort                    uint16
	UseNewActionCache                  bool
	LocalRepository                    map[string]string
	Validate                           bool
	Strict                             bool
	NoWorkflowRecurse                  bool

	Raw *RawConfig
}

type Normalizer struct {
	raw *RawConfig
	ctx context.Context
}

func NewNormalizer(ctx context.Context, raw *RawConfig) *Normalizer {
	return &Normalizer{
		raw: raw,
		ctx: ctx,
	}
}

func (n *Normalizer) Normalize() (*ExecutionConfig, error) {
	ec := &ExecutionConfig{
		Raw: n.raw,
	}

	if err := n.normalizePaths(ec); err != nil {
		return nil, err
	}

	if err := n.normalizeMaps(ec); err != nil {
		return nil, err
	}

	if err := n.normalizeMisc(ec); err != nil {
		return nil, err
	}

	if err := n.normalizeDocker(ec); err != nil {
		return nil, err
	}

	return ec, nil
}

func (n *Normalizer) normalizePaths(ec *ExecutionConfig) error {
	workdir := n.raw.Workdir.Value
	if workdir == "" {
		workdir = "."
	}
	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return fmt.Errorf("failed to resolve workdir: %w", err)
	}
	ec.Workdir = absWorkdir

	ec.WorkflowsPath = n.resolvePath(absWorkdir, n.raw.WorkflowsPath.Value)
	ec.EventPath = n.resolvePath(absWorkdir, n.raw.EventPath.Value)
	ec.Actor = n.raw.Actor.Value
	ec.AutodetectEvent = n.raw.AutodetectEvent.Value
	ec.DefaultBranch = n.raw.DefaultBranch.Value
	ec.RemoteName = n.raw.RemoteName.Value
	ec.GitHubInstance = n.raw.GitHubInstance.Value

	return nil
}

func (n *Normalizer) normalizeMaps(ec *ExecutionConfig) error {
	envs := parseEnvs(n.raw.Envs.Value)
	envfilePath := n.resolvePath(ec.Workdir, n.raw.Envfile.Value)
	readEnvsFileToMap(envfilePath, envs, false)
	ec.Env = envs

	inputs := parseEnvs(n.raw.Inputs.Value)
	inputfilePath := n.resolvePath(ec.Workdir, n.raw.Inputfile.Value)
	readEnvsFileToMap(inputfilePath, inputs, false)
	ec.Inputs = inputs

	secrets := n.parseSecrets(n.raw.Secrets.Value)
	secretfilePath := n.resolvePath(ec.Workdir, n.raw.Secretfile.Value)
	readEnvsFileToMap(secretfilePath, secrets, true)

	if _, hasGitHubToken := secrets["GITHUB_TOKEN"]; !hasGitHubToken {
		ctx, cancel := common.EarlyCancelContext(n.ctx)
		defer cancel()
		secrets["GITHUB_TOKEN"], _ = gh.GetToken(ctx, "")
	}
	ec.Secrets = secrets
	ec.Token = secrets["GITHUB_TOKEN"]

	vars := n.parseSecrets(n.raw.Vars.Value)
	varfilePath := n.resolvePath(ec.Workdir, n.raw.Varfile.Value)
	readEnvsFileToMap(varfilePath, vars, false)
	ec.Vars = vars

	ec.Matrix = parseMatrix(n.raw.Matrix.Value)
	ec.Platforms = n.parsePlatforms(n.raw.Platforms.Value)

	ec.LocalRepository = make(map[string]string)
	for _, l := range n.raw.LocalRepository.Value {
		k, v, _ := strings.Cut(l, "=")
		ec.LocalRepository[k] = v
	}

	return nil
}

func (n *Normalizer) normalizeMisc(ec *ExecutionConfig) error {
	ec.ForcePull = !n.raw.ActionOfflineMode.Value && n.raw.ForcePull.Value
	ec.ForceRebuild = n.raw.ForceRebuild.Value
	ec.ReuseContainers = n.raw.ReuseContainers.Value
	ec.BindWorkdir = n.raw.BindWorkdir.Value
	ec.LogOutput = !n.raw.NoOutput.Value
	ec.JSONLogger = n.raw.JSONLogger.Value
	ec.LogPrefixJobID = n.raw.LogPrefixJobID.Value
	ec.Dryrun = n.raw.Dryrun.Value
	ec.InsecureSecrets = n.raw.InsecureSecrets.Value
	ec.Privileged = n.raw.Privileged.Value
	ec.UsernsMode = n.raw.UsernsMode.Value
	ec.ContainerArchitecture = n.raw.ContainerArchitecture.Value
	ec.ContainerDaemonSocket = n.raw.ContainerDaemonSocket.Value
	ec.ContainerOptions = n.raw.ContainerOptions.Value
	ec.UseGitIgnore = n.raw.UseGitIgnore.Value
	ec.ContainerCapAdd = n.raw.ContainerCapAdd.Value
	ec.ContainerCapDrop = n.raw.ContainerCapDrop.Value
	ec.AutoRemove = n.raw.AutoRemove.Value
	ec.ArtifactServerPath = n.raw.ArtifactServerPath.Value
	ec.ArtifactServerAddr = n.raw.ArtifactServerAddr.Value
	ec.ArtifactServerPort = n.raw.ArtifactServerPort.Value
	ec.NoSkipCheckout = n.raw.NoSkipCheckout.Value
	ec.ReplaceGheActionWithGithubCom = n.raw.ReplaceGheActionWithGithubCom.Value
	ec.ReplaceGheActionTokenWithGithubCom = n.raw.ReplaceGheActionTokenWithGithubCom.Value
	ec.ActionCacheDir = n.raw.ActionCachePath.Value
	ec.ActionOfflineMode = n.raw.ActionOfflineMode.Value
	ec.NoCacheServer = n.raw.NoCacheServer.Value
	ec.CacheServerPath = n.raw.CacheServerPath.Value
	ec.CacheServerExternalURL = n.raw.CacheServerExternalURL.Value
	ec.CacheServerAddr = n.raw.CacheServerAddr.Value
	ec.CacheServerPort = n.raw.CacheServerPort.Value
	ec.UseNewActionCache = n.raw.UseNewActionCache.Value
	ec.Validate = n.raw.Validate.Value
	ec.Strict = n.raw.Strict.Value
	ec.NoWorkflowRecurse = n.raw.NoWorkflowRecurse.Value
	ec.ConcurrentJobs = n.raw.ConcurrentJobs.Value

	if n.raw.NetworkName.Value != "" {
		ec.ContainerNetworkMode = docker_container.NetworkMode(n.raw.NetworkName.Value)
	}

	return nil
}

func (n *Normalizer) normalizeDocker(ec *ExecutionConfig) error {
	if ec.ContainerDaemonSocket != "" {
		if ret, err := container.GetSocketAndHost(ec.ContainerDaemonSocket); err != nil {
			log.Warnf("Couldn't get a valid docker connection: %+v", err)
		} else {
			os.Setenv("DOCKER_HOST", ret.Host)
			ec.ContainerDaemonSocket = ret.Socket
			log.Infof("Using docker host '%s', and daemon socket '%s'", ret.Host, ret.Socket)
		}
	}
	return nil
}

func (n *Normalizer) resolvePath(base, path string) string {
	if path == "" {
		return path
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return path
}

func (n *Normalizer) parseSecrets(secretList []string) map[string]string {
	s := make(map[string]string)
	for _, secretPair := range secretList {
		secretPairParts := strings.SplitN(secretPair, "=", 2)
		secretPairParts[0] = strings.ToUpper(secretPairParts[0])
		if strings.ToUpper(s[secretPairParts[0]]) == secretPairParts[0] {
			log.Errorf("Secret %s is already defined (secrets are case insensitive)", secretPairParts[0])
		}
		if len(secretPairParts) == 2 {
			s[secretPairParts[0]] = secretPairParts[1]
		} else if env, ok := os.LookupEnv(secretPairParts[0]); ok && env != "" {
			s[secretPairParts[0]] = env
		}
	}
	return s
}

func (n *Normalizer) parsePlatforms(platformList []string) map[string]string {
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

func ParseEnvs(env []string) map[string]string {
	return parseEnvs(env)
}

func ParseMatrix(matrix []string) map[string]map[string]bool {
	return parseMatrix(matrix)
}

func parseEnvs(env []string) map[string]string {
	envs := make(map[string]string, len(env))
	for _, envVar := range env {
		e := strings.SplitN(envVar, "=", 2)
		if len(e) == 2 {
			envs[e[0]] = e[1]
		} else {
			envs[e[0]] = ""
		}
	}
	return envs
}

func parseMatrix(matrix []string) map[string]map[string]bool {
	r := regexp.MustCompile(":")
	matrixes := make(map[string]map[string]bool)
	for _, m := range matrix {
		matrix := r.Split(m, 2)
		if len(matrix) < 2 {
			log.Fatalf("Invalid matrix format. Failed to parse %s", m)
		}
		if _, ok := matrixes[matrix[0]]; !ok {
			matrixes[matrix[0]] = make(map[string]bool)
		}
		matrixes[matrix[0]][matrix[1]] = true
	}
	return matrixes
}

func readEnvsFileToMap(path string, target map[string]string, caseInsensitive bool) {
	if _, err := os.Stat(path); err == nil {
		var env map[string]string
		var err error
		if ext := filepath.Ext(path); ext == ".yml" || ext == ".yaml" {
			env, err = readYamlFile(path)
		} else {
			env, err = readEnvsFromFile(path)
		}
		if err != nil {
			log.Fatalf("Error loading from %s: %v", path, err)
		}
		for k, v := range env {
			if caseInsensitive {
				k = strings.ToUpper(k)
			}
			if _, ok := target[k]; !ok {
				target[k] = v
			}
		}
	}
}

func readEnvsFromFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	envs := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			envs[key] = os.ExpandEnv(value)
		}
	}
	return envs, nil
}
