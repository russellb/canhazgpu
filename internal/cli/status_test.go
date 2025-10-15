package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/stretchr/testify/assert"
)

func TestDisplayGPUStatusTable(t *testing.T) {
	// Create sample GPU status data
	statuses := []gpu.GPUStatusInfo{
		{
			GPUID:          0,
			Status:         "AVAILABLE",
			LastReleased:   time.Now().Add(-5 * time.Minute),
			ValidationInfo: "[validated: 1MB used]",
		},
		{
			GPUID:           1,
			Status:          "IN_USE",
			User:            "testuser",
			ReservationType: "manual",
			Duration:        2 * time.Hour,
			ExpiryTime:      time.Now().Add(1 * time.Hour),
			ValidationInfo:  "[validated: 512MB, 1 processes]",
		},
		{
			GPUID:           2,
			Status:          "UNRESERVED",
			UnreservedUsers: []string{"baduser"},
			ProcessInfo:     "1024MB used by 1 process",
		},
	}

	// Test that the function doesn't panic and produces expected output structure
	var buf bytes.Buffer
	tbl := table.NewWriter()
	tbl.SetOutputMirror(&buf)
	tbl.SetStyle(table.StyleLight)

	// Set header
	tbl.AppendHeader(table.Row{"GPU", "STATUS", "USER", "DURATION", "TYPE", "DETAILS", "VALIDATION", "MODEL"})

	// Test each status type
	for _, status := range statuses {
		addGPUStatusRow(tbl, status, true)
	}

	tbl.Render()
	output := buf.String()

	// Verify the output contains expected elements
	assert.Contains(t, output, "GPU", "Should contain header")
	assert.Contains(t, output, "STATUS", "Should contain status header")
	assert.Contains(t, output, "MODEL", "Should contain model header")
	assert.Contains(t, output, "AVAILABLE", "Should show available status")
	assert.Contains(t, output, "IN_USE", "Should show in-use status")
	assert.Contains(t, output, "UNRESERVED", "Should show unreserved status")
	assert.Contains(t, output, "testuser", "Should show user name")
	assert.Contains(t, output, "baduser", "Should show unreserved user")

	// Check that it's formatted as a table
	lines := strings.Split(output, "\n")
	assert.True(t, len(lines) >= 5, "Should have multiple lines of output")
	assert.NotEmpty(t, output, "Output should not be empty")
}

func TestDisplayGPUStatusTable_Structure(t *testing.T) {
	// Test that the table structure is maintained
	assert.NotNil(t, displayGPUStatusTable)
	assert.NotNil(t, addGPUStatusRow)

	// Test with empty status list (should not panic)
	var emptyStatuses []gpu.GPUStatusInfo
	assert.NotPanics(t, func() {
		displayGPUStatusTable(emptyStatuses)
	})
}

func TestDisplaySingleGPUStatus_ModelInfo(t *testing.T) {
	// Test that model information is displayed correctly
	status := gpu.GPUStatusInfo{
		GPUID:           0,
		Status:          "IN_USE",
		User:            "testuser",
		ReservationType: "run",
		Duration:        1 * time.Hour,
		ValidationInfo:  "[validated: 1GB, 1 processes]",
		ModelInfo: &gpu.ModelInfo{
			Provider: "meta-llama",
			Model:    "meta-llama/Llama-2-7b-chat-hf",
		},
	}

	var buf bytes.Buffer
	tbl := table.NewWriter()
	tbl.SetOutputMirror(&buf)
	tbl.SetStyle(table.StyleLight)

	addGPUStatusRow(tbl, status, true)
	tbl.Render()

	output := buf.String()
	assert.Contains(t, output, "meta-llama/Llama-2-7b-chat-hf", "Should display model information in MODEL column")
}

func TestDisplayGPUStatusTable_ConditionalModelColumn(t *testing.T) {
	// Test with GPUs that have no model information - MODEL column should be excluded
	statusesNoModel := []gpu.GPUStatusInfo{
		{
			GPUID:          0,
			Status:         "AVAILABLE",
			ValidationInfo: "[validated: 45MB used]",
			ModelInfo:      nil,
		},
		{
			GPUID:           1,
			Status:          "IN_USE",
			User:            "alice",
			ReservationType: "run",
			Duration:        30 * time.Minute,
			ValidationInfo:  "[validated: 1GB, 1 processes]",
			ModelInfo:       nil,
		},
	}

	var bufNoModel bytes.Buffer
	tbl1 := table.NewWriter()
	tbl1.SetOutputMirror(&bufNoModel)
	tbl1.SetStyle(table.StyleLight)

	// Check if any GPU has model information
	hasModels := false
	for _, status := range statusesNoModel {
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			hasModels = true
			break
		}
	}

	// Print header - exclude MODEL column if no models detected
	if hasModels {
		tbl1.AppendHeader(table.Row{"GPU", "STATUS", "USER", "DURATION", "TYPE", "DETAILS", "VALIDATION", "MODEL"})
	} else {
		tbl1.AppendHeader(table.Row{"GPU", "STATUS", "USER", "DURATION", "TYPE", "DETAILS", "VALIDATION"})
	}

	for _, status := range statusesNoModel {
		addGPUStatusRow(tbl1, status, hasModels)
	}
	tbl1.Render()

	outputNoModel := bufNoModel.String()
	assert.NotContains(t, outputNoModel, "MODEL", "Should not include MODEL column when no models detected")
	// Check that the output has the expected columns without MODEL
	assert.Contains(t, outputNoModel, "GPU", "Should have GPU column")
	assert.Contains(t, outputNoModel, "STATUS", "Should have STATUS column")
	assert.Contains(t, outputNoModel, "DETAILS", "Should have DETAILS column")
	assert.Contains(t, outputNoModel, "VALIDATION", "Should have VALIDATION column")

	// Test with GPUs that have model information - MODEL column should be included
	statusesWithModel := []gpu.GPUStatusInfo{
		{
			GPUID:          0,
			Status:         "AVAILABLE",
			ValidationInfo: "[validated: 45MB used]",
			ModelInfo:      nil,
		},
		{
			GPUID:           1,
			Status:          "IN_USE",
			User:            "alice",
			ReservationType: "run",
			Duration:        30 * time.Minute,
			ValidationInfo:  "[validated: 1GB, 1 processes]",
			ModelInfo: &gpu.ModelInfo{
				Provider: "meta-llama",
				Model:    "meta-llama/Llama-2-7b-chat-hf",
			},
		},
	}

	var bufWithModel bytes.Buffer
	tbl2 := table.NewWriter()
	tbl2.SetOutputMirror(&bufWithModel)
	tbl2.SetStyle(table.StyleLight)

	// Check if any GPU has model information
	hasModels2 := false
	for _, status := range statusesWithModel {
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			hasModels2 = true
			break
		}
	}

	// Print header - include MODEL column if models detected
	if hasModels2 {
		tbl2.AppendHeader(table.Row{"GPU", "STATUS", "USER", "DURATION", "TYPE", "DETAILS", "VALIDATION", "MODEL"})
	} else {
		tbl2.AppendHeader(table.Row{"GPU", "STATUS", "USER", "DURATION", "TYPE", "DETAILS", "VALIDATION"})
	}

	for _, status := range statusesWithModel {
		addGPUStatusRow(tbl2, status, hasModels2)
	}
	tbl2.Render()

	outputWithModel := bufWithModel.String()
	assert.Contains(t, outputWithModel, "MODEL", "Should include MODEL column when models detected")
	// Check that the output has the expected columns including MODEL
	assert.Contains(t, outputWithModel, "GPU", "Should have GPU column")
	assert.Contains(t, outputWithModel, "STATUS", "Should have STATUS column")
	assert.Contains(t, outputWithModel, "MODEL", "Should have MODEL column")
	assert.Contains(t, outputWithModel, "DETAILS", "Should have DETAILS column")
	assert.Contains(t, outputWithModel, "VALIDATION", "Should have VALIDATION column")
	assert.Contains(t, outputWithModel, "meta-llama/Llama-2-7b-chat-hf", "Should display the detected model")
}
