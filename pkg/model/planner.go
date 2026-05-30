package model

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
)

// WorkflowPlanner contains methods for creating plans
type WorkflowPlanner interface {
	PlanEvent(eventName string) (*Plan, error)
	PlanJob(jobName string) (*Plan, error)
	PlanAll() (*Plan, error)
	GetEvents() []string
	ExpandMatrix(plan *Plan) (*Plan, error)
}

type MatrixSelector struct {
	dimensions map[string]map[string]bool
	matrixKey  string
}

func NewMatrixSelector(dimensions map[string]map[string]bool, matrixKey string) *MatrixSelector {
	return &MatrixSelector{dimensions: dimensions, matrixKey: matrixKey}
}

func (s *MatrixSelector) Match(run *Run) bool {
	if s == nil {
		return true
	}
	if len(s.dimensions) > 0 {
		if run.Matrix == nil || len(run.Matrix) == 0 {
			return false
		}
		if !run.MatchesMatrixFilter(s.dimensions) {
			return false
		}
	}
	if s.matrixKey != "" && run.MatrixKey != s.matrixKey {
		return false
	}
	return true
}

func (s *MatrixSelector) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.dimensions) == 0 && s.matrixKey == ""
}

func (s *MatrixSelector) Reason() string {
	if s.IsEmpty() {
		return ""
	}
	var parts []string
	if len(s.dimensions) > 0 {
		keys := make([]string, 0, len(s.dimensions))
		for k := range s.dimensions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		dimParts := make([]string, 0, len(keys))
		for _, k := range keys {
			vals := make([]string, 0, len(s.dimensions[k]))
			for v := range s.dimensions[k] {
				vals = append(vals, v)
			}
			sort.Strings(vals)
			dimParts = append(dimParts, fmt.Sprintf("%s=%s", k, strings.Join(vals, "|")))
		}
		parts = append(parts, "matrix="+strings.Join(dimParts, ", "))
	}
	if s.matrixKey != "" {
		parts = append(parts, "matrix-key="+s.matrixKey)
	}
	return strings.Join(parts, ", ")
}

// Plan contains a list of stages to run in series
type Plan struct {
	Stages []*Stage
}

// Stage contains a list of runs to execute in parallel
type Stage struct {
	Runs []*Run
}

// Run represents a job from a workflow that needs to be run
type Run struct {
	Workflow  *Workflow
	JobID     string
	Matrix    map[string]interface{}
	MatrixKey string
}

func (r *Run) String() string {
	jobName := r.Job().Name
	if jobName == "" {
		jobName = r.JobID
	}
	if r.Matrix != nil && len(r.Matrix) > 0 {
		return fmt.Sprintf("%s (%s)", jobName, FormatMatrix(r.Matrix))
	}
	return jobName
}

// DisplayName returns a human-readable name for the run including matrix info
func (r *Run) DisplayName() string {
	return r.String()
}

// SimpleName returns the job name without matrix info
func (r *Run) SimpleName() string {
	jobName := r.Job().Name
	if jobName == "" {
		jobName = r.JobID
	}
	return jobName
}

// FormatMatrix formats a matrix map as a human-readable string
func FormatMatrix(matrix map[string]interface{}) string {
	if len(matrix) == 0 {
		return ""
	}
	keys := make([]string, 0, len(matrix))
	for k := range matrix {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, matrix[k]))
	}
	return strings.Join(parts, ", ")
}

func ComputeMatrixKey(matrix map[string]interface{}) string {
	if len(matrix) == 0 {
		return ""
	}
	keys := make([]string, 0, len(matrix))
	for k := range matrix {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := url.QueryEscape(fmt.Sprintf("%v", matrix[k]))
		parts = append(parts, url.QueryEscape(k)+"="+v)
	}
	return strings.Join(parts, "&")
}

// MatchesMatrixFilter checks if the run's matrix matches the given filter.
// The filter is a map of dimension key to allowed values (as strings).
// Multiple dimensions are ANDed together.
func (r *Run) MatchesMatrixFilter(filter map[string]map[string]bool) bool {
	if r.Matrix == nil || len(filter) == 0 {
		return true
	}
	for key, allowedVals := range filter {
		val, ok := r.Matrix[key]
		if !ok {
			return false
		}
		valStr := fmt.Sprintf("%v", val)
		if _, ok := allowedVals[valStr]; !ok {
			return false
		}
	}
	return true
}

// Job returns the job for this Run
func (r *Run) Job() *Job {
	return r.Workflow.GetJob(r.JobID)
}

type WorkflowFiles struct {
	workflowDirEntry os.DirEntry
	dirPath          string
}

// NewWorkflowPlanner will load a specific workflow, all workflows from a directory or all workflows from a directory and its subdirectories
func NewWorkflowPlanner(path string, noWorkflowRecurse, strict bool) (WorkflowPlanner, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var workflows []WorkflowFiles

	if fi.IsDir() {
		log.Debugf("Loading workflows from '%s'", path)
		if noWorkflowRecurse {
			files, err := os.ReadDir(path)
			if err != nil {
				return nil, err
			}

			for _, v := range files {
				workflows = append(workflows, WorkflowFiles{
					dirPath:          path,
					workflowDirEntry: v,
				})
			}
		} else {
			log.Debug("Loading workflows recursively")
			if err := filepath.Walk(path,
				func(p string, f os.FileInfo, err error) error {
					if err != nil {
						return err
					}

					if !f.IsDir() {
						log.Debugf("Found workflow '%s' in '%s'", f.Name(), p)
						workflows = append(workflows, WorkflowFiles{
							dirPath:          filepath.Dir(p),
							workflowDirEntry: fs.FileInfoToDirEntry(f),
						})
					}

					return nil
				}); err != nil {
				return nil, err
			}
		}
	} else {
		log.Debugf("Loading workflow '%s'", path)
		dirname := filepath.Dir(path)

		workflows = append(workflows, WorkflowFiles{
			dirPath:          dirname,
			workflowDirEntry: fs.FileInfoToDirEntry(fi),
		})
	}

	wp := new(workflowPlanner)
	for _, wf := range workflows {
		ext := filepath.Ext(wf.workflowDirEntry.Name())
		if ext == ".yml" || ext == ".yaml" {
			f, err := os.Open(filepath.Join(wf.dirPath, wf.workflowDirEntry.Name()))
			if err != nil {
				return nil, err
			}

			log.Debugf("Reading workflow '%s'", f.Name())
			workflow, err := ReadWorkflow(f, strict)
			if err != nil {
				_ = f.Close()
				if err == io.EOF {
					return nil, fmt.Errorf("unable to read workflow '%s': file is empty: %w", wf.workflowDirEntry.Name(), err)
				}
				return nil, fmt.Errorf("workflow is not valid. '%s': %w", wf.workflowDirEntry.Name(), err)
			}
			_, err = f.Seek(0, 0)
			if err != nil {
				_ = f.Close()
				return nil, fmt.Errorf("error occurring when resetting io pointer in '%s': %w", wf.workflowDirEntry.Name(), err)
			}

			workflow.File = wf.workflowDirEntry.Name()
			if workflow.Name == "" {
				workflow.Name = wf.workflowDirEntry.Name()
			}

			err = validateJobName(workflow)
			if err != nil {
				_ = f.Close()
				return nil, err
			}

			wp.workflows = append(wp.workflows, workflow)
			_ = f.Close()
		}
	}

	return wp, nil
}

func NewSingleWorkflowPlanner(name string, f io.Reader) (WorkflowPlanner, error) {
	wp := new(workflowPlanner)

	log.Debugf("Reading workflow %s", name)
	workflow, err := ReadWorkflow(f, false)
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("unable to read workflow '%s': file is empty: %w", name, err)
		}
		return nil, fmt.Errorf("workflow is not valid. '%s': %w", name, err)
	}
	workflow.File = name
	if workflow.Name == "" {
		workflow.Name = name
	}

	err = validateJobName(workflow)
	if err != nil {
		return nil, err
	}

	wp.workflows = append(wp.workflows, workflow)

	return wp, nil
}

func validateJobName(workflow *Workflow) error {
	jobNameRegex := regexp.MustCompile(`^([[:alpha:]_][[:alnum:]_\-]*)$`)
	for k := range workflow.Jobs {
		if ok := jobNameRegex.MatchString(k); !ok {
			return fmt.Errorf("workflow is not valid. '%s': Job name '%s' is invalid. Names must start with a letter or '_' and contain only alphanumeric characters, '-', or '_'", workflow.Name, k)
		}
	}
	return nil
}

type workflowPlanner struct {
	workflows []*Workflow
}

// PlanEvent builds a new list of runs to execute in parallel for an event name
func (wp *workflowPlanner) PlanEvent(eventName string) (*Plan, error) {
	plan := new(Plan)
	if len(wp.workflows) == 0 {
		log.Debug("no workflows found by planner")
		return plan, nil
	}
	var lastErr error

	for _, w := range wp.workflows {
		events := w.On()
		if len(events) == 0 {
			log.Debugf("no events found for workflow: %s", w.File)
			continue
		}

		for _, e := range events {
			if e == eventName {
				stages, err := createStages(w, w.GetJobIDs()...)
				if err != nil {
					log.Warn(err)
					lastErr = err
				} else {
					plan.mergeStages(stages)
				}
			}
		}
	}
	return plan, lastErr
}

// PlanJob builds a new run to execute in parallel for a job name
func (wp *workflowPlanner) PlanJob(jobName string) (*Plan, error) {
	plan := new(Plan)
	if len(wp.workflows) == 0 {
		log.Debugf("no jobs found for workflow: %s", jobName)
	}
	var lastErr error

	for _, w := range wp.workflows {
		stages, err := createStages(w, jobName)
		if err != nil {
			log.Warn(err)
			lastErr = err
		} else {
			plan.mergeStages(stages)
		}
	}
	return plan, lastErr
}

// PlanAll builds a new run to execute in parallel all
func (wp *workflowPlanner) PlanAll() (*Plan, error) {
	plan := new(Plan)
	if len(wp.workflows) == 0 {
		log.Debug("no workflows found by planner")
		return plan, nil
	}
	var lastErr error

	for _, w := range wp.workflows {
		stages, err := createStages(w, w.GetJobIDs()...)
		if err != nil {
			log.Warn(err)
			lastErr = err
		} else {
			plan.mergeStages(stages)
		}
	}

	return plan, lastErr
}

// GetEvents gets all the events in the workflows file
func (wp *workflowPlanner) GetEvents() []string {
	events := make([]string, 0)
	for _, w := range wp.workflows {
		found := false
		for _, e := range events {
			for _, we := range w.On() {
				if e == we {
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			events = append(events, w.On()...)
		}
	}

	// sort the list based on depth of dependencies
	sort.Slice(events, func(i, j int) bool {
		return events[i] < events[j]
	})

	return events
}

// ExpandMatrix expands matrix jobs in the plan into individual runs with specific matrix combinations.
// If filter is provided, only matrix combinations matching the filter are included.
func (wp *workflowPlanner) ExpandMatrix(plan *Plan) (*Plan, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}

	newPlan := &Plan{}

	for _, stage := range plan.Stages {
		newStage := &Stage{}

		for _, run := range stage.Runs {
			job := run.Job()
			if job == nil {
				continue
			}

			matrixes, err := job.GetMatrixes()
			if err != nil {
				return nil, fmt.Errorf("failed to get matrixes for job %s: %w", run.JobID, err)
			}

			for _, matrix := range matrixes {
				newRun := &Run{
					Workflow:  run.Workflow,
					JobID:     run.JobID,
					Matrix:    matrix,
					MatrixKey: ComputeMatrixKey(matrix),
				}

				newStage.Runs = append(newStage.Runs, newRun)
			}
		}

		if len(newStage.Runs) > 0 {
			newPlan.Stages = append(newPlan.Stages, newStage)
		}
	}

	log.Debugf("Expanded matrix plan: original stages=%d, new stages=%d", len(plan.Stages), len(newPlan.Stages))
	for i, stage := range newPlan.Stages {
		for _, run := range stage.Runs {
			log.Debugf("  Stage %d: %s", i, run.String())
		}
	}

	return newPlan, nil
}

// MaxRunNameLen determines the max name length of all jobs
func (p *Plan) MaxRunNameLen() int {
	maxRunNameLen := 0
	for _, stage := range p.Stages {
		for _, run := range stage.Runs {
			runNameLen := len(run.String())
			if runNameLen > maxRunNameLen {
				maxRunNameLen = runNameLen
			}
		}
	}
	return maxRunNameLen
}

// FilterRuns filters runs in the plan based on the given filter function.
// Only runs for which the filter returns true are kept.
func (p *Plan) FilterRuns(filter func(*Run) bool) *Plan {
	newPlan := &Plan{}
	for _, stage := range p.Stages {
		newStage := &Stage{}
		for _, run := range stage.Runs {
			if filter(run) {
				newStage.Runs = append(newStage.Runs, run)
			}
		}
		if len(newStage.Runs) > 0 {
			newPlan.Stages = append(newPlan.Stages, newStage)
		}
	}
	return newPlan
}

// IsEmpty returns true if the plan has no runs
func (p *Plan) IsEmpty() bool {
	for _, stage := range p.Stages {
		if len(stage.Runs) > 0 {
			return false
		}
	}
	return true
}

// GetJobIDs will get all the job names in the stage
func (s *Stage) GetJobIDs() []string {
	names := make([]string, 0)
	for _, r := range s.Runs {
		names = append(names, r.JobID)
	}
	return names
}

// Merge stages with existing stages in plan
func (p *Plan) mergeStages(stages []*Stage) {
	newStages := make([]*Stage, int(math.Max(float64(len(p.Stages)), float64(len(stages)))))
	for i := 0; i < len(newStages); i++ {
		newStages[i] = new(Stage)
		if i >= len(p.Stages) {
			newStages[i].Runs = append(newStages[i].Runs, stages[i].Runs...)
		} else if i >= len(stages) {
			newStages[i].Runs = append(newStages[i].Runs, p.Stages[i].Runs...)
		} else {
			newStages[i].Runs = append(newStages[i].Runs, p.Stages[i].Runs...)
			newStages[i].Runs = append(newStages[i].Runs, stages[i].Runs...)
		}
	}
	p.Stages = newStages
}

func createStages(w *Workflow, jobIDs ...string) ([]*Stage, error) {
	// first, build a list of all the necessary jobs to run, and their dependencies
	jobDependencies := make(map[string][]string)
	for len(jobIDs) > 0 {
		newJobIDs := make([]string, 0)
		for _, jID := range jobIDs {
			// make sure we haven't visited this job yet
			if _, ok := jobDependencies[jID]; !ok {
				if job := w.GetJob(jID); job != nil {
					jobDependencies[jID] = job.Needs()
					newJobIDs = append(newJobIDs, job.Needs()...)
				}
			}
		}
		jobIDs = newJobIDs
	}

	// next, build an execution graph
	stages := make([]*Stage, 0)
	for len(jobDependencies) > 0 {
		stage := new(Stage)
		for jID, jDeps := range jobDependencies {
			// make sure all deps are in the graph already
			if listInStages(jDeps, stages...) {
				stage.Runs = append(stage.Runs, &Run{
					Workflow: w,
					JobID:    jID,
				})
				delete(jobDependencies, jID)
			}
		}
		if len(stage.Runs) == 0 {
			return nil, fmt.Errorf("unable to build dependency graph for %s (%s)", w.Name, w.File)
		}
		stages = append(stages, stage)
	}

	return stages, nil
}

// return true iff all strings in srcList exist in at least one of the stages
func listInStages(srcList []string, stages ...*Stage) bool {
	for _, src := range srcList {
		found := false
		for _, stage := range stages {
			for _, search := range stage.GetJobIDs() {
				if src == search {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}
