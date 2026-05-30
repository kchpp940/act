package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nektos/act/pkg/model"
)

func printList(plan *model.Plan) error {
	type lineInfoDef struct {
		jobID   string
		jobName string
		stage   string
		wfName  string
		wfFile  string
		events  string
		matrix  string
	}
	lineInfos := []lineInfoDef{}

	header := lineInfoDef{
		jobID:   "Job ID",
		jobName: "Job name",
		stage:   "Stage",
		wfName:  "Workflow name",
		wfFile:  "Workflow file",
		events:  "Events",
		matrix:  "Matrix",
	}

	jobs := map[string]bool{}
	duplicateJobIDs := false
	hasMatrix := false

	jobIDMaxWidth := len(header.jobID)
	jobNameMaxWidth := len(header.jobName)
	stageMaxWidth := len(header.stage)
	wfNameMaxWidth := len(header.wfName)
	wfFileMaxWidth := len(header.wfFile)
	eventsMaxWidth := len(header.events)
	matrixMaxWidth := len(header.matrix)

	for i, stage := range plan.Stages {
		for _, r := range stage.Runs {
			jobID := r.JobID
			matrixStr := ""
			if r.MatrixKey != "" {
				matrixStr = r.MatrixKey
				hasMatrix = true
			}
			line := lineInfoDef{
				jobID:   jobID,
				jobName: r.SimpleName(),
				stage:   strconv.Itoa(i),
				wfName:  r.Workflow.Name,
				wfFile:  r.Workflow.File,
				events:  strings.Join(r.Workflow.On(), `,`),
				matrix:  matrixStr,
			}
			if _, ok := jobs[jobID]; ok {
				duplicateJobIDs = true
			} else {
				jobs[jobID] = true
			}
			lineInfos = append(lineInfos, line)
			if jobIDMaxWidth < len(line.jobID) {
				jobIDMaxWidth = len(line.jobID)
			}
			if jobNameMaxWidth < len(line.jobName) {
				jobNameMaxWidth = len(line.jobName)
			}
			if stageMaxWidth < len(line.stage) {
				stageMaxWidth = len(line.stage)
			}
			if wfNameMaxWidth < len(line.wfName) {
				wfNameMaxWidth = len(line.wfName)
			}
			if wfFileMaxWidth < len(line.wfFile) {
				wfFileMaxWidth = len(line.wfFile)
			}
			if eventsMaxWidth < len(line.events) {
				eventsMaxWidth = len(line.events)
			}
			if matrixMaxWidth < len(line.matrix) {
				matrixMaxWidth = len(line.matrix)
			}
		}
	}

	jobIDMaxWidth += 2
	jobNameMaxWidth += 2
	stageMaxWidth += 2
	wfNameMaxWidth += 2
	wfFileMaxWidth += 2
	eventsMaxWidth += 2
	matrixMaxWidth += 2

	if hasMatrix {
		fmt.Printf("%*s%*s%*s%*s%*s%*s%*s\n",
			-stageMaxWidth, header.stage,
			-jobIDMaxWidth, header.jobID,
			-jobNameMaxWidth, header.jobName,
			-matrixMaxWidth, header.matrix,
			-wfNameMaxWidth, header.wfName,
			-wfFileMaxWidth, header.wfFile,
			-eventsMaxWidth, header.events,
		)
		for _, line := range lineInfos {
			fmt.Printf("%*s%*s%*s%*s%*s%*s%*s\n",
				-stageMaxWidth, line.stage,
				-jobIDMaxWidth, line.jobID,
				-jobNameMaxWidth, line.jobName,
				-matrixMaxWidth, line.matrix,
				-wfNameMaxWidth, line.wfName,
				-wfFileMaxWidth, line.wfFile,
				-eventsMaxWidth, line.events,
			)
		}
	} else {
		fmt.Printf("%*s%*s%*s%*s%*s%*s\n",
			-stageMaxWidth, header.stage,
			-jobIDMaxWidth, header.jobID,
			-jobNameMaxWidth, header.jobName,
			-wfNameMaxWidth, header.wfName,
			-wfFileMaxWidth, header.wfFile,
			-eventsMaxWidth, header.events,
		)
		for _, line := range lineInfos {
			fmt.Printf("%*s%*s%*s%*s%*s%*s\n",
				-stageMaxWidth, line.stage,
				-jobIDMaxWidth, line.jobID,
				-jobNameMaxWidth, line.jobName,
				-wfNameMaxWidth, line.wfName,
				-wfFileMaxWidth, line.wfFile,
				-eventsMaxWidth, line.events,
			)
		}
	}
	if duplicateJobIDs {
		fmt.Print("\nDetected multiple jobs with the same job name, use `-W` to specify the path to the specific workflow.\n")
	}
	if hasMatrix {
		fmt.Print("\nTo run a specific matrix combination, use: act --matrix \"os=ubuntu-latest,node=14.x\"\nTo filter by matrix key, use: act --matrix-key \"node=14.x&os=ubuntu-latest\"\n")
	}
	return nil
}
