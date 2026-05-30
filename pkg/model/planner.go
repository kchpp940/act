package model

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	log "github.com/sirupsen/logrus"
)

type WorkflowPlanner interface {
	GetEvents() []string
	NewPipeline() *PlannerPipeline
}

type SelectionPolicy struct {
	Strategy   SelectionStrategy
	EventName  string
	JobID      string
	AutoDetect bool
	SelectAll  bool
	Matrix     map[string]map[string]bool
}

type SelectionStrategy string

const (
	SelectionStrategyByEvent    SelectionStrategy = "by_event"
	SelectionStrategyByJob      SelectionStrategy = "by_job"
	SelectionStrategyAllJobs    SelectionStrategy = "all_jobs"
	SelectionStrategyAutoDetect SelectionStrategy = "auto_detect"
)

func (s SelectionStrategy) String() string {
	return string(s)
}

func SelectByEvent(eventName string) *SelectionPolicy {
	return &SelectionPolicy{
		Strategy:  SelectionStrategyByEvent,
		EventName: eventName,
	}
}

func SelectByJob(jobID string) *SelectionPolicy {
	return &SelectionPolicy{
		Strategy: SelectionStrategyByJob,
		JobID:    jobID,
	}
}

func SelectAllJobs() *SelectionPolicy {
	return &SelectionPolicy{
		Strategy:  SelectionStrategyAllJobs,
		SelectAll: true,
	}
}

func SelectWithAutoDetect() *SelectionPolicy {
	return &SelectionPolicy{
		Strategy:   SelectionStrategyAutoDetect,
		AutoDetect: true,
	}
}

func (s *SelectionPolicy) WithMatrix(matrix map[string]map[string]bool) *SelectionPolicy {
	s.Matrix = matrix
	return s
}

func (s *SelectionPolicy) WithAutoDetect(auto bool) *SelectionPolicy {
	s.AutoDetect = auto
	return s
}

type DiscoveryInput struct {
	WorkflowsPath     string
	NoWorkflowRecurse bool
	Strict            bool
}

type DiscoverySummary struct {
	WorkflowCount int
	WorkflowNames []string
}

type DiscoveryResult struct {
	Workflows []*Workflow
	Summary   DiscoverySummary
}

type InferInput struct {
	Workflows  []*Workflow
	EventName  string
	AutoDetect bool
}

type InferSummary struct {
	RequestedEvent  string
	AvailableEvents []string
	SelectedEvent   string
	WasAutoDetected bool
}

type InferResult struct {
	EventName string
	Summary   InferSummary
}

type SelectInput struct {
	Workflows []*Workflow
	EventName string
	JobID     string
	SelectAll bool
}

type SelectSummary struct {
	SelectedJobCount int
	SelectedJobs     []string
	EventFilter      string
	JobIDFilter      string
	SelectAllEnabled bool
}

type SelectResult struct {
	Jobs    []*SelectedJob
	Summary SelectSummary
}

type ExpandInput struct {
	SelectedJobs []*SelectedJob
	MatrixFilter map[string]map[string]bool
}

type ExpandSummary struct {
	JobCount             int
	TotalMatrixCombos    int
	FilteredMatrixCombos int
}

type ExpandResult struct {
	Runs    []*Run
	Summary ExpandSummary
}

type SortInput struct {
	Runs []*Run
}

type SortSummary struct {
	StageCount int
	RunCount   int
	SortErrors []string
}

type SortResult struct {
	Plan    *Plan
	Err     error
	Summary SortSummary
}

type PlannerResult struct {
	Plan             *Plan
	EventName        string
	Workflows        []*Workflow
	Error            error
	Policy           *SelectionPolicy
	DiscoverySummary DiscoverySummary
	InferSummary     InferSummary
	SelectSummary    SelectSummary
	ExpandSummary    ExpandSummary
	SortSummary      SortSummary
}

type SelectedJob struct {
	Workflow *Workflow
	JobID    string
}

type Plan struct {
	Stages []*Stage
}

type Stage struct {
	Runs []*Run
}

type Run struct {
	Workflow *Workflow
	JobID    string
	Matrix   map[string]interface{}
}

type WorkflowFiles struct {
	workflowDirEntry os.DirEntry
	dirPath          string
}

type PlannerPipeline struct {
	workflows []*Workflow
}

func NewPlannerPipeline(workflows []*Workflow) *PlannerPipeline {
	return &PlannerPipeline{
		workflows: workflows,
	}
}

func (p *PlannerPipeline) Execute(policy *SelectionPolicy) *PlannerResult {
	return p.Run(policy)
}

func (p *PlannerPipeline) Run(policy *SelectionPolicy) *PlannerResult {
	workflowNames := make([]string, 0, len(p.workflows))
	for _, w := range p.workflows {
		workflowNames = append(workflowNames, w.Name)
	}
	discoverResult := &DiscoveryResult{
		Workflows: p.workflows,
		Summary: DiscoverySummary{
			WorkflowCount: len(p.workflows),
			WorkflowNames: workflowNames,
		},
	}

	inferResult := InferEvent(InferInput{
		Workflows:  discoverResult.Workflows,
		EventName:  policy.EventName,
		AutoDetect: policy.AutoDetect,
	})

	selectResult := SelectJobs(SelectInput{
		Workflows: discoverResult.Workflows,
		EventName: inferResult.EventName,
		JobID:     policy.JobID,
		SelectAll: policy.SelectAll,
	})

	expandResult := ExpandMatrix(ExpandInput{
		SelectedJobs: selectResult.Jobs,
		MatrixFilter: policy.Matrix,
	})

	sortResult := SortRuns(SortInput{
		Runs: expandResult.Runs,
	})

	return &PlannerResult{
		Plan:             sortResult.Plan,
		EventName:        inferResult.EventName,
		Workflows:        discoverResult.Workflows,
		Error:            sortResult.Err,
		Policy:           policy,
		DiscoverySummary: discoverResult.Summary,
		InferSummary:     inferResult.Summary,
		SelectSummary:    selectResult.Summary,
		ExpandSummary:    expandResult.Summary,
		SortSummary:      sortResult.Summary,
	}
}

func DiscoverWorkflows(input DiscoveryInput) (*DiscoveryResult, error) {
	path, err := filepath.Abs(input.WorkflowsPath)
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
		if input.NoWorkflowRecurse {
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

	result := &DiscoveryResult{}
	for _, wf := range workflows {
		ext := filepath.Ext(wf.workflowDirEntry.Name())
		if ext == ".yml" || ext == ".yaml" {
			f, err := os.Open(filepath.Join(wf.dirPath, wf.workflowDirEntry.Name()))
			if err != nil {
				return nil, err
			}

			log.Debugf("Reading workflow '%s'", f.Name())
			workflow, err := ReadWorkflow(f, input.Strict)
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

			result.Workflows = append(result.Workflows, workflow)
			_ = f.Close()
		}
	}

	result.Summary = DiscoverySummary{
		WorkflowCount: len(result.Workflows),
	}
	for _, w := range result.Workflows {
		result.Summary.WorkflowNames = append(result.Summary.WorkflowNames, w.Name)
	}

	return result, nil
}

func InferEvent(input InferInput) *InferResult {
	availableEvents := getEventsFromWorkflows(input.Workflows)

	if input.EventName != "" {
		return &InferResult{
			EventName: input.EventName,
			Summary: InferSummary{
				RequestedEvent:  input.EventName,
				AvailableEvents: availableEvents,
				SelectedEvent:   input.EventName,
				WasAutoDetected: false,
			},
		}
	}

	if len(availableEvents) == 0 {
		return &InferResult{
			EventName: "",
			Summary: InferSummary{
				RequestedEvent:  "",
				AvailableEvents: availableEvents,
				SelectedEvent:   "",
				WasAutoDetected: false,
			},
		}
	}

	if input.AutoDetect || len(availableEvents) == 1 {
		log.Debugf("Using detected workflow event: %s", availableEvents[0])
		return &InferResult{
			EventName: availableEvents[0],
			Summary: InferSummary{
				RequestedEvent:  "",
				AvailableEvents: availableEvents,
				SelectedEvent:   availableEvents[0],
				WasAutoDetected: true,
			},
		}
	}

	log.Debugf("Using default workflow event: push")
	return &InferResult{
		EventName: "push",
		Summary: InferSummary{
			RequestedEvent:  "",
			AvailableEvents: availableEvents,
			SelectedEvent:   "push",
			WasAutoDetected: false,
		},
	}
}

func SelectJobs(input SelectInput) *SelectResult {
	result := &SelectResult{
		Summary: SelectSummary{
			EventFilter:      input.EventName,
			JobIDFilter:      input.JobID,
			SelectAllEnabled: input.SelectAll,
		},
	}

	if len(input.Workflows) == 0 {
		log.Debug("no workflows found by planner")
		return result
	}

	for _, w := range input.Workflows {
		if input.JobID != "" {
			if job := w.GetJob(input.JobID); job != nil {
				result.Jobs = append(result.Jobs, &SelectedJob{
					Workflow: w,
					JobID:    input.JobID,
				})
			}
		} else if input.SelectAll || input.EventName == "" {
			for _, jobID := range w.GetJobIDs() {
				result.Jobs = append(result.Jobs, &SelectedJob{
					Workflow: w,
					JobID:    jobID,
				})
			}
		} else if input.EventName != "" {
			events := w.On()
			foundEvent := false
			for _, e := range events {
				if e == input.EventName {
					foundEvent = true
					for _, jobID := range w.GetJobIDs() {
						result.Jobs = append(result.Jobs, &SelectedJob{
							Workflow: w,
							JobID:    jobID,
						})
					}
				}
			}
			if !foundEvent {
				log.Debugf("no events found for workflow: %s", w.File)
			}
		}
	}

	result.Summary.SelectedJobCount = len(result.Jobs)
	for _, sj := range result.Jobs {
		result.Summary.SelectedJobs = append(result.Summary.SelectedJobs, sj.JobID)
	}

	return result
}

func ExpandMatrix(input ExpandInput) *ExpandResult {
	result := &ExpandResult{
		Summary: ExpandSummary{
			JobCount: len(input.SelectedJobs),
		},
	}

	for _, sj := range input.SelectedJobs {
		job := sj.Workflow.GetJob(sj.JobID)
		if job == nil {
			continue
		}

		matrixes, err := job.GetMatrixes()
		if err != nil {
			log.Warn(err)
			matrixes = []map[string]interface{}{{}}
		}
		result.Summary.TotalMatrixCombos += len(matrixes)

		filteredMatrixes := matrixes
		if input.MatrixFilter != nil && len(input.MatrixFilter) > 0 {
			filteredMatrixes = SelectMatrixes(matrixes, input.MatrixFilter)
		}
		result.Summary.FilteredMatrixCombos += len(filteredMatrixes)

		for _, matrix := range filteredMatrixes {
			run := &Run{
				Workflow: sj.Workflow,
				JobID:    sj.JobID,
				Matrix:   matrix,
			}
			result.Runs = append(result.Runs, run)
		}
	}

	return result
}

func SortRuns(input SortInput) *SortResult {
	plan := new(Plan)
	var lastErr error
	summary := SortSummary{
		RunCount: len(input.Runs),
	}

	workflowJobs := make(map[*Workflow][]*Run)
	for _, r := range input.Runs {
		workflowJobs[r.Workflow] = append(workflowJobs[r.Workflow], r)
	}

	for w, runs := range workflowJobs {
		jobIDs := make([]string, 0, len(runs))
		runMap := make(map[string]*Run)
		for _, r := range runs {
			jobIDs = append(jobIDs, r.JobID)
			runMap[r.JobID] = r
		}

		stages, err := createStages(w, jobIDs...)
		if err != nil {
			log.Warn(err)
			lastErr = err
			summary.SortErrors = append(summary.SortErrors, err.Error())
			continue
		}

		for _, stage := range stages {
			for i, run := range stage.Runs {
				if r, ok := runMap[run.JobID]; ok {
					stage.Runs[i] = r
				}
			}
		}

		plan.mergeStages(stages)
	}

	summary.StageCount = len(plan.Stages)

	return &SortResult{
		Plan:    plan,
		Err:     lastErr,
		Summary: summary,
	}
}

func SelectMatrixes(originalMatrixes []map[string]interface{}, targetMatrixValues map[string]map[string]bool) []map[string]interface{} {
	matrixes := make([]map[string]interface{}, 0)
	for _, original := range originalMatrixes {
		flag := true
		for key, val := range original {
			if allowedVals, ok := targetMatrixValues[key]; ok {
				valToString := fmt.Sprintf("%v", val)
				if _, ok := allowedVals[valToString]; !ok {
					flag = false
				}
			}
		}
		if flag {
			matrixes = append(matrixes, original)
		}
	}
	return matrixes
}

func getEventsFromWorkflows(workflows []*Workflow) []string {
	events := make([]string, 0)
	for _, w := range workflows {
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

	sort.Slice(events, func(i, j int) bool {
		return events[i] < events[j]
	})

	return events
}

func (r *Run) String() string {
	jobName := r.Job().Name
	if jobName == "" {
		jobName = r.JobID
	}
	return jobName
}

func (r *Run) Job() *Job {
	return r.Workflow.GetJob(r.JobID)
}

func NewWorkflowPlanner(path string, noWorkflowRecurse, strict bool) (WorkflowPlanner, error) {
	result, err := DiscoverWorkflows(DiscoveryInput{
		WorkflowsPath:     path,
		NoWorkflowRecurse: noWorkflowRecurse,
		Strict:            strict,
	})
	if err != nil {
		return nil, err
	}

	wp := &workflowPlanner{
		workflows: result.Workflows,
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

func (wp *workflowPlanner) NewPipeline() *PlannerPipeline {
	return NewPlannerPipeline(wp.workflows)
}

func (wp *workflowPlanner) GetEvents() []string {
	return getEventsFromWorkflows(wp.workflows)
}

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

func (s *Stage) GetJobIDs() []string {
	names := make([]string, 0)
	for _, r := range s.Runs {
		names = append(names, r.JobID)
	}
	return names
}

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
	jobDependencies := make(map[string][]string)
	for len(jobIDs) > 0 {
		newJobIDs := make([]string, 0)
		for _, jID := range jobIDs {
			if _, ok := jobDependencies[jID]; !ok {
				if job := w.GetJob(jID); job != nil {
					jobDependencies[jID] = job.Needs()
					newJobIDs = append(newJobIDs, job.Needs()...)
				}
			}
		}
		jobIDs = newJobIDs
	}

	stages := make([]*Stage, 0)
	for len(jobDependencies) > 0 {
		stage := new(Stage)
		for jID, jDeps := range jobDependencies {
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
