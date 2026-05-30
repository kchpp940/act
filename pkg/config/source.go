package config

type ConfigSource int

const (
	SourceDefault ConfigSource = iota
	SourceActrcGlobal
	SourceActrcHome
	SourceActrcLocal
	SourceEnvFile
	SourceSecretFile
	SourceVarFile
	SourceInputFile
	SourceEventFile
	SourceFlags
)

func (s ConfigSource) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceActrcGlobal:
		return "actrc-global"
	case SourceActrcHome:
		return "actrc-home"
	case SourceActrcLocal:
		return "actrc-local"
	case SourceEnvFile:
		return "env-file"
	case SourceSecretFile:
		return "secret-file"
	case SourceVarFile:
		return "var-file"
	case SourceInputFile:
		return "input-file"
	case SourceEventFile:
		return "event-file"
	case SourceFlags:
		return "flags"
	default:
		return "unknown"
	}
}

type sourcedValue[T any] struct {
	Value  T
	Source ConfigSource
}

type RawConfig struct {
	Actor                              sourcedValue[string]
	Workdir                            sourcedValue[string]
	WorkflowsPath                      sourcedValue[string]
	AutodetectEvent                    sourcedValue[bool]
	EventPath                          sourcedValue[string]
	ReuseContainers                    sourcedValue[bool]
	BindWorkdir                        sourcedValue[bool]
	Secrets                            sourcedValue[[]string]
	Vars                               sourcedValue[[]string]
	Envs                               sourcedValue[[]string]
	Inputs                             sourcedValue[[]string]
	Platforms                          sourcedValue[[]string]
	Dryrun                             sourcedValue[bool]
	ForcePull                          sourcedValue[bool]
	ForceRebuild                       sourcedValue[bool]
	NoOutput                           sourcedValue[bool]
	Envfile                            sourcedValue[string]
	Inputfile                          sourcedValue[string]
	Secretfile                         sourcedValue[string]
	Varfile                            sourcedValue[string]
	InsecureSecrets                    sourcedValue[bool]
	DefaultBranch                      sourcedValue[string]
	Privileged                         sourcedValue[bool]
	UsernsMode                         sourcedValue[string]
	ContainerArchitecture              sourcedValue[string]
	ContainerDaemonSocket              sourcedValue[string]
	ContainerOptions                   sourcedValue[string]
	NoWorkflowRecurse                  sourcedValue[bool]
	UseGitIgnore                       sourcedValue[bool]
	GitHubInstance                     sourcedValue[string]
	ContainerCapAdd                    sourcedValue[[]string]
	ContainerCapDrop                   sourcedValue[[]string]
	AutoRemove                         sourcedValue[bool]
	ArtifactServerPath                 sourcedValue[string]
	ArtifactServerAddr                 sourcedValue[string]
	ArtifactServerPort                 sourcedValue[string]
	NoCacheServer                      sourcedValue[bool]
	CacheServerPath                    sourcedValue[string]
	CacheServerExternalURL             sourcedValue[string]
	CacheServerAddr                    sourcedValue[string]
	CacheServerPort                    sourcedValue[uint16]
	JSONLogger                         sourcedValue[bool]
	NoSkipCheckout                     sourcedValue[bool]
	RemoteName                         sourcedValue[string]
	ReplaceGheActionWithGithubCom      sourcedValue[[]string]
	ReplaceGheActionTokenWithGithubCom sourcedValue[string]
	Matrix                             sourcedValue[[]string]
	ActionCachePath                    sourcedValue[string]
	ActionOfflineMode                  sourcedValue[bool]
	LogPrefixJobID                     sourcedValue[bool]
	NetworkName                        sourcedValue[string]
	UseNewActionCache                  sourcedValue[bool]
	LocalRepository                    sourcedValue[[]string]
	ConcurrentJobs                     sourcedValue[int]
	Validate                           sourcedValue[bool]
	Strict                             sourcedValue[bool]
}

func NewRawConfig() *RawConfig {
	return &RawConfig{}
}
