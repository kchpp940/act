package config

import (
	"context"

	"github.com/nektos/act/pkg/common"
)

type Pipeline struct {
	ctx    context.Context
	loader *Loader
	raw    *RawConfig
}

func NewPipeline(ctx context.Context) *Pipeline {
	return &Pipeline{
		ctx:    ctx,
		loader: NewLoader(),
	}
}

func (p *Pipeline) LoadDefaults() *Pipeline {
	p.loader.LoadDefaults()
	return p
}

func (p *Pipeline) LoadActrc() *Pipeline {
	p.loader.LoadActrc()
	return p
}

func (p *Pipeline) LoadFlags(args []string) *Pipeline {
	p.loader.LoadFlags(args)
	return p
}

func (p *Pipeline) LoadFiles() *Pipeline {
	p.raw = p.loader.RawConfig()
	p.loader.LoadEnvFile()
	p.loader.LoadSecretFile(p.ctx)
	p.loader.LoadVarFile()
	p.loader.LoadInputFile()
	return p
}

func (p *Pipeline) LoadFromInput(adapter *InputAdapter) *Pipeline {
	p.loader.LoadFromInput(adapter)
	return p
}

func (p *Pipeline) Normalize() (*ExecutionConfig, error) {
	if p.raw == nil {
		p.raw = p.loader.RawConfig()
	}
	normalizer := NewNormalizer(p.ctx, p.raw)
	return normalizer.Normalize()
}

func (p *Pipeline) FullPipeline(args []string) (*ExecutionConfig, error) {
	return p.LoadDefaults().
		LoadActrc().
		LoadFlags(args).
		LoadFiles().
		Normalize()
}

func (p *Pipeline) RawConfig() *RawConfig {
	if p.raw == nil {
		p.raw = p.loader.RawConfig()
	}
	return p.raw
}

func LoadConfig(ctx context.Context, args []string) (*ExecutionConfig, error) {
	pipeline := NewPipeline(ctx)
	return pipeline.FullPipeline(args)
}

func GetOutboundIP() string {
	ip := common.GetOutboundIP()
	if ip != nil {
		return ip.String()
	}
	return "127.0.0.1"
}

func ConfigLocations() []string {
	return newLoader().configLocations()
}

func ReadArgsFile(file string, split bool) []string {
	return readArgsFile(file, split)
}

func DefaultImageSurvey(actrc string) error {
	return defaultImageSurvey(actrc)
}

func ParsePlatforms(platformList []string) map[string]string {
	return parsePlatforms(platformList)
}
