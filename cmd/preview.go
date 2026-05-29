package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	docker_container "github.com/moby/moby/api/types/container"
	"github.com/nektos/act/pkg/model"
	"github.com/nektos/act/pkg/runner"
	log "github.com/sirupsen/logrus"
)

func newPreviewPlan(ctx context.Context, input *Input, plan *model.Plan, eventName string, defaultBranch string, envs map[string]string, secrets map[string]string, vars map[string]string, inputs map[string]string, matrixes map[string]map[string]bool) (*runner.PreviewPlan, error) {
	workflowsPath := input.WorkflowsPath()

	platforms := make(map[string]string)
	for _, p := range input.platforms {
		k, v, _ := strings.Cut(p, "=")
		if k != "" && v != "" {
			platforms[k] = v
		}
	}

	cacheDir := input.actionCachePath

	config := &runner.Config{
		Actor:                              input.actor,
		EventName:                          eventName,
		EventPath:                          input.EventPath(),
		DefaultBranch:                      defaultBranch,
		ForcePull:                          !input.actionOfflineMode && input.forcePull,
		ForceRebuild:                       input.forceRebuild,
		ReuseContainers:                    input.reuseContainers,
		Workdir:                            input.Workdir(),
		ActionCacheDir:                     cacheDir,
		ActionOfflineMode:                  input.actionOfflineMode,
		BindWorkdir:                        input.bindWorkdir,
		LogOutput:                          !input.noOutput,
		JSONLogger:                         input.jsonLogger,
		LogPrefixJobID:                     input.logPrefixJobID,
		Env:                                envs,
		Secrets:                            secrets,
		Vars:                               vars,
		Inputs:                             inputs,
		Token:                              secrets["GITHUB_TOKEN"],
		InsecureSecrets:                    input.insecureSecrets,
		Platforms:                          platforms,
		Privileged:                         input.privileged,
		UsernsMode:                         input.usernsMode,
		ContainerArchitecture:              input.containerArchitecture,
		ContainerDaemonSocket:              input.containerDaemonSocket,
		ContainerOptions:                   input.containerOptions,
		UseGitIgnore:                       input.useGitIgnore,
		GitHubInstance:                     input.githubInstance,
		ContainerCapAdd:                    input.containerCapAdd,
		ContainerCapDrop:                   input.containerCapDrop,
		AutoRemove:                         input.autoRemove,
		ArtifactServerPath:                 input.artifactServerPath,
		ArtifactServerAddr:                 input.artifactServerAddr,
		ArtifactServerPort:                 input.artifactServerPort,
		NoSkipCheckout:                     input.noSkipCheckout,
		RemoteName:                         input.remoteName,
		ReplaceGheActionWithGithubCom:      input.replaceGheActionWithGithubCom,
		ReplaceGheActionTokenWithGithubCom: input.replaceGheActionTokenWithGithubCom,
		Matrix:                             matrixes,
		ContainerNetworkMode:               docker_container.NetworkMode(input.networkName),
		ConcurrentJobs:                     input.concurrentJobs,
	}

	if input.useNewActionCache || len(input.localRepository) > 0 {
		if input.actionOfflineMode {
			config.ActionCache = &runner.GoGitActionCacheOfflineMode{
				Parent: runner.GoGitActionCache{
					Path: cacheDir,
				},
			}
		} else {
			config.ActionCache = &runner.GoGitActionCache{
				Path: cacheDir,
			}
		}
		if len(input.localRepository) > 0 {
			localRepositories := map[string]string{}
			for _, l := range input.localRepository {
				k, v, _ := strings.Cut(l, "=")
				localRepositories[k] = v
			}
			config.ActionCache = &runner.LocalRepositoryCache{
				Parent:            config.ActionCache,
				LocalRepositories: localRepositories,
				CacheDirCache:     map[string]string{},
			}
		}
	}

	selection := runner.Selection{
		EventName: eventName,
		JobID:     "",
		Workflow:  "",
	}

	previewInput := &runner.PreviewInput{
		Ctx:                ctx,
		Config:             config,
		Plan:               plan,
		WorkflowsPath:      workflowsPath,
		Selection:          selection,
		EnvFile:            input.Envfile(),
		SecretFile:         input.Secretfile(),
		VarFile:            input.Varfile(),
		EnvCLI:             envs,
		SecretCLI:          secrets,
		VarCLI:             vars,
		ActionCacheDir:     cacheDir,
		ActionOfflineMode:  input.actionOfflineMode,
		UseNewActionCache:  input.useNewActionCache,
		LocalRepositories:  input.localRepository,
	}

	return runner.NewPreviewPlan(previewInput)
}

func printPreview(pp *runner.PreviewPlan, asJSON bool) error {
	if asJSON {
		jsonStr, err := pp.JSON()
		if err != nil {
			return fmt.Errorf("failed to serialize preview: %w", err)
		}
		fmt.Println(jsonStr)
	} else {
		fmt.Print(pp.Table())
	}
	return nil
}

func printListFromPreview(pp *runner.PreviewPlan) {
	fmt.Print(pp.ListTable())
}

func printGraphFromPreview(pp *runner.PreviewPlan) {
	for _, line := range pp.GraphDrawing() {
		fmt.Println(line)
	}
}

func listEvents() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	planner, err := model.NewWorkflowPlanner(dir, false, false)
	if err != nil {
		log.Fatal(err)
	}

	events := planner.GetEvents()

	log.Info("Events")
	for _, e := range events {
		log.Infof("  %s", e)
	}
}
