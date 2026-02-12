package gpu

import (
	"testing"
)

func TestUnmarshalAMDSmiOutput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "ROCm 6.x bare array",
			input:     `[{"gpu": 0, "mem_usage": {}}, {"gpu": 1, "mem_usage": {}}]`,
			wantCount: 2,
		},
		{
			name:      "ROCm 7.x wrapped format",
			input:     `{"gpu_data": [{"gpu": 0, "mem_usage": {}}, {"gpu": 1, "mem_usage": {}}]}`,
			wantCount: 2,
		},
		{
			name:      "ROCm 7.x empty gpu_data",
			input:     `{"gpu_data": []}`,
			wantCount: 0,
		},
		{
			name:      "ROCm 6.x empty array",
			input:     `[]`,
			wantCount: 0,
		},
		{
			name:      "ROCm 6.x single GPU",
			input:     `[{"gpu": 0}]`,
			wantCount: 1,
		},
		{
			name:      "ROCm 7.x single GPU",
			input:     `{"gpu_data": [{"gpu": 0}]}`,
			wantCount: 1,
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantErr: true,
		},
		{
			name:    "JSON object without gpu_data",
			input:   `{"other_field": [{"gpu": 0}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := unmarshalAMDSmiOutput([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(result) != tt.wantCount {
				t.Errorf("expected %d items, got %d", tt.wantCount, len(result))
			}
		})
	}
}
