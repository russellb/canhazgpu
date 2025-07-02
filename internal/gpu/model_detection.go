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

// GetProviderIcon returns an SVG icon for the given provider
func GetProviderIcon(provider string) string {
	switch provider {
	case "openai":
		return getOpenAIIcon()
	case "meta-llama":
		return getMetaLlamaIcon()
	case "qwen":
		return getQwenIcon()
	case "deepseek-ai":
		return getDeepSeekIcon()
	default:
		return ""
	}
}

// SVG icon definitions
func getOpenAIIcon() string {
	return `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
		<path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z" fill="currentColor"/>
	</svg>`
}

func getMetaLlamaIcon() string {
	return `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
		<path d="M12 2C6.477 2 2 6.477 2 12s4.477 10 10 10 10-4.477 10-10S17.523 2 12 2zm0 18c-4.411 0-8-3.589-8-8s3.589-8 8-8 8 3.589 8 8-3.589 8-8 8zm-1-13h2v6h-2zm0 8h2v2h-2z" fill="currentColor"/>
		<circle cx="12" cy="8" r="1.5" fill="currentColor"/>
	</svg>`
}

func getQwenIcon() string {
	return `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
		<path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
	</svg>`
}

func getDeepSeekIcon() string {
	return `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
		<path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" fill="currentColor"/>
	</svg>`
}