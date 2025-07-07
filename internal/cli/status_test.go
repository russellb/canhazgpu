package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

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
			ProcessInfo:     "1024MB used by PID 1234 (python train.py)",
		},
	}

	// Test that the function doesn't panic and produces expected output structure
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// Print header like the actual function does
	_, err := w.Write([]byte("GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION\tMODEL\n"))
	assert.NoError(t, err)

	_, err = w.Write([]byte("---\t------\t----\t--------\t----\t-------\t----------\t-----\n"))
	assert.NoError(t, err)

	// Test each status type
	for _, status := range statuses {
		displaySingleGPUStatus(w, status, true)
	}

	_ = w.Flush()
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

	// Check that it's formatted as a table (has tab separators)
	lines := strings.Split(output, "\n")
	assert.True(t, len(lines) >= 5, "Should have header, separator, and data lines")

	// Verify structured format (tabwriter converts tabs to spaces for alignment)
	for i, line := range lines[:4] { // Check first 4 lines (header, separator, and first two data lines)
		if strings.TrimSpace(line) != "" {
			// Check that line has multiple columns separated by spaces
			fields := strings.Fields(line)
			assert.True(t, len(fields) >= 4, "Line %d should have multiple columns: %s", i, line)
		}
	}
}

func TestDisplayGPUStatusTable_Structure(t *testing.T) {
	// Test that the table structure is maintained
	assert.NotNil(t, displayGPUStatusTable)
	assert.NotNil(t, displaySingleGPUStatus)

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
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	displaySingleGPUStatus(w, status, true)
	_ = w.Flush()

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
	w1 := tabwriter.NewWriter(&bufNoModel, 0, 0, 2, ' ', 0)

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
		_, _ = fmt.Fprintln(w1, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION\tMODEL")
		_, _ = fmt.Fprintln(w1, "---\t------\t----\t--------\t----\t-------\t----------\t-----")
	} else {
		_, _ = fmt.Fprintln(w1, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION")
		_, _ = fmt.Fprintln(w1, "---\t------\t----\t--------\t----\t-------\t----------")
	}

	for _, status := range statusesNoModel {
		displaySingleGPUStatus(w1, status, hasModels)
	}
	_ = w1.Flush()

	outputNoModel := bufNoModel.String()
	assert.NotContains(t, outputNoModel, "MODEL", "Should not include MODEL column when no models detected")
	// Check that the output has the expected columns without MODEL
	lines := strings.Split(outputNoModel, "\n")
	if len(lines) > 0 {
		headerLine := lines[0]
		assert.Contains(t, headerLine, "GPU", "Should have GPU column")
		assert.Contains(t, headerLine, "STATUS", "Should have STATUS column")
		assert.Contains(t, headerLine, "DETAILS", "Should have DETAILS column")
		assert.Contains(t, headerLine, "VALIDATION", "Should have VALIDATION column")
	}

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
	w2 := tabwriter.NewWriter(&bufWithModel, 0, 0, 2, ' ', 0)

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
		_, _ = fmt.Fprintln(w2, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION\tMODEL")
		_, _ = fmt.Fprintln(w2, "---\t------\t----\t--------\t----\t-------\t----------\t-----")
	} else {
		_, _ = fmt.Fprintln(w2, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION")
		_, _ = fmt.Fprintln(w2, "---\t------\t----\t--------\t----\t-------\t----------")
	}

	for _, status := range statusesWithModel {
		displaySingleGPUStatus(w2, status, hasModels2)
	}
	_ = w2.Flush()

	outputWithModel := bufWithModel.String()
	assert.Contains(t, outputWithModel, "MODEL", "Should include MODEL column when models detected")
	// Check that the output has the expected columns including MODEL
	lines2 := strings.Split(outputWithModel, "\n")
	if len(lines2) > 0 {
		headerLine2 := lines2[0]
		assert.Contains(t, headerLine2, "GPU", "Should have GPU column")
		assert.Contains(t, headerLine2, "STATUS", "Should have STATUS column")
		assert.Contains(t, headerLine2, "MODEL", "Should have MODEL column")
		assert.Contains(t, headerLine2, "DETAILS", "Should have DETAILS column")
		assert.Contains(t, headerLine2, "VALIDATION", "Should have VALIDATION column")
	}
	assert.Contains(t, outputWithModel, "meta-llama/Llama-2-7b-chat-hf", "Should display the detected model")
}
