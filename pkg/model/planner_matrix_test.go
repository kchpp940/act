package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeMatrixKey(t *testing.T) {
	tests := []struct {
		name     string
		matrix   map[string]interface{}
		expected string
	}{
		{
			name:     "empty matrix",
			matrix:   map[string]interface{}{},
			expected: "",
		},
		{
			name:     "nil matrix",
			matrix:   nil,
			expected: "",
		},
		{
			name:     "single key",
			matrix:   map[string]interface{}{"os": "ubuntu-latest"},
			expected: "os=ubuntu-latest",
		},
		{
			name:     "keys sorted canonically",
			matrix:   map[string]interface{}{"node": "14.x", "os": "ubuntu-latest"},
			expected: "node=14.x&os=ubuntu-latest",
		},
		{
			name:     "numeric values",
			matrix:   map[string]interface{}{"version": 14, "os": "ubuntu-latest"},
			expected: "os=ubuntu-latest&version=14",
		},
		{
			name:     "special characters escaped",
			matrix:   map[string]interface{}{"os": "ubuntu 20.04"},
			expected: "os=ubuntu+20.04",
		},
		{
			name:     "value with ampersand escaped",
			matrix:   map[string]interface{}{"ref": "a&b"},
			expected: "ref=a%26b",
		},
		{
			name:     "same values different insertion order produce same key",
			matrix:   map[string]interface{}{"z": "1", "a": "2"},
			expected: "a=2&z=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeMatrixKey(tt.matrix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComputeMatrixKey_Determinism(t *testing.T) {
	m1 := map[string]interface{}{"node": "14.x", "os": "ubuntu-latest", "arch": "amd64"}
	m2 := map[string]interface{}{"arch": "amd64", "os": "ubuntu-latest", "node": "14.x"}
	assert.Equal(t, ComputeMatrixKey(m1), ComputeMatrixKey(m2))
}

func TestFormatMatrix(t *testing.T) {
	tests := []struct {
		name     string
		matrix   map[string]interface{}
		expected string
	}{
		{
			name:     "empty matrix",
			matrix:   map[string]interface{}{},
			expected: "",
		},
		{
			name:     "nil matrix",
			matrix:   nil,
			expected: "",
		},
		{
			name:     "single key",
			matrix:   map[string]interface{}{"os": "ubuntu-latest"},
			expected: "os=ubuntu-latest",
		},
		{
			name:     "multiple keys sorted",
			matrix:   map[string]interface{}{"node": "14.x", "os": "ubuntu-latest"},
			expected: "node=14.x, os=ubuntu-latest",
		},
		{
			name:     "numeric values",
			matrix:   map[string]interface{}{"version": 14, "os": "ubuntu-latest"},
			expected: "os=ubuntu-latest, version=14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMatrix(tt.matrix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRun_MatchesMatrixFilter(t *testing.T) {
	run := &Run{
		Matrix: map[string]interface{}{
			"os":   "ubuntu-latest",
			"node": "14.x",
		},
	}

	tests := []struct {
		name     string
		filter   map[string]map[string]bool
		expected bool
	}{
		{
			name:     "empty filter matches everything",
			filter:   map[string]map[string]bool{},
			expected: true,
		},
		{
			name:     "nil filter matches everything",
			filter:   nil,
			expected: true,
		},
		{
			name: "single dimension match",
			filter: map[string]map[string]bool{
				"os": {"ubuntu-latest": true},
			},
			expected: true,
		},
		{
			name: "single dimension no match",
			filter: map[string]map[string]bool{
				"os": {"macos-latest": true},
			},
			expected: false,
		},
		{
			name: "multiple dimensions match",
			filter: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
			},
			expected: true,
		},
		{
			name: "multiple dimensions one no match",
			filter: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"16.x": true},
			},
			expected: false,
		},
		{
			name: "missing dimension key",
			filter: map[string]map[string]bool{
				"missing": {"value": true},
			},
			expected: false,
		},
		{
			name: "multiple allowed values for one key",
			filter: map[string]map[string]bool{
				"os": {"ubuntu-latest": true, "macos-latest": true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := run.MatchesMatrixFilter(tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRun_String(t *testing.T) {
	workflow := &Workflow{
		Name: "test-workflow",
		Jobs: map[string]*Job{
			"test-job": {
				Name: "Test Job",
			},
		},
	}

	tests := []struct {
		name     string
		run      *Run
		expected string
	}{
		{
			name: "no matrix",
			run: &Run{
				Workflow: workflow,
				JobID:    "test-job",
			},
			expected: "Test Job",
		},
		{
			name: "with matrix",
			run: &Run{
				Workflow: workflow,
				JobID:    "test-job",
				Matrix: map[string]interface{}{
					"os":   "ubuntu-latest",
					"node": "14.x",
				},
			},
			expected: "Test Job (node=14.x, os=ubuntu-latest)",
		},
		{
			name: "empty job name uses job id",
			run: &Run{
				Workflow: &Workflow{
					Jobs: map[string]*Job{
						"test-job": {},
					},
				},
				JobID: "test-job",
			},
			expected: "test-job",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.run.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRun_SimpleName(t *testing.T) {
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"test-job": {
				Name: "Test Job",
			},
		},
	}

	run := &Run{
		Workflow: workflow,
		JobID:    "test-job",
		Matrix: map[string]interface{}{
			"os": "ubuntu-latest",
		},
	}

	assert.Equal(t, "Test Job", run.SimpleName())
	assert.Contains(t, run.String(), "Test Job (")
}

func TestPlan_FilterRuns(t *testing.T) {
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"job1": {Name: "Job 1"},
			"job2": {Name: "Job 2"},
		},
	}

	plan := &Plan{
		Stages: []*Stage{
			{
				Runs: []*Run{
					{Workflow: workflow, JobID: "job1", Matrix: map[string]interface{}{"os": "ubuntu"}},
					{Workflow: workflow, JobID: "job2", Matrix: map[string]interface{}{"os": "macos"}},
				},
			},
		},
	}

	filtered := plan.FilterRuns(func(r *Run) bool {
		return r.Matrix["os"] == "ubuntu"
	})

	assert.Len(t, filtered.Stages, 1)
	assert.Len(t, filtered.Stages[0].Runs, 1)
	assert.Equal(t, "job1", filtered.Stages[0].Runs[0].JobID)
}

func TestPlan_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		plan     *Plan
		expected bool
	}{
		{
			name:     "nil stages",
			plan:     &Plan{},
			expected: true,
		},
		{
			name:     "empty stages",
			plan:     &Plan{Stages: []*Stage{}},
			expected: true,
		},
		{
			name: "stages with empty runs",
			plan: &Plan{
				Stages: []*Stage{
					{Runs: []*Run{}},
				},
			},
			expected: true,
		},
		{
			name: "stages with runs",
			plan: &Plan{
				Stages: []*Stage{
					{Runs: []*Run{{JobID: "test"}}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.plan.IsEmpty())
		})
	}
}

func TestMatrixSelector_Match(t *testing.T) {
	workflow := &Workflow{
		Jobs: map[string]*Job{
			"build": {Name: "Build"},
		},
	}

	runUbuntu := &Run{
		Workflow:  workflow,
		JobID:     "build",
		Matrix:    map[string]interface{}{"os": "ubuntu-latest", "node": "14.x"},
		MatrixKey: "node=14.x&os=ubuntu-latest",
	}
	runMacos := &Run{
		Workflow:  workflow,
		JobID:     "build",
		Matrix:    map[string]interface{}{"os": "macos-latest", "node": "16.x"},
		MatrixKey: "node=16.x&os=macos-latest",
	}
	runNoMatrix := &Run{
		Workflow: workflow,
		JobID:    "build",
	}

	tests := []struct {
		name     string
		selector *MatrixSelector
		run      *Run
		expected bool
	}{
		{
			name:     "nil selector matches everything",
			selector: nil,
			run:      runUbuntu,
			expected: true,
		},
		{
			name:     "empty selector matches everything",
			selector: NewMatrixSelector(nil, ""),
			run:      runUbuntu,
			expected: true,
		},
		{
			name:     "dimension filter matches",
			selector: NewMatrixSelector(map[string]map[string]bool{"os": {"ubuntu-latest": true}}, ""),
			run:      runUbuntu,
			expected: true,
		},
		{
			name:     "dimension filter no match",
			selector: NewMatrixSelector(map[string]map[string]bool{"os": {"ubuntu-latest": true}}, ""),
			run:      runMacos,
			expected: false,
		},
		{
			name:     "matrix key filter matches",
			selector: NewMatrixSelector(nil, "node=14.x&os=ubuntu-latest"),
			run:      runUbuntu,
			expected: true,
		},
		{
			name:     "matrix key filter no match",
			selector: NewMatrixSelector(nil, "node=14.x&os=ubuntu-latest"),
			run:      runMacos,
			expected: false,
		},
		{
			name:     "both dimensions and key must match",
			selector: NewMatrixSelector(map[string]map[string]bool{"os": {"ubuntu-latest": true}}, "node=14.x&os=ubuntu-latest"),
			run:      runUbuntu,
			expected: true,
		},
		{
			name:     "dimension matches but key does not",
			selector: NewMatrixSelector(map[string]map[string]bool{"os": {"ubuntu-latest": true}}, "node=16.x&os=ubuntu-latest"),
			run:      runUbuntu,
			expected: false,
		},
		{
			name:     "no matrix run with dimension filter",
			selector: NewMatrixSelector(map[string]map[string]bool{"os": {"ubuntu-latest": true}}, ""),
			run:      runNoMatrix,
			expected: false,
		},
		{
			name:     "no matrix run with empty selector",
			selector: NewMatrixSelector(nil, ""),
			run:      runNoMatrix,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.selector.Match(tt.run)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatrixSelector_IsEmpty(t *testing.T) {
	assert.True(t, (*MatrixSelector)(nil).IsEmpty())
	assert.True(t, NewMatrixSelector(nil, "").IsEmpty())
	assert.True(t, NewMatrixSelector(map[string]map[string]bool{}, "").IsEmpty())
	assert.False(t, NewMatrixSelector(map[string]map[string]bool{"os": {"ubuntu": true}}, "").IsEmpty())
	assert.False(t, NewMatrixSelector(nil, "os=ubuntu").IsEmpty())
}
