package main

import (
	_ "embed"
	"strings"

	"github.com/nektos/act/cmd"
	"github.com/nektos/act/pkg/common"
)

var version string

//go:embed VERSION
var embeddedVersion string

func init() {
	if version == "" {
		version = strings.TrimSpace(embeddedVersion)
	}
}

func main() {
	ctx, cancel := common.CreateGracefulJobCancellationContext()
	defer cancel()

	cmd.Execute(ctx, version)
}
