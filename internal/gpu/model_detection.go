package gpu

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/russellb/canhazgpu/internal/types"
)

// ModelInfo represents detected model information
type ModelInfo struct {
	Provider string `json:"provider,omitempty"` // e.g., "openai", "meta-llama", "qwen", "deepseek-ai"
	Model    string `json:"model,omitempty"`    // e.g., "openai/whisper-large-v3"
}

// truncateModelName truncates model names longer than 50 characters
func truncateModelName(model string) string {
	if len(model) > 50 {
		return model[:50] + "..."
	}
	return model
}

// DetectModelFromProcesses analyzes GPU processes to detect running models
func DetectModelFromProcesses(processes []types.GPUProcessInfo) *ModelInfo {
	for _, proc := range processes {
		// First try the process name from nvidia-smi
		if modelInfo := detectModelFromProcessName(proc.ProcessName); modelInfo != nil {
			return modelInfo
		}

		// If process name doesn't contain model info, try to get full command line
		// This is important for Python processes where nvidia-smi only shows "python3"
		// but the full command line contains the actual script and arguments
		if fullCmdline, err := getProcessCommandLine(proc.PID); err == nil && fullCmdline != "" {
			if modelInfo := detectModelFromProcessName(fullCmdline); modelInfo != nil {
				return modelInfo
			}
		}

		// If still no model found, check parent process
		// This handles cases where the GPU process is spawned by a parent with model info
		if modelInfo := detectModelFromParentProcess(proc.PID); modelInfo != nil {
			return modelInfo
		}
	}
	return nil
}

// detectModelFromProcessName parses a process name/command to extract model information
func detectModelFromProcessName(processName string) *ModelInfo {
	// Look for vLLM commands in various forms:
	// 1. vllm serve model_name
	// 2. python -m vllm.entrypoints.openai.api_server --model model_name
	// 3. /path/to/vllm serve model_name
	// 4. VLLM_USE_V1=1 canhazgpu run -- vllm serve model_name
	parts := strings.Fields(processName)

	// Check for lm_eval commands FIRST (before generic model detection)
	// Handle cases like:
	// - lm_eval --model ...
	// - python lm_eval --model ...
	// - /path/to/python /path/to/lm_eval --model ...
	// - python -m lm_eval --model ...
	lmEvalFound := false
	for _, part := range parts {
		// Check if this part is lm_eval (handles direct execution and python script)
		if strings.HasSuffix(part, "lm_eval") || part == "lm_eval" {
			lmEvalFound = true
			break
		}
	}

	if lmEvalFound {
		return parseLMEvalCommand(processName)
	}

	// Check if this is a vLLM command by looking for "vllm" in the command
	vllmFound := false
	for _, part := range parts {
		// Check if this part contains "vllm" (handles both "vllm" and paths like "/usr/bin/vllm")
		if strings.Contains(part, "vllm") {
			// Also check if it ends with "vllm" or contains "vllm." to handle paths and modules
			if strings.HasSuffix(part, "vllm") || strings.Contains(part, "vllm.") {
				vllmFound = true
				break
			}
		}
	}

	if vllmFound {
		return parseVLLMCommand(processName)
	}

	// Try generic model detection for any command with --model arguments
	if modelInfo := parseGenericModelCommand(processName); modelInfo != nil {
		return modelInfo
	}

	// Add more model detection patterns here as needed
	// Could extend to detect other inference engines like TGI, SGLang, etc.

	return nil
}

// getParentPID gets the parent process ID for a given PID
func getParentPID(pid int) (int, error) {
	// Try /proc filesystem first
	statFile := fmt.Sprintf("/proc/%d/stat", pid)
	file, err := os.Open(statFile)
	if err != nil {
		return -1, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return -1, fmt.Errorf("failed to read stat file")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 4 {
		return -1, fmt.Errorf("invalid stat file format")
	}

	// PPID is the 4th field in /proc/PID/stat
	ppid, err := strconv.Atoi(fields[3])
	if err != nil {
		return -1, fmt.Errorf("failed to parse PPID: %v", err)
	}

	return ppid, nil
}

// getProcessCommandLine gets the full command line for a given PID
func getProcessCommandLine(pid int) (string, error) {
	// Try /proc filesystem first
	cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", pid)
	content, err := os.ReadFile(cmdlineFile)
	if err != nil {
		return "", err
	}

	// Replace null bytes with spaces
	cmdline := strings.ReplaceAll(string(content), "\x00", " ")
	cmdline = strings.TrimSpace(cmdline)

	if cmdline == "" {
		return "", fmt.Errorf("empty command line")
	}

	return cmdline, nil
}

// detectModelFromParentProcess checks parent processes for model information
func detectModelFromParentProcess(pid int) *ModelInfo {
	// Check up to 3 levels of parent processes to avoid infinite loops
	for depth := 0; depth < 3; depth++ {
		parentPID, err := getParentPID(pid)
		if err != nil || parentPID <= 1 {
			// No parent or reached init process
			break
		}

		// Get parent process command line
		cmdline, err := getProcessCommandLine(parentPID)
		if err != nil {
			// Try next parent level
			pid = parentPID
			continue
		}

		// Check if parent process contains model information
		if modelInfo := detectModelFromProcessName(cmdline); modelInfo != nil {
			return modelInfo
		}

		pid = parentPID
	}

	return nil
}

// parseVLLMCommand extracts model information from vllm commands
// Examples:
// - "vllm serve openai/whisper-large-v3 --port 8000" (positional model)
// - "vllm serve --model openai/whisper-large-v3 --port 8000" (--model flag with serve)
// - "python -m vllm.entrypoints.openai.api_server --model openai/whisper-large-v3 --port 8000" (--model flag)
func parseVLLMCommand(command string) *ModelInfo {
	parts := strings.Fields(command)

	model := ""

	// Check for --model flag anywhere in the command (works with both direct vllm serve and Python module)
	for i := 0; i < len(parts); i++ {
		// Handle --model value format
		if parts[i] == "--model" && i+1 < len(parts) {
			model = parts[i+1]
			break
		}
		// Handle --model=value format
		if strings.HasPrefix(parts[i], "--model=") {
			model = strings.TrimPrefix(parts[i], "--model=")
			break
		}
	}

	// If no --model flag found, look for "serve" with positional model
	if model == "" {
		serveIndex := -1
		for i, part := range parts {
			if part == "serve" {
				serveIndex = i
				break
			}
		}

		if serveIndex != -1 && serveIndex+1 < len(parts) {
			candidate := parts[serveIndex+1]
			if !strings.HasPrefix(candidate, "--") {
				model = candidate
			}
		}
	}

	if model == "" {
		return nil
	}

	// Extract provider from model name (part before the first /)
	provider := extractProviderFromModel(model)

	return &ModelInfo{
		Provider: provider,
		Model:    truncateModelName(model),
	}
}

// extractProviderFromModel extracts the provider name from a model identifier
// Examples:
// "openai/whisper-large-v3" -> "openai"
// "meta-llama/Llama-2-7b-chat-hf" -> "meta-llama"
// "qwen/Qwen2-7B-Instruct" -> "qwen"
// "deepseek-ai/deepseek-coder-6.7b-instruct" -> "deepseek-ai"
func extractProviderFromModel(model string) string {
	if slashIndex := strings.Index(model, "/"); slashIndex != -1 {
		return model[:slashIndex]
	}
	return ""
}

// parseLMEvalCommand extracts model information from lm_eval commands
// Examples:
// - lm_eval --model vllm --model_args {"pretrained": "meta-llama/Meta-Llama-3-8B-Instruct", "gpu_memory_utilization": 0.8} --tasks gsm8k
// - lm_eval --model vllm --model_args pretrained=/path/to/Meta-Llama-3.1-8B-Instruct-custom,dtype=auto,... --tasks ruler
func parseLMEvalCommand(command string) *ModelInfo {
	// Look for --model_args parameter
	modelArgsIndex := strings.Index(command, "--model_args")
	if modelArgsIndex == -1 {
		return nil
	}

	// Extract the argument after --model_args
	remaining := command[modelArgsIndex+len("--model_args"):]
	remaining = strings.TrimSpace(remaining)

	// Check if it starts with JSON
	if strings.HasPrefix(remaining, "{") {
		return parseLMEvalJSONArgs(remaining)
	}

	// Otherwise, parse as key=value format
	return parseLMEvalKeyValueArgs(remaining)
}

// parseLMEvalJSONArgs handles JSON-formatted model_args
func parseLMEvalJSONArgs(remaining string) *ModelInfo {
	// Find the matching closing brace
	jsonEnd := -1
	braceCount := 0
	inQuotes := false
	escapeNext := false

	for i := 0; i < len(remaining); i++ {
		if escapeNext {
			escapeNext = false
			continue
		}

		switch remaining[i] {
		case '\\':
			escapeNext = true
		case '"':
			inQuotes = !inQuotes
		case '{':
			if !inQuotes {
				braceCount++
			}
		case '}':
			if !inQuotes {
				braceCount--
				if braceCount == 0 {
					jsonEnd = i + 1
					break
				}
			}
		}
	}

	if jsonEnd == -1 {
		return nil
	}

	jsonStr := remaining[:jsonEnd]

	// Parse the JSON to extract model arguments
	var modelArgs map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &modelArgs); err != nil {
		return nil
	}

	// Extract the "pretrained" field
	pretrained, exists := modelArgs["pretrained"]
	if !exists {
		return nil
	}

	// Convert to string
	model, ok := pretrained.(string)
	if !ok {
		return nil
	}

	if model == "" {
		return nil
	}

	// Extract provider from model name
	provider := extractProviderFromModel(model)

	return &ModelInfo{
		Provider: provider,
		Model:    truncateModelName(model),
	}
}

// parseLMEvalKeyValueArgs handles key=value formatted model_args
func parseLMEvalKeyValueArgs(remaining string) *ModelInfo {
	// Find the end of model_args (next flag starting with -- or end of string)
	argsEnd := strings.Index(remaining, " --")
	args := remaining
	if argsEnd != -1 {
		args = remaining[:argsEnd]
	}

	// Split by commas to get individual key=value pairs
	pairs := strings.Split(args, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if strings.HasPrefix(pair, "pretrained=") {
			modelPath := strings.TrimPrefix(pair, "pretrained=")

			// If it's an absolute path, extract the model name from the filename
			if strings.HasPrefix(modelPath, "/") {
				modelPath = extractModelFromPath(modelPath)
			}

			if modelPath == "" {
				return nil
			}

			// Extract provider from model name
			provider := extractProviderFromModel(modelPath)

			return &ModelInfo{
				Provider: provider,
				Model:    truncateModelName(modelPath),
			}
		}
	}

	return nil
}

// extractModelFromPath extracts model information from file paths
// Example: /path/to/Meta-Llama-3.1-8B-Instruct-custom -> meta-llama/Meta-Llama-3.1-8B-Instruct-custom
func extractModelFromPath(path string) string {
	// Get the last component of the path
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}

	filename := parts[len(parts)-1]
	filenameLower := strings.ToLower(filename)

	// Known model patterns to match at the beginning of filenames (case-insensitive)
	knownPatterns := []struct {
		prefix   string
		provider string
	}{
		{"meta-llama", "meta-llama"},
		{"llama", "meta-llama"},
		{"qwen", "qwen"},
		{"deepseek", "deepseek-ai"},
		{"mistral", "mistralai"},
		{"mixtral", "mistralai"},
		{"gpt", "openai"},
		{"whisper", "openai"},
	}

	for _, pattern := range knownPatterns {
		if strings.HasPrefix(filenameLower, pattern.prefix) {
			// Return in the format provider/filename (keeping the whole filename)
			return pattern.provider + "/" + filename
		}
	}

	// If no known pattern matches, return empty
	return ""
}

// parseGenericModelCommand extracts model information from any command with --model arguments
// Examples:
// - "python train.py --model openai/whisper-large-v3 --epochs 10"
// - "some-inference-server --model=meta-llama/Llama-2-7b-chat-hf --port 8080"
// - "custom-tool --batch-size 32 --model qwen/Qwen2-7B-Instruct --output ./results"
func parseGenericModelCommand(command string) *ModelInfo {
	parts := strings.Fields(command)

	model := ""

	// Check for --model flag anywhere in the command
	for i := 0; i < len(parts); i++ {
		// Handle --model value format
		if parts[i] == "--model" && i+1 < len(parts) {
			model = parts[i+1]
			break
		}
		// Handle --model=value format
		if strings.HasPrefix(parts[i], "--model=") {
			model = strings.TrimPrefix(parts[i], "--model=")
			break
		}
	}

	if model == "" {
		return nil
	}

	// Extract provider from model name (part before the first /)
	provider := extractProviderFromModel(model)

	return &ModelInfo{
		Provider: provider,
		Model:    truncateModelName(model),
	}
}
