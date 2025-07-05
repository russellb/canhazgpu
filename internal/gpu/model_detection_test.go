package gpu

import (
	"os"
	"testing"

	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestDetectModelFromProcesses(t *testing.T) {
	tests := []struct {
		name      string
		processes []types.GPUProcessInfo
		expected  *ModelInfo
	}{
		{
			name:      "No processes",
			processes: []types.GPUProcessInfo{},
			expected:  nil,
		},
		{
			name: "Non-vLLM process",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "python train.py", User: "user1"},
			},
			expected: nil,
		},
		{
			name: "vLLM serve with positional model argument",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "python -m vllm.entrypoints.openai.api_server serve openai/whisper-large-v3 --port 8000", User: "user1"},
			},
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name: "vLLM serve with positional model and flags",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "vllm serve meta-llama/Llama-2-7b-chat-hf --port 8080", User: "user1"},
			},
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name: "Python module vLLM with --model flag",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "python -m vllm.entrypoints.openai.api_server --model qwen/Qwen2-7B-Instruct --host 0.0.0.0 --port 8080", User: "user1"},
			},
			expected: &ModelInfo{Provider: "qwen", Model: "qwen/Qwen2-7B-Instruct"},
		},
		{
			name: "Multiple processes, first has vLLM",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "vllm serve deepseek-ai/deepseek-coder-6.7b-instruct", User: "user1"},
				{PID: 5678, ProcessName: "python train.py", User: "user2"},
			},
			expected: &ModelInfo{Provider: "deepseek-ai", Model: "deepseek-ai/deepseek-coder-6.7b-instruct"},
		},
		{
			name: "vLLM serve with wrapper command (canhazgpu run)",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "VLLM_USE_V1=1 canhazgpu run -- vllm serve openai/whisper-large-v3 --enforce-eager --port 8123", User: "user1"},
			},
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name: "vLLM serve with --model flag",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "vllm serve --model mistralai/Mistral-7B-Instruct-v0.1 --port 8000", User: "user1"},
			},
			expected: &ModelInfo{Provider: "mistralai", Model: "mistralai/Mistral-7B-Instruct-v0.1"},
		},
		{
			name: "vLLM serve with absolute path",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "/usr/local/bin/vllm serve meta-llama/Llama-2-7b-chat-hf --port 8080", User: "user1"},
			},
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name: "Process without model but with current process PID",
			processes: []types.GPUProcessInfo{
				{PID: os.Getpid(), ProcessName: "python worker.py", User: "user1"},
			},
			expected: nil, // Parent process detection will be tested separately
		},
		{
			name: "Generic command with --model flag",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "python train.py --model openai/whisper-large-v3 --epochs 10", User: "user1"},
			},
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name: "Generic command with --model=value format",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "some-inference-server --model=meta-llama/Llama-2-7b-chat-hf --port 8080", User: "user1"},
			},
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name: "Generic command with --model in middle of arguments",
			processes: []types.GPUProcessInfo{
				{PID: 1234, ProcessName: "custom-tool --batch-size 32 --model qwen/Qwen2-7B-Instruct --output ./results", User: "user1"},
			},
			expected: &ModelInfo{Provider: "qwen", Model: "qwen/Qwen2-7B-Instruct"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectModelFromProcesses(tt.processes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseVLLMCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected *ModelInfo
	}{
		{
			name:     "Simple vllm serve with model",
			command:  "vllm serve openai/whisper-large-v3",
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name:     "vllm serve with positional model",
			command:  "vllm serve meta-llama/Llama-2-7b-chat-hf",
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name:     "vllm serve with other flags before model",
			command:  "vllm serve --host 0.0.0.0 --port 8080 qwen/Qwen2-7B-Instruct",
			expected: nil, // Model is after other flags, should not be detected as positional
		},
		{
			name:     "python module with --model flag",
			command:  "python -m vllm.entrypoints.openai.api_server --host 0.0.0.0 --model qwen/Qwen2-7B-Instruct --port 8080",
			expected: &ModelInfo{Provider: "qwen", Model: "qwen/Qwen2-7B-Instruct"},
		},
		{
			name:     "Complex command with python module",
			command:  "python -m vllm.entrypoints.openai.api_server serve openai/whisper-large-v3 --port 8000",
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name:     "No serve command",
			command:  "vllm generate openai/whisper-large-v3",
			expected: nil,
		},
		{
			name:     "No model specified",
			command:  "vllm serve --port 8080",
			expected: nil,
		},
		{
			name:     "vllm serve with wrapper command",
			command:  "VLLM_USE_V1=1 canhazgpu run -- vllm serve openai/whisper-large-v3 --enforce-eager --port 8123",
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name:     "vllm serve with environment variables",
			command:  "CUDA_VISIBLE_DEVICES=0,1 vllm serve meta-llama/Llama-2-7b-chat-hf",
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name:     "vllm as absolute path",
			command:  "/usr/local/bin/vllm serve qwen/Qwen2-7B-Instruct --port 8080",
			expected: &ModelInfo{Provider: "qwen", Model: "qwen/Qwen2-7B-Instruct"},
		},
		{
			name:     "vllm as relative path",
			command:  "./venv/bin/vllm serve deepseek-ai/deepseek-coder-6.7b-instruct",
			expected: &ModelInfo{Provider: "deepseek-ai", Model: "deepseek-ai/deepseek-coder-6.7b-instruct"},
		},
		{
			name:     "vllm serve with --model flag",
			command:  "vllm serve --model openai/whisper-large-v3 --port 8000",
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name:     "vllm serve with --model flag and other arguments",
			command:  "vllm serve --host 0.0.0.0 --model meta-llama/Llama-2-7b-chat-hf --port 8080 --enforce-eager",
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name:     "vllm serve with --model=value format",
			command:  "vllm serve --model=meta-llama/Llama-3.2-1B-Instruct --port 8000",
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-3.2-1B-Instruct"},
		},
		{
			name:     "python module with --model=value format",
			command:  "python -m vllm.entrypoints.openai.api_server --host 0.0.0.0 --model=qwen/Qwen2-7B-Instruct --port 8080",
			expected: &ModelInfo{Provider: "qwen", Model: "qwen/Qwen2-7B-Instruct"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVLLMCommand(tt.command)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractProviderFromModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{
			name:     "OpenAI model",
			model:    "openai/whisper-large-v3",
			expected: "openai",
		},
		{
			name:     "Meta Llama model",
			model:    "meta-llama/Llama-2-7b-chat-hf",
			expected: "meta-llama",
		},
		{
			name:     "Qwen model",
			model:    "qwen/Qwen2-7B-Instruct",
			expected: "qwen",
		},
		{
			name:     "DeepSeek model",
			model:    "deepseek-ai/deepseek-coder-6.7b-instruct",
			expected: "deepseek-ai",
		},
		{
			name:     "Model without provider",
			model:    "llama-2-7b",
			expected: "",
		},
		{
			name:     "Empty model",
			model:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractProviderFromModel(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetParentPID(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		expected bool // true if should succeed
	}{
		{
			name:     "Current process",
			pid:      os.Getpid(),
			expected: true,
		},
		{
			name:     "Invalid PID",
			pid:      -1,
			expected: false,
		},
		{
			name:     "Non-existent PID",
			pid:      999999,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getParentPID(tt.pid)
			if tt.expected {
				assert.NoError(t, err)
				assert.Greater(t, result, 0)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGetProcessCommandLine(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		expected bool // true if should succeed
	}{
		{
			name:     "Current process",
			pid:      os.Getpid(),
			expected: true,
		},
		{
			name:     "Invalid PID",
			pid:      -1,
			expected: false,
		},
		{
			name:     "Non-existent PID",
			pid:      999999,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getProcessCommandLine(tt.pid)
			if tt.expected {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				// Should contain "go" since this is a Go test
				assert.Contains(t, result, "go")
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestParseGenericModelCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected *ModelInfo
	}{
		{
			name:     "Python training script with --model flag",
			command:  "python train.py --model openai/whisper-large-v3 --epochs 10",
			expected: &ModelInfo{Provider: "openai", Model: "openai/whisper-large-v3"},
		},
		{
			name:     "Inference server with --model=value format",
			command:  "some-inference-server --model=meta-llama/Llama-2-7b-chat-hf --port 8080",
			expected: &ModelInfo{Provider: "meta-llama", Model: "meta-llama/Llama-2-7b-chat-hf"},
		},
		{
			name:     "Custom tool with --model in middle of arguments",
			command:  "custom-tool --batch-size 32 --model qwen/Qwen2-7B-Instruct --output ./results",
			expected: &ModelInfo{Provider: "qwen", Model: "qwen/Qwen2-7B-Instruct"},
		},
		{
			name:     "Command with --model flag but no value",
			command:  "python script.py --model",
			expected: nil,
		},
		{
			name:     "Command without --model flag",
			command:  "python script.py --epochs 10 --batch-size 32",
			expected: nil,
		},
		{
			name:     "Command with --model=empty value",
			command:  "tool --model= --other-flag",
			expected: nil,
		},
		{
			name:     "Model without provider (no slash)",
			command:  "tool --model llama-2-7b --port 8080",
			expected: &ModelInfo{Provider: "", Model: "llama-2-7b"},
		},
		{
			name:     "Complex model path with multiple slashes",
			command:  "tool --model huggingface/transformers/bert-base-uncased --task classification",
			expected: &ModelInfo{Provider: "huggingface", Model: "huggingface/transformers/bert-base-uncased"},
		},
		{
			name:     "Environment variables and complex command",
			command:  "CUDA_VISIBLE_DEVICES=0,1 python -m some.module --config config.json --model=deepseek-ai/deepseek-coder-6.7b-instruct --verbose",
			expected: &ModelInfo{Provider: "deepseek-ai", Model: "deepseek-ai/deepseek-coder-6.7b-instruct"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGenericModelCommand(tt.command)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectModelFromParentProcess(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		expected *ModelInfo
	}{
		{
			name:     "Current process (no vLLM in parents)",
			pid:      os.Getpid(),
			expected: nil,
		},
		{
			name:     "Invalid PID",
			pid:      -1,
			expected: nil,
		},
		{
			name:     "Non-existent PID",
			pid:      999999,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectModelFromParentProcess(tt.pid)
			assert.Equal(t, tt.expected, result)
		})
	}
}
