package gpu

import (
	"bufio"
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

// DetectModelFromProcesses analyzes GPU processes to detect running models
func DetectModelFromProcesses(processes []types.GPUProcessInfo) *ModelInfo {
	for _, proc := range processes {
		if modelInfo := detectModelFromProcessName(proc.ProcessName); modelInfo != nil {
			return modelInfo
		}
		
		// If no model found in current process, check parent process
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
	defer file.Close()

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
// - "python -m vllm.entrypoints.openai.api_server --model openai/whisper-large-v3 --port 8000" (--model flag)
func parseVLLMCommand(command string) *ModelInfo {
	parts := strings.Fields(command)
	
	model := ""
	
	// Check for --model flag anywhere in the command (used with Python module)
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "--model" && i+1 < len(parts) {
			model = parts[i+1]
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
		Model:    model,
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

