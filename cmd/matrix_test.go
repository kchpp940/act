package cmd

import (
	"testing"

	"github.com/nektos/act/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestParseMatrix(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected map[string]map[string]bool
	}{
		{
			name:     "empty input",
			input:    []string{},
			expected: map[string]map[string]bool{},
		},
		{
			name:  "single key:value",
			input: []string{"os:ubuntu-latest"},
			expected: map[string]map[string]bool{
				"os": {"ubuntu-latest": true},
			},
		},
		{
			name:  "single key=value",
			input: []string{"os=ubuntu-latest"},
			expected: map[string]map[string]bool{
				"os": {"ubuntu-latest": true},
			},
		},
		{
			name:  "multiple entries",
			input: []string{"os=ubuntu-latest", "node=14.x"},
			expected: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
			},
		},
		{
			name:  "comma-separated entries",
			input: []string{"os=ubuntu-latest,node=14.x"},
			expected: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
			},
		},
		{
			name:  "mixed separators",
			input: []string{"os:ubuntu-latest", "node=14.x"},
			expected: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
			},
		},
		{
			name:  "with spaces",
			input: []string{" os = ubuntu-latest ", " node : 14.x "},
			expected: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
			},
		},
		{
			name:  "multiple values for same key",
			input: []string{"os=ubuntu-latest", "os=macos-latest"},
			expected: map[string]map[string]bool{
				"os": {"ubuntu-latest": true, "macos-latest": true},
			},
		},
		{
			name:  "complex combination",
			input: []string{"os=ubuntu-latest,node=14.x", "php=8.0"},
			expected: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
				"php":  {"8.0": true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMatrix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatrixSelectorReason(t *testing.T) {
	tests := []struct {
		name      string
		dimensions map[string]map[string]bool
		matrixKey string
		expected  string
	}{
		{
			name:      "empty filter",
			dimensions: map[string]map[string]bool{},
			matrixKey: "",
			expected:  "",
		},
		{
			name: "single dimension",
			dimensions: map[string]map[string]bool{
				"os": {"ubuntu-latest": true},
			},
			matrixKey: "",
			expected:  "matrix=os=ubuntu-latest",
		},
		{
			name: "multiple dimensions sorted",
			dimensions: map[string]map[string]bool{
				"os":   {"ubuntu-latest": true},
				"node": {"14.x": true},
			},
			matrixKey: "",
			expected:  "matrix=node=14.x, os=ubuntu-latest",
		},
		{
			name: "multiple values for key",
			dimensions: map[string]map[string]bool{
				"os": {"ubuntu-latest": true, "macos-latest": true},
			},
			matrixKey: "",
			expected:  "matrix=os=macos-latest|ubuntu-latest",
		},
		{
			name:      "matrix key only",
			dimensions: nil,
			matrixKey: "node=14.x&os=ubuntu-latest",
			expected:  "matrix-key=node=14.x&os=ubuntu-latest",
		},
		{
			name: "both dimensions and key",
			dimensions: map[string]map[string]bool{
				"os": {"ubuntu-latest": true},
			},
			matrixKey: "node=14.x&os=ubuntu-latest",
			expected:  "matrix=os=ubuntu-latest, matrix-key=node=14.x&os=ubuntu-latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := model.NewMatrixSelector(tt.dimensions, tt.matrixKey)
			result := selector.Reason()
			assert.Equal(t, tt.expected, result)
		})
	}
}
