package cli

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/spf13/cobra"
)

var (
	webPort int
	webHost string
	webDemo bool
)

//go:embed static/*
var staticFiles embed.FS

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start a web server for GPU status monitoring",
	Long:  `Start a web server that provides a dashboard for monitoring GPU status and usage reports.`,
	RunE:  runWeb,
}

func init() {
	webCmd.Flags().IntVarP(&webPort, "port", "p", 8080, "Port to run the web server on")
	webCmd.Flags().StringVar(&webHost, "host", "0.0.0.0", "Host to bind the web server to")
	webCmd.Flags().BoolVar(&webDemo, "demo", false, "Run in demo mode with simulated data")
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	var server *webServer

	if webDemo {
		// Demo mode - no Redis connection needed
		fmt.Println("Starting web server in DEMO mode")
		server = &webServer{
			client: nil,
			engine: nil,
			demo:   true,
		}
	} else {
		// Initialize Redis client
		config := getConfig()
		client := redis_client.NewClient(config)
		defer func() {
			if err := client.Close(); err != nil {
				fmt.Printf("Warning: failed to close Redis client: %v\n", err)
			}
		}()

		// Test connection
		if err := client.Ping(ctx); err != nil {
			return fmt.Errorf("failed to connect to Redis: %v", err)
		}

		// Create server
		server = &webServer{
			client: client,
			engine: gpu.NewAllocationEngine(client, config),
			demo:   false,
		}
	}

	// Set up routes
	http.HandleFunc("/", server.handleIndex)
	http.HandleFunc("/api/status", server.handleAPIStatus)
	http.HandleFunc("/api/report", server.handleAPIReport)
	http.Handle("/static/", http.FileServer(http.FS(staticFiles)))

	// Start server
	addr := fmt.Sprintf("%s:%d", webHost, webPort)
	fmt.Printf("Starting web server on http://%s\n", addr)
	return http.ListenAndServe(addr, nil)
}

type webServer struct {
	client *redis_client.Client
	engine *gpu.AllocationEngine
	demo   bool
}

func (ws *webServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Get system hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>canhazgpu Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        
        :root {
            --bg-primary: #0f0f0f;
            --bg-secondary: #1a1a1a;
            --bg-tertiary: #252525;
            --text-primary: #e0e0e0;
            --text-secondary: #888;
            --border-color: #333;
            --accent-color: #4CAF50;
            --card-hover: rgba(76, 175, 80, 0.2);
        }
        
        [data-theme="light"] {
            --bg-primary: #ffffff;
            --bg-secondary: #f5f5f5;
            --bg-tertiary: #e0e0e0;
            --text-primary: #333333;
            --text-secondary: #666666;
            --border-color: #d0d0d0;
            --accent-color: #2E7D32;
            --card-hover: rgba(46, 125, 50, 0.1);
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.6;
            transition: background-color 0.3s ease, color 0.3s ease;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background: var(--bg-secondary);
            padding: 20px 0;
            margin-bottom: 30px;
            border-bottom: 1px solid var(--border-color);
        }
        .header-content {
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .header-text {
            flex: 1;
        }
        .header-icons {
            display: flex;
            gap: 15px;
            align-items: center;
        }
        .theme-toggle {
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            padding: 8px;
            border-radius: 4px;
            cursor: pointer;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .theme-toggle:hover {
            background: var(--accent-color);
            color: white;
        }
        .theme-toggle svg {
            width: 24px;
            height: 24px;
            fill: currentColor;
        }
        .header-icons a {
            color: var(--text-secondary);
            text-decoration: none;
            padding: 8px;
            border-radius: 4px;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .header-icons a:hover {
            color: var(--accent-color);
            background: var(--bg-tertiary);
        }
        .header-icons svg {
            width: 24px;
            height: 24px;
            fill: currentColor;
        }
        h1 {
            color: var(--accent-color);
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        .subtitle {
            color: var(--text-secondary);
            font-size: 1.2em;
        }
        .section {
            background: var(--bg-secondary);
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 30px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.3);
        }
        h2 {
            color: var(--accent-color);
            margin-bottom: 20px;
            font-size: 1.8em;
        }
        .gpu-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .gpu-card {
            background: var(--bg-tertiary);
            border-radius: 6px;
            padding: 12px;
            border: 1px solid var(--border-color);
            transition: all 0.3s ease;
            cursor: pointer;
        }
        .gpu-card:hover {
            border-color: var(--accent-color);
            box-shadow: 0 4px 12px var(--card-hover);
        }
        .gpu-card.expanded {
            padding: 15px;
        }
        .gpu-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            position: relative;
            min-height: 60px;
            padding: 5px 0;
        }
        .gpu-header-left {
            display: flex;
            align-items: center;
            gap: 15px;
            z-index: 2;
        }
        .gpu-header-center {
            position: absolute;
            left: 50%;
            top: 50%;
            transform: translate(-50%, -50%);
            text-align: center;
            z-index: 1;
            pointer-events: none;
            max-width: 40%;
            line-height: 1.3;
        }
        .status-badge {
            z-index: 2;
        }
        .model-icon {
            display: flex;
            align-items: center;
            justify-content: center;
            width: 24px;
            height: 24px;
            flex-shrink: 0;
        }
        .model-icon svg {
            width: 20px;
            height: 20px;
            fill: var(--accent-color);
            opacity: 0.8;
        }
        .expand-icon {
            width: 20px;
            height: 20px;
            fill: var(--text-secondary);
            transition: transform 0.3s ease;
            flex-shrink: 0;
        }
        .gpu-card.expanded .expand-icon {
            transform: rotate(90deg);
        }
        .gpu-id {
            font-size: 1.2em;
            font-weight: bold;
            text-align: center;
        }
        .gpu-summary {
            font-size: 0.9em;
            color: var(--text-secondary);
            margin-left: 5px;
            text-align: center;
            word-wrap: break-word;
            overflow-wrap: break-word;
        }
        .status-badge {
            padding: 5px 12px;
            border-radius: 20px;
            font-size: 0.85em;
            font-weight: 600;
            text-transform: uppercase;
            flex-shrink: 0;
            position: relative;
            z-index: 2;
        }
        .status-available { background: #2e7d32; color: white; }
        .status-in-use { background: #1976d2; color: white; }
        .status-unreserved { background: #d32f2f; color: white; }
        .gpu-details {
            font-size: 0.9em;
            color: var(--text-secondary);
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid var(--border-color);
            display: none;
        }
        .gpu-hardware-info {
            color: var(--accent-color);
            font-weight: 600;
            margin-bottom: 8px;
            padding-bottom: 8px;
            border-bottom: 1px solid var(--border-color);
        }
        .memory-usage {
            display: flex;
            align-items: center;
            gap: 10px;
            margin: 8px 0;
        }
        .memory-bar {
            flex: 1;
            height: 8px;
            background: var(--bg-secondary);
            border-radius: 4px;
            overflow: hidden;
            border: 1px solid var(--border-color);
        }
        .memory-fill {
            height: 100%;
            transition: width 0.3s ease;
            border-radius: 3px;
        }
        .memory-low { background: var(--accent-color); }
        .memory-medium { background: #FF9800; }
        .memory-high { background: #f44336; }
        .memory-text {
            font-family: 'SF Mono', Monaco, monospace;
            font-size: 0.8em;
            color: var(--text-secondary);
            min-width: 80px;
            text-align: right;
        }
        .gpu-card.expanded .gpu-details {
            display: block;
        }
        .gpu-details div {
            margin: 5px 0;
        }
        .controls {
            display: flex;
            gap: 20px;
            align-items: center;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }
        .control-group {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        label {
            color: var(--text-secondary);
            font-weight: 500;
        }
        select, button {
            background: var(--bg-tertiary);
            color: var(--text-primary);
            border: 1px solid var(--border-color);
            padding: 8px 15px;
            border-radius: 4px;
            font-size: 14px;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        select:hover, button:hover {
            border-color: var(--accent-color);
            background: var(--bg-secondary);
        }
        button:active {
            transform: translateY(1px);
        }
        .usage-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        .usage-table th, .usage-table td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid var(--border-color);
        }
        .usage-table th {
            background: var(--bg-tertiary);
            color: var(--accent-color);
            font-weight: 600;
        }
        .usage-table tr:hover {
            background: var(--bg-tertiary);
        }
        .usage-bar {
            display: inline-block;
            height: 20px;
            background: var(--accent-color);
            border-radius: 3px;
            margin-right: 10px;
            vertical-align: middle;
        }
        .loading {
            text-align: center;
            padding: 40px;
            color: var(--text-secondary);
        }
        .error {
            background: #d32f2f;
            color: white;
            padding: 15px;
            border-radius: 6px;
            margin: 20px 0;
        }
        .timestamp {
            color: var(--text-secondary);
            font-size: 0.85em;
            margin-top: 10px;
        }
        @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.5; }
            100% { opacity: 1; }
        }
        .refreshing {
            animation: pulse 1s infinite;
        }
        @keyframes skeleton-pulse {
            0% { background-color: var(--bg-tertiary); }
            50% { background-color: var(--border-color); }
            100% { background-color: var(--bg-tertiary); }
        }
        .skeleton-pulse {
            animation: skeleton-pulse 1.5s ease-in-out infinite;
            border-radius: 6px;
        }
    </style>
</head>
<body>
    <header>
        <div class="container">
            <div class="header-content">
                <div class="header-text">
                    <h1>canhazgpu Dashboard{{if .Demo}} (DEMO){{end}}</h1>
                    <div class="subtitle">GPU Reservation System Monitor - {{.Hostname}}</div>
                </div>
                <div class="header-icons">
                    <button class="theme-toggle" onclick="toggleTheme()" title="Toggle dark/light mode">
                        <svg id="theme-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path d="M17.75,4.09L15.22,6.03L16.13,9.09L13.5,7.28L10.87,9.09L11.78,6.03L9.25,4.09L12.44,4L13.5,1L14.56,4L17.75,4.09M21.25,11L19.61,12.25L20.2,14.23L18.5,13.06L16.8,14.23L17.39,12.25L15.75,11L17.81,10.95L18.5,9L19.19,10.95L21.25,11M18.97,15.95C19.8,15.87 20.69,17.05 20.16,17.8C19.84,18.25 19.5,18.67 19.08,19.07C15.17,23 8.84,23 4.94,19.07C1.03,15.17 1.03,8.83 4.94,4.93C5.34,4.53 5.76,4.17 6.21,3.85C6.96,3.32 8.14,4.21 8.06,5.04C7.79,7.9 8.75,10.87 10.95,13.06C13.14,15.26 16.1,16.22 18.97,15.95M17.33,17.97C14.5,17.81 11.7,16.64 9.53,14.5C7.36,12.31 6.2,9.5 6.04,6.68C3.23,9.82 3.34,14.4 6.35,17.41C9.37,20.43 14,20.54 17.33,17.97Z"/>
                        </svg>
                    </button>
                    <a href="https://blog.russellbryant.net/canhazgpu/" target="_blank" title="Documentation">
                        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path d="M14,2H6A2,2 0 0,0 4,4V20A2,2 0 0,0 6,22H18A2,2 0 0,0 20,20V8L14,2M18,20H6V4H13V9H18V20Z"/>
                        </svg>
                    </a>
                    <a href="https://github.com/russellb/canhazgpu" target="_blank" title="GitHub Repository">
                        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                            <path d="M12,2A10,10 0 0,0 2,12C2,16.42 4.87,20.17 8.84,21.5C9.34,21.58 9.5,21.27 9.5,21C9.5,20.77 9.5,20.14 9.5,19.31C6.73,19.91 6.14,17.97 6.14,17.97C5.68,16.81 5.03,16.5 5.03,16.5C4.12,15.88 5.1,15.9 5.1,15.9C6.1,15.97 6.63,16.93 6.63,16.93C7.5,18.45 8.97,18 9.54,17.76C9.63,17.11 9.89,16.67 10.17,16.42C7.95,16.17 5.62,15.31 5.62,11.5C5.62,10.39 6,9.5 6.65,8.79C6.55,8.54 6.2,7.5 6.75,6.15C6.75,6.15 7.59,5.88 9.5,7.17C10.29,6.95 11.15,6.84 12,6.84C12.85,6.84 13.71,6.95 14.5,7.17C16.41,5.88 17.25,6.15 17.25,6.15C17.8,7.5 17.45,8.54 17.35,8.79C18,9.5 18.38,10.39 18.38,11.5C18.38,15.32 16.04,16.16 13.81,16.41C14.17,16.72 14.5,17.33 14.5,18.26C14.5,19.6 14.5,20.68 14.5,21C14.5,21.27 14.66,21.59 15.17,21.5C19.14,20.16 22,16.42 22,12A10,10 0 0,0 12,2Z"/>
                        </svg>
                    </a>
                </div>
            </div>
        </div>
    </header>
    
    <div class="container">
        <div class="section">
            <h2>GPU Status</h2>
            <div class="controls">
                <button onclick="refreshStatus()">↻ Refresh</button>
                <button onclick="toggleExpandAll()" id="expand-all-btn">⤧ Expand All</button>
                <div class="timestamp" id="status-timestamp"></div>
            </div>
            <div id="gpu-status" class="loading">Loading GPU status...</div>
        </div>

        <div class="section">
            <h2>GPU Reservation Report</h2>
            <div class="controls">
                <div class="control-group">
                    <label for="days-select">Time Period:</label>
                    <select id="days-select" onchange="refreshReport()">
                        <option value="1">Last 24 hours</option>
                        <option value="3">Last 3 days</option>
                        <option value="7">Last 7 days</option>
                        <option value="14">Last 14 days</option>
                        <option value="30" selected>Last 30 days</option>
                        <option value="60">Last 60 days</option>
                        <option value="90">Last 90 days</option>
                    </select>
                </div>
                <button onclick="refreshReport()">↻ Refresh</button>
                <div class="timestamp" id="report-timestamp"></div>
            </div>
            <div id="usage-report" class="loading">Loading reservation report...</div>
        </div>
    </div>

    <script>
        let statusRefreshInterval;
        let reportRefreshInterval;

        async function fetchStatus() {
            try {
                const response = await fetch('/api/status');
                if (!response.ok) throw new Error('Failed to fetch status');
                return await response.json();
            } catch (error) {
                console.error('Error fetching status:', error);
                throw error;
            }
        }

        async function fetchReport(days) {
            try {
                const response = await fetch('/api/report?days=' + days);
                if (!response.ok) throw new Error('Failed to fetch report');
                return await response.json();
            } catch (error) {
                console.error('Error fetching report:', error);
                throw error;
            }
        }

        function formatDuration(seconds) {
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            const secs = Math.floor(seconds % 60);
            
            if (hours > 0) {
                return hours + 'h ' + minutes + 'm ' + secs + 's';
            } else if (minutes > 0) {
                return minutes + 'm ' + secs + 's';
            } else {
                return secs + 's';
            }
        }

        function formatTimestamp(timestamp) {
            if (!timestamp) return 'never';
            const date = new Date(timestamp);
            const now = new Date();
            const diff = now - date;
            
            // Use compact time formatting
            if (diff < 60000) return 'now';
            if (diff < 3600000) return Math.floor(diff / 60000) + 'm';
            if (diff < 86400000) return Math.floor(diff / 3600000) + 'h';
            if (diff < 604800000) return Math.floor(diff / 86400000) + 'd';
            
            // For longer periods, show absolute date in compact format
            const today = new Date();
            const isThisYear = date.getFullYear() === today.getFullYear();
            
            if (isThisYear) {
                return (date.getMonth() + 1) + '/' + date.getDate();
            } else {
                return (date.getMonth() + 1) + '/' + date.getDate() + '/' + date.getFullYear().toString().slice(-2);
            }
        }

        function formatCompactTime(date) {
            const hours = date.getHours();
            const minutes = date.getMinutes();
            const seconds = date.getSeconds();
            
            // Format as HH:MM:SS in 24-hour format
            return hours.toString().padStart(2, '0') + ':' + 
                   minutes.toString().padStart(2, '0') + ':' + 
                   seconds.toString().padStart(2, '0');
        }

        function getProviderIcon(provider) {
            // Convert to lowercase for case-insensitive comparison
            const providerLower = provider ? provider.toLowerCase() : '';
            
            switch (providerLower) {
                case 'openai':
                    return '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z" fill="currentColor"/></svg>';
                case 'meta-llama':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 50 50" width="1em" xmlns="http://www.w3.org/2000/svg"><title>Meta</title><path d="M47.3,21.01c-0.58-1.6-1.3-3.16-2.24-4.66c-0.93-1.49-2.11-2.93-3.63-4.13c-1.51-1.19-3.49-2.09-5.59-2.26l-0.78-0.04	c-0.27,0.01-0.57,0.01-0.85,0.04c-0.57,0.06-1.11,0.19-1.62,0.34c-1.03,0.32-1.93,0.8-2.72,1.32c-1.42,0.94-2.55,2.03-3.57,3.15	c0.01,0.02,0.03,0.03,0.04,0.05l0.22,0.28c0.51,0.67,1.62,2.21,2.61,3.87c1.23-1.2,2.83-2.65,3.49-3.07	c0.5-0.31,0.99-0.55,1.43-0.68c0.23-0.06,0.44-0.11,0.64-0.12c0.1-0.02,0.19-0.01,0.3-0.02l0.38,0.02c0.98,0.09,1.94,0.49,2.85,1.19	c1.81,1.44,3.24,3.89,4.17,6.48c0.95,2.6,1.49,5.44,1.52,8.18c0,1.31-0.17,2.57-0.57,3.61c-0.39,1.05-1.38,1.45-2.5,1.45	c-1.63,0-2.81-0.7-3.76-1.68c-1.04-1.09-2.02-2.31-2.96-3.61c-0.78-1.09-1.54-2.22-2.26-3.37c-1.27-2.06-2.97-4.67-4.15-6.85	L25,16.35c-0.31-0.39-0.61-0.78-0.94-1.17c-1.11-1.26-2.34-2.5-3.93-3.56c-0.79-0.52-1.69-1-2.72-1.32	c-0.51-0.15-1.05-0.28-1.62-0.34c-0.18-0.02-0.36-0.03-0.54-0.03c-0.11,0-0.21-0.01-0.31-0.01l-0.78,0.04	c-2.1,0.17-4.08,1.07-5.59,2.26c-1.52,1.2-2.7,2.64-3.63,4.13C4,17.85,3.28,19.41,2.7,21.01c-1.13,3.2-1.74,6.51-1.75,9.93	c0.01,1.78,0.24,3.63,0.96,5.47c0.7,1.8,2.02,3.71,4.12,4.77c1.03,0.53,2.2,0.81,3.32,0.81c1.23,0.03,2.4-0.32,3.33-0.77	c1.87-0.93,3.16-2.16,4.33-3.4c2.31-2.51,4.02-5.23,5.6-8c0.44-0.76,0.86-1.54,1.27-2.33c-0.21-0.41-0.42-0.84-0.64-1.29	c-0.62-1.03-1.39-2.25-1.95-3.1c-0.83,1.5-1.69,2.96-2.58,4.41c-1.59,2.52-3.3,4.97-5.21,6.98c-0.95,0.98-2,1.84-2.92,2.25	c-0.47,0.2-0.83,0.27-1.14,0.25c-0.43,0-0.79-0.1-1.13-0.28c-0.67-0.35-1.3-1.1-1.69-2.15c-0.4-1.04-0.57-2.3-0.57-3.61	c0.03-2.74,0.57-5.58,1.52-8.18c0.93-2.59,2.36-5.04,4.17-6.48c0.91-0.7,1.87-1.1,2.85-1.19l0.38-0.02c0.11,0.01,0.2,0,0.3,0.02	c0.2,0.01,0.41,0.06,0.64,0.12c0.26,0.08,0.54,0.19,0.83,0.34c0.2,0.1,0.4,0.21,0.6,0.34c1,0.64,1.99,1.58,2.92,2.62	c0.72,0.81,1.41,1.71,2.1,2.63L25,25.24c0.75,1.55,1.53,3.09,2.39,4.58c1.58,2.77,3.29,5.49,5.6,8c0.68,0.73,1.41,1.45,2.27,2.1	c0.61,0.48,1.28,0.91,2.06,1.3c0.93,0.45,2.1,0.8,3.33,0.77c1.12,0,2.29-0.28,3.32-0.81c2.1-1.06,3.42-2.97,4.12-4.77	c0.72-1.84,0.95-3.69,0.96-5.47C49.04,27.52,48.43,24.21,47.3,21.01z" fill="currentColor"/></svg>';
                case 'qwen':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 24 24" width="1em" xmlns="http://www.w3.org/2000/svg"><title>Qwen</title><path d="M12.604 1.34c.393.69.784 1.382 1.174 2.075a.18.18 0 00.157.091h5.552c.174 0 .322.11.446.327l1.454 2.57c.19.337.24.478.024.837-.26.43-.513.864-.76 1.3l-.367.658c-.106.196-.223.28-.04.512l2.652 4.637c.172.301.111.494-.043.77-.437.785-.882 1.564-1.335 2.34-.159.272-.352.375-.68.37-.777-.016-1.552-.01-2.327.016a.099.099 0 00-.081.05 575.097 575.097 0 01-2.705 4.74c-.169.293-.38.363-.725.364-.997.003-2.002.004-3.017.002a.537.537 0 01-.465-.271l-1.335-2.323a.09.09 0 00-.083-.049H4.982c-.285.03-.553-.001-.805-.092l-1.603-2.77a.543.543 0 01-.002-.54l1.207-2.12a.198.198 0 000-.197 550.951 550.951 0 01-1.875-3.272l-.79-1.395c-.16-.31-.173-.496.095-.965.465-.813.927-1.625 1.387-2.436.132-.234.304-.334.584-.335a338.3 338.3 0 012.589-.001.124.124 0 00.107-.063l2.806-4.895a.488.488 0 01.422-.246c.524-.001 1.053 0 1.583-.006L11.704 1c.341-.003.724.032.9.34zm-3.432.403a.06.06 0 00-.052.03L6.254 6.788a.157.157 0 01-.135.078H3.253c-.056 0-.07.025-.041.074l5.81 10.156c.025.042.013.062-.034.063l-2.795.015a.218.218 0 00-.2.116l-1.32 2.31c-.044.078-.021.118.068.118l5.716.008c.046 0 .08.02.104.061l1.403 2.454c.046.081.092.082.139 0l5.006-8.76.783-1.382a.055.055 0 01.096 0l1.424 2.53a.122.122 0 00.107.062l2.763-.02a.04.04 0 00.035-.02.041.041 0 000-.04l-2.9-5.086a.108.108 0 010-.113l.293-.507 1.12-1.977c.024-.041.012-.062-.035-.062H9.2c-.059 0-.073-.026-.043-.077l1.434-2.505a.107.107 0 000-.114L9.225 1.774a.06.06 0 00-.053-.031zm6.29 8.02c.046 0 .058.02.034.06l-.832 1.465-2.613 4.585a.056.056 0 01-.05.029.058.058 0 01-.05-.029L8.498 9.841c-.02-.034-.01-.052.028-.054l.216-.012 6.722-.012z" fill="currentColor" fill-rule="nonzero"></path></svg>';
                case 'deepseek-ai':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 24 24" width="1em" xmlns="http://www.w3.org/2000/svg"><title>DeepSeek</title><path d="M23.748 4.482c-.254-.124-.364.113-.512.234-.051.039-.094.09-.137.136-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.156-.708-.311-.955-.65-.172-.241-.219-.51-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.093.172.187.129.323-.082.28-.18.552-.266.833-.055.179-.137.217-.329.14a5.526 5.526 0 01-1.736-1.18c-.857-.828-1.631-1.742-2.597-2.458a11.365 11.365 0 00-.689-.471c-.985-.957.13-1.743.388-1.836.27-.098.093-.432-.779-.428-.872.004-1.67.295-2.687.684a3.055 3.055 0 01-.465.137 9.597 9.597 0 00-2.883-.102c-1.885.21-3.39 1.102-4.497 2.623C.082 8.606-.231 10.684.152 12.85c.403 2.284 1.569 4.175 3.36 5.653 1.858 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.133-.284 4.994-1.86.47.234.962.327 1.78.397.63.059 1.236-.03 1.705-.128.735-.156.684-.837.419-.961-2.155-1.004-1.682-.595-2.113-.926 1.096-1.296 2.746-2.642 3.392-7.003.05-.347.007-.565 0-.845-.004-.17.035-.237.23-.256a4.173 4.173 0 001.545-.475c1.396-.763 1.96-2.015 2.093-3.517.02-.23-.004-.467-.247-.588zM11.581 18c-2.089-1.642-3.102-2.183-3.52-2.16-.392.024-.321.471-.235.763.09.288.207.486.371.739.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.167-1.361-.802-2.5-1.86-3.301-3.307-.774-1.393-1.224-2.887-1.298-4.482-.02-.386.093-.522.477-.592a4.696 4.696 0 011.529-.039c2.132.312 3.946 1.265 5.468 2.774.868.86 1.525 1.887 2.202 2.891.72 1.066 1.494 2.082 2.48 2.914.348.292.625.514.891.677-.802.09-2.14.11-3.054-.614zm1-6.44a.306.306 0 01.415-.287.302.302 0 01.2.288.306.306 0 01-.31.307.303.303 0 01-.304-.308zm3.11 1.596c-.2.081-.399.151-.59.16a1.245 1.245 0 01-.798-.254c-.274-.23-.47-.358-.552-.758a1.73 1.73 0 01.016-.588c.07-.327-.008-.537-.239-.727-.187-.156-.426-.199-.688-.199a.559.559 0 01-.254-.078c-.11-.054-.2-.19-.114-.358.028-.054.16-.186.192-.21.356-.202.767-.136 1.146.016.352.144.618.408 1.001.782.391.451.462.576.685.914.176.265.336.537.445.848.067.195-.019.354-.25.452z" fill="currentColor"></path></svg>';
                case 'redhatai':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 32 32" width="1em" xmlns="http://www.w3.org/2000/svg"><title>Red Hat</title><path d="M26.135 15.933c0.136 0.467 0.233 1.011 0.271 1.572l0.001 0.024c0 2.206-2.479 3.43-5.74 3.43-7.367 0.005-13.821-4.313-13.821-7.165 0-0.002 0-0.004 0-0.005 0-0.416 0.087-0.811 0.245-1.169l-0.007 0.019c-2.648 0.132-6.080 0.606-6.080 3.634 0 4.96 11.753 11.073 21.058 11.073 7.135 0 8.934-3.227 8.934-5.773 0-2.006-1.733-4.28-4.857-5.638zM21.010 17.732c1.971 0 4.824-0.407 4.824-2.752 0.001-0.020 0.001-0.043 0.001-0.067 0-0.167-0.019-0.33-0.054-0.486l0.003 0.015-1.175-5.099c-0.27-1.122-0.507-1.631-2.477-2.615-1.684-0.889-3.637-1.604-5.692-2.045l-0.151-0.027c-0.916 0-1.183 1.182-2.277 1.182-1.052 0-1.833-0.882-2.818-0.882-0.946 0-1.562 0.644-2.037 1.969 0 0-1.325 3.736-1.496 4.279-0.023 0.080-0.036 0.172-0.036 0.267 0 0.014 0 0.028 0.001 0.042l-0-0.002c0 1.452 5.72 6.216 13.384 6.216z" fill="currentColor"></path></svg>';
                case 'ibm-granite':
                case 'ibm-research':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 24 24" width="1em" xmlns="http://www.w3.org/2000/svg"><title>IBM</title><path d="M23.544 15.993c.038 0 .06-.017.06-.053v-.036c0-.035-.022-.052-.06-.052h-.09v.14zm-.09.262h-.121v-.498h.225c.112 0 .169.066.169.157 0 .079-.036.129-.09.15l.111.19h-.133l-.092-.17h-.07zm.434-.222v-.062c0-.2-.157-.357-.363-.357a.355.355 0 0 0-.363.357v.062c0 .2.156.358.363.358a.355.355 0 0 0 .363-.358zm-.838-.03c0-.28.212-.492.475-.492.264 0 .475.213.475.491a.477.477 0 0 1-.475.491.477.477 0 0 1-.475-.49zM16.21 8.13l-.216-.624h-3.56v.624zm.413 1.19-.216-.623h-3.973v.624zm2.65 7.147h3.107v-.624h-3.108zm0-1.192h3.107v-.623h-3.108zm0-1.19h1.864v-.624h-1.865zm0-1.191h1.864v-.624h-1.865zm0-1.191h1.864v-.624h-3.555l-.175.504-.175-.504h-3.555v.624h1.865v-.574l.2.574h3.33l.2-.574zm1.864-1.815h-3.142l-.217.624h3.359zm-7.46 3.006h1.865v-.624h-1.865zm0 1.19h1.865v-.623h-1.865zm-1.243 1.191h3.108v-.623h-3.108zm0 1.192h3.108v-.624h-3.108zm6.386-8.961-.216.624h3.776v-.624zm-.629 1.815h4.19v-.624h-3.974zm-4.514 1.19h3.359l-.216-.623h-3.143zm2.482 2.383h2.496l.218-.624h-2.932zm.417 1.19h1.662l.218-.623h-2.098zm.416 1.191h.83l.218-.623h-1.266zm.414 1.192.217-.624h-.432zm-12.433-.006 4.578.006c.622 0 1.18-.237 1.602-.624h-6.18zm4.86-3v.624h2.092c0-.216-.03-.425-.083-.624zm-3.616.624h1.865v-.624H6.217zm3.617-3.573h2.008c.053-.199.083-.408.083-.624H9.834zm-3.617 0h1.865v-.624H6.217zM9.55 7.507H4.973v.624h6.18a2.36 2.36 0 0 0-1.602-.624zm2.056 1.191H4.973v.624h6.884a2.382 2.382 0 0 0-.25-.624zm-5.39 2.382v.624h4.87c.207-.176.382-.387.519-.624zm4.87 1.191h-4.87v.624h5.389a2.39 2.39 0 0 0-.519-.624zm-6.114 3.006h6.634c.11-.193.196-.402.25-.624H4.973zM0 8.13h4.352v-.624H0zm0 1.191h4.352v-.624H0zm1.243 1.191h1.865v-.624H1.243zm0 1.191h1.865v-.624H1.243zm0 1.19h1.865v-.623H1.243zm0 1.192h1.865v-.624H1.243zM0 15.276h4.352v-.623H0zm0 1.192h4.352v-.624H0z" fill="currentColor"/></svg>';
                case 'google':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 210 210" width="1em" xmlns="http://www.w3.org/2000/svg"><title>Google</title><path d="M0,105C0,47.103,47.103,0,105,0c23.383,0,45.515,7.523,64.004,21.756l-24.4,31.696C133.172,44.652,119.477,40,105,40 c-35.841,0-65,29.159-65,65s29.159,65,65,65c28.867,0,53.398-18.913,61.852-45H105V85h105v20c0,57.897-47.103,105-105,105 S0,162.897,0,105z" fill="currentColor"/></svg>';
                case 'mistralai':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 129 91" width="1em" xmlns="http://www.w3.org/2000/svg"><title>Mistral AI</title><g fill="currentColor"><rect x="18.292" y="0" width="18.293" height="18.123"/><rect x="91.473" y="0" width="18.293" height="18.123"/><rect x="18.292" y="18.121" width="36.586" height="18.123"/><rect x="73.181" y="18.121" width="36.586" height="18.123"/><rect x="18.292" y="36.243" width="91.476" height="18.122"/><rect x="18.292" y="54.37" width="18.293" height="18.123"/><rect x="54.883" y="54.37" width="18.293" height="18.123"/><rect x="91.473" y="54.37" width="18.293" height="18.123"/><rect x="0" y="72.504" width="54.89" height="18.123"/><rect x="73.181" y="72.504" width="54.89" height="18.123"/></g></svg>';
                default:
                    return '';
            }
        }

        function renderStatus(data) {
            const container = document.getElementById('gpu-status');
            
            if (!data || data.length === 0) {
                container.innerHTML = '<div class="error">No GPU data available</div>';
                return;
            }

            // Save current expanded state before re-rendering
            const expandedStates = {};
            const existingCards = container.querySelectorAll('.gpu-card');
            existingCards.forEach(card => {
                const gpuId = card.getAttribute('data-gpu-id');
                if (gpuId) {
                    expandedStates[gpuId] = card.classList.contains('expanded');
                }
            });

            let html = '<div class="gpu-grid">';
            
            data.forEach(gpu => {
                const statusClass = gpu.status.toLowerCase().replace('_', '-');
                let statusText = gpu.status.replace('_', ' ');
                
                // Generate summary text for collapsed view
                let summary = '';
                if (gpu.user) {
                    summary = gpu.user;
                    if (gpu.reservation_type === 'manual' && gpu.expiry_time) {
                        const expiresIn = new Date(gpu.expiry_time) - new Date();
                        if (expiresIn > 0) {
                            summary += ', expires in ' + formatDuration(expiresIn / 1000);
                        }
                    } else if (gpu.duration) {
                        summary += ', ' + formatDuration(gpu.duration / 1000000000);
                    }
                } else if (gpu.unreserved_users && gpu.unreserved_users.length > 0) {
                    summary = 'Used by ' + gpu.unreserved_users.join(', ');
                } else if (gpu.last_released) {
                    summary = 'Last released ' + formatTimestamp(gpu.last_released);
                }
                
                // Check if this GPU was previously expanded
                const isExpanded = expandedStates[gpu.gpu_id] || false;
                const expandedClass = isExpanded ? ' expanded' : '';
                
                html += '<div class="gpu-card' + expandedClass + '" data-gpu-id="' + gpu.gpu_id + '" onclick="toggleCard(this)">';
                html += '<div class="gpu-header">';
                html += '<div class="gpu-header-left">';
                html += '<svg class="expand-icon" viewBox="0 0 24 24">';
                html += '<path d="M8.59,16.58L13.17,12L8.59,7.41L10,6L16,12L10,18L8.59,16.58Z"/>';
                html += '</svg>';
                
                // Add model icon if available
                if (gpu.model_info && gpu.model_info.provider) {
                    html += '<div class="model-icon" title="' + gpu.model_info.model + '">';
                    html += getProviderIcon(gpu.model_info.provider);
                    html += '</div>';
                }
                html += '</div>';
                
                html += '<div class="gpu-header-center">';
                html += '<div class="gpu-id">GPU ' + gpu.gpu_id + '</div>';
                
                if (summary) {
                    html += '<div class="gpu-summary">' + summary + '</div>';
                }
                html += '</div>';
                
                html += '<span class="status-badge status-' + statusClass + '">' + statusText + '</span>';
                html += '</div>';
                html += '<div class="gpu-details">';
                
                // Add GPU provider and model information at the top of details
                if (gpu.provider) {
                    if (gpu.gpu_model) {
                        html += '<div class="gpu-hardware-info"><strong>Hardware:</strong> ' + gpu.provider + ' ' + gpu.gpu_model + '</div>';
                    } else {
                        html += '<div class="gpu-hardware-info"><strong>Hardware:</strong> ' + gpu.provider + '</div>';
                    }
                }
                
                if (gpu.user) {
                    html += '<div><strong>User:</strong> ' + gpu.user + '</div>';
                    html += '<div><strong>Type:</strong> ' + gpu.reservation_type + '</div>';
                    html += '<div><strong>Duration:</strong> ' + formatDuration(gpu.duration / 1000000000) + '</div>';
                    
                    if (gpu.reservation_type === 'run' && gpu.last_heartbeat) {
                        html += '<div><strong>Last heartbeat:</strong> ' + formatTimestamp(gpu.last_heartbeat) + '</div>';
                    }
                    
                    if (gpu.reservation_type === 'manual' && gpu.expiry_time) {
                        const expiresIn = new Date(gpu.expiry_time) - new Date();
                        if (expiresIn > 0) {
                            html += '<div><strong>Expires in:</strong> ' + formatDuration(expiresIn / 1000) + '</div>';
                        }
                    }
                }
                
                if (gpu.validation_info) {
                    html += '<div><strong>Validation:</strong> ' + gpu.validation_info + '</div>';
                    
                    // Add memory usage bar if validation info contains memory data
                    const memoryMatch = gpu.validation_info.match(/(\d+)MB/);
                    if (memoryMatch) {
                        const memoryMB = parseInt(memoryMatch[1]);
                        const maxMemoryMB = 80000; // Rough estimate for H100
                        const percentage = Math.min((memoryMB / maxMemoryMB) * 100, 100);
                        let memoryClass = 'memory-low';
                        if (percentage > 70) memoryClass = 'memory-high';
                        else if (percentage > 40) memoryClass = 'memory-medium';
                        
                        html += '<div class="memory-usage">';
                        html += '<div class="memory-bar">';
                        html += '<div class="memory-fill ' + memoryClass + '" style="width: ' + percentage + '%"></div>';
                        html += '</div>';
                        html += '<div class="memory-text">' + percentage.toFixed(1) + '%</div>';
                        html += '</div>';
                    }
                }
                
                // Show 0% memory usage for GPUs that are in use but have no detected usage
                if (gpu.status === 'IN_USE' && (!gpu.validation_info || 
                    (gpu.validation_info && gpu.validation_info.includes('no usage detected')))) {
                    html += '<div class="memory-usage">';
                    html += '<div class="memory-bar">';
                    html += '<div class="memory-fill memory-low" style="width: 0%"></div>';
                    html += '</div>';
                    html += '<div class="memory-text">0.0%</div>';
                    html += '</div>';
                }
                
                if (gpu.model_info && gpu.model_info.model) {
                    html += '<div><strong>Model:</strong> ' + gpu.model_info.model + '</div>';
                }
                
                if (gpu.unreserved_users && gpu.unreserved_users.length > 0) {
                    html += '<div><strong>Unreserved users:</strong> ' + gpu.unreserved_users.join(', ') + '</div>';
                }
                
                if (gpu.process_info) {
                    html += '<div><strong>Processes:</strong> ' + gpu.process_info + '</div>';
                }
                
                if (gpu.status === 'AVAILABLE' && gpu.last_released) {
                    html += '<div><strong>Last released:</strong> ' + formatTimestamp(gpu.last_released) + '</div>';
                }
                
                html += '</div></div>';
            });
            
            html += '</div>';
            container.innerHTML = html;
            
            document.getElementById('status-timestamp').textContent = 'Last updated: ' + formatCompactTime(new Date());
        }

        function renderReport(data) {
            const container = document.getElementById('usage-report');
            
            if (!data || !data.users || data.users.length === 0) {
                container.innerHTML = '<div>No reservation data available for this period</div>';
                return;
            }

            let html = '<table class="usage-table">';
            html += '<thead><tr>';
            html += '<th>User</th>';
            html += '<th>GPU Hours</th>';
            html += '<th>Percentage</th>';
            html += '<th>Run</th>';
            html += '<th>Manual</th>';
            html += '</tr></thead>';
            html += '<tbody>';
            
            const maxHours = Math.max(...data.users.map(u => u.gpu_hours));
            
            data.users.forEach(user => {
                const barWidth = (user.gpu_hours / maxHours) * 200;
                
                html += '<tr>';
                html += '<td>' + user.name + '</td>';
                html += '<td>';
                html += '<span class="usage-bar" style="width: ' + barWidth + 'px"></span>';
                html += user.gpu_hours.toFixed(2);
                html += '</td>';
                html += '<td>' + user.percentage.toFixed(1) + '%</td>';
                html += '<td>' + user.run_count + '</td>';
                html += '<td>' + user.manual_count + '</td>';
                html += '</tr>';
            });
            
            html += '</tbody>';
            html += '<tfoot><tr>';
            html += '<td><strong>TOTAL</strong></td>';
            html += '<td><strong>' + data.total_gpu_hours.toFixed(2) + '</strong></td>';
            html += '<td><strong>100.0%</strong></td>';
            html += '<td><strong>' + data.total_reservations + '</strong></td>';
            html += '<td><strong>-</strong></td>';
            html += '</tr></tfoot>';
            html += '</table>';
            
            html += '<div style="margin-top: 20px; color: #888;">';
            html += 'Total reservations: ' + data.total_reservations + '<br>';
            html += 'Unique users: ' + data.unique_users + '<br>';
            html += 'Period: ' + data.start_date + ' to ' + data.end_date;
            html += '</div>';
            
            container.innerHTML = html;
            
            document.getElementById('report-timestamp').textContent = 'Last updated: ' + formatCompactTime(new Date());
        }

        async function refreshStatus() {
            const container = document.getElementById('gpu-status');
            container.classList.add('refreshing');
            
            try {
                const data = await fetchStatus();
                renderStatus(data);
            } catch (error) {
                container.innerHTML = '<div class="error">Failed to load GPU status: ' + error.message + '</div>';
            } finally {
                container.classList.remove('refreshing');
            }
        }

        async function refreshReport() {
            const container = document.getElementById('usage-report');
            const days = document.getElementById('days-select').value;
            container.classList.add('refreshing');
            
            try {
                const data = await fetchReport(days);
                renderReport(data);
            } catch (error) {
                container.innerHTML = '<div class="error">Failed to load reservation report: ' + error.message + '</div>';
            } finally {
                container.classList.remove('refreshing');
            }
        }

        // Initial load
        refreshStatus();
        refreshReport();

        // Auto-refresh status every 30 seconds
        statusRefreshInterval = setInterval(refreshStatus, 30000);

        // Auto-refresh report every 5 minutes
        reportRefreshInterval = setInterval(refreshReport, 300000);

        // Clean up intervals when page is hidden
        document.addEventListener('visibilitychange', () => {
            if (document.hidden) {
                clearInterval(statusRefreshInterval);
                clearInterval(reportRefreshInterval);
            } else {
                refreshStatus();
                refreshReport();
                statusRefreshInterval = setInterval(refreshStatus, 30000);
                reportRefreshInterval = setInterval(refreshReport, 300000);
            }
        });

        // Toggle individual GPU card
        function toggleCard(card) {
            card.classList.toggle('expanded');
        }

        // Toggle all GPU cards
        function toggleExpandAll() {
            const cards = document.querySelectorAll('.gpu-card');
            const button = document.getElementById('expand-all-btn');
            const allExpanded = Array.from(cards).every(card => card.classList.contains('expanded'));
            
            if (allExpanded) {
                // Collapse all
                cards.forEach(card => card.classList.remove('expanded'));
                button.textContent = '⤧ Expand All';
            } else {
                // Expand all
                cards.forEach(card => card.classList.add('expanded'));
                button.textContent = '⤴ Collapse All';
            }
        }

        // Theme toggle functionality
        function toggleTheme() {
            const body = document.body;
            const themeIcon = document.getElementById('theme-icon');
            const currentTheme = body.getAttribute('data-theme');
            
            if (currentTheme === 'light') {
                body.setAttribute('data-theme', 'dark');
                // Moon icon for dark mode
                themeIcon.querySelector('path').setAttribute('d', 'M17.75,4.09L15.22,6.03L16.13,9.09L13.5,7.28L10.87,9.09L11.78,6.03L9.25,4.09L12.44,4L13.5,1L14.56,4L17.75,4.09M21.25,11L19.61,12.25L20.2,14.23L18.5,13.06L16.8,14.23L17.39,12.25L15.75,11L17.81,10.95L18.5,9L19.19,10.95L21.25,11M18.97,15.95C19.8,15.87 20.69,17.05 20.16,17.8C19.84,18.25 19.5,18.67 19.08,19.07C15.17,23 8.84,23 4.94,19.07C1.03,15.17 1.03,8.83 4.94,4.93C5.34,4.53 5.76,4.17 6.21,3.85C6.96,3.32 8.14,4.21 8.06,5.04C7.79,7.9 8.75,10.87 10.95,13.06C13.14,15.26 16.1,16.22 18.97,15.95M17.33,17.97C14.5,17.81 11.7,16.64 9.53,14.5C7.36,12.31 6.2,9.5 6.04,6.68C3.23,9.82 3.34,14.4 6.35,17.41C9.37,20.43 14,20.54 17.33,17.97Z');
                localStorage.setItem('theme', 'dark');
            } else {
                body.setAttribute('data-theme', 'light');
                // Sun icon for light mode
                themeIcon.querySelector('path').setAttribute('d', 'M12,8A4,4 0 0,0 8,12A4,4 0 0,0 12,16A4,4 0 0,0 16,12A4,4 0 0,0 12,8M12,18A6,6 0 0,1 6,12A6,6 0 0,1 12,6A6,6 0 0,1 18,12A6,6 0 0,1 12,18M20,8.69V4H15.31L12,0.69L8.69,4H4V8.69L0.69,12L4,15.31V20H8.69L12,23.31L15.31,20H20V15.31L23.31,12L20,8.69Z');
                localStorage.setItem('theme', 'light');
            }
        }

        // Initialize theme from localStorage
        function initTheme() {
            const savedTheme = localStorage.getItem('theme');
            const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
            const theme = savedTheme || (prefersDark ? 'dark' : 'light');
            
            document.body.setAttribute('data-theme', theme);
            const themeIcon = document.getElementById('theme-icon');
            if (theme === 'light') {
                // Sun icon for light mode
                themeIcon.querySelector('path').setAttribute('d', 'M12,8A4,4 0 0,0 8,12A4,4 0 0,0 12,16A4,4 0 0,0 16,12A4,4 0 0,0 12,8M12,18A6,6 0 0,1 6,12A6,6 0 0,1 12,6A6,6 0 0,1 18,12A6,6 0 0,1 12,18M20,8.69V4H15.31L12,0.69L8.69,4H4V8.69L0.69,12L4,15.31V20H8.69L12,23.31L15.31,20H20V15.31L23.31,12L20,8.69Z');
            } else {
                // Moon icon for dark mode
                themeIcon.querySelector('path').setAttribute('d', 'M17.75,4.09L15.22,6.03L16.13,9.09L13.5,7.28L10.87,9.09L11.78,6.03L9.25,4.09L12.44,4L13.5,1L14.56,4L17.75,4.09M21.25,11L19.61,12.25L20.2,14.23L18.5,13.06L16.8,14.23L17.39,12.25L15.75,11L17.81,10.95L18.5,9L19.19,10.95L21.25,11M18.97,15.95C19.8,15.87 20.69,17.05 20.16,17.8C19.84,18.25 19.5,18.67 19.08,19.07C15.17,23 8.84,23 4.94,19.07C1.03,15.17 1.03,8.83 4.94,4.93C5.34,4.53 5.76,4.17 6.21,3.85C6.96,3.32 8.14,4.21 8.06,5.04C7.79,7.9 8.75,10.87 10.95,13.06C13.14,15.26 16.1,16.22 18.97,15.95M17.33,17.97C14.5,17.81 11.7,16.64 9.53,14.5C7.36,12.31 6.2,9.5 6.04,6.68C3.23,9.82 3.34,14.4 6.35,17.41C9.37,20.43 14,20.54 17.33,17.97Z');
            }
        }

        // Progressive enhancement: Skeleton loading states
        function showSkeletonLoader() {
            return '<div class="gpu-grid">' + 
                   Array(8).fill('<div class="gpu-card skeleton-pulse"><div style="height:60px;"></div></div>').join('') +
                   '</div>';
        }

        // Progressive enhancement: Graceful error recovery
        function showRetryableError(message, retryCallback) {
            return '<div class="error">' + message + 
                   '<br><button onclick="' + retryCallback + '" style="margin-top:10px;background:var(--accent-color);border:none;">↻ Retry</button>' +
                   '</div>';
        }

        // Enhanced refresh with skeleton states
        async function refreshStatusWithSkeleton() {
            const container = document.getElementById('gpu-status');
            container.innerHTML = showSkeletonLoader();
            
            try {
                const data = await fetchStatus();
                renderStatus(data);
            } catch (error) {
                container.innerHTML = showRetryableError('Failed to load GPU status: ' + error.message, 'refreshStatus()');
            }
        }

        // Initialize theme on page load
        initTheme();

        // Listen for system theme changes
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            if (!localStorage.getItem('theme')) {
                document.body.setAttribute('data-theme', e.matches ? 'dark' : 'light');
                const themeIcon = document.getElementById('theme-icon');
                if (e.matches) {
                    // Dark mode - moon icon
                    themeIcon.querySelector('path').setAttribute('d', 'M17.75,4.09L15.22,6.03L16.13,9.09L13.5,7.28L10.87,9.09L11.78,6.03L9.25,4.09L12.44,4L13.5,1L14.56,4L17.75,4.09M21.25,11L19.61,12.25L20.2,14.23L18.5,13.06L16.8,14.23L17.39,12.25L15.75,11L17.81,10.95L18.5,9L19.19,10.95L21.25,11M18.97,15.95C19.8,15.87 20.69,17.05 20.16,17.8C19.84,18.25 19.5,18.67 19.08,19.07C15.17,23 8.84,23 4.94,19.07C1.03,15.17 1.03,8.83 4.94,4.93C5.34,4.53 5.76,4.17 6.21,3.85C6.96,3.32 8.14,4.21 8.06,5.04C7.79,7.9 8.75,10.87 10.95,13.06C13.14,15.26 16.1,16.22 18.97,15.95M17.33,17.97C14.5,17.81 11.7,16.64 9.53,14.5C7.36,12.31 6.2,9.5 6.04,6.68C3.23,9.82 3.34,14.4 6.35,17.41C9.37,20.43 14,20.54 17.33,17.97Z');
                } else {
                    // Light mode - sun icon
                    themeIcon.querySelector('path').setAttribute('d', 'M12,8A4,4 0 0,0 8,12A4,4 0 0,0 12,16A4,4 0 0,0 16,12A4,4 0 0,0 12,8M12,18A6,6 0 0,1 6,12A6,6 0 0,1 12,6A6,6 0 0,1 18,12A6,6 0 0,1 12,18M20,8.69V4H15.31L12,0.69L8.69,4H4V8.69L0.69,12L4,15.31V20H8.69L12,23.31L15.31,20H20V15.31L23.31,12L20,8.69Z');
                }
            }
        });
    </script>
</body>
</html>`

	t, err := template.New("index").Parse(tmpl)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := t.Execute(w, struct {
		Hostname string
		Demo     bool
	}{
		Hostname: hostname,
		Demo:     ws.demo,
	}); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

func (ws *webServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var statuses []gpu.GPUStatusInfo
	var err error

	if ws.demo {
		// Use demo data
		statuses = ws.generateDemoStatus()
	} else {
		// Clean up expired reservations first
		if err := ws.engine.CleanupExpiredReservations(ctx); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: Failed to cleanup expired reservations: %v\n", err)
		}

		statuses, err = ws.engine.GetGPUStatus(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get GPU status: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Convert to JSON-friendly format
	type jsonGPUStatus struct {
		GPUID           int            `json:"gpu_id"`
		Status          string         `json:"status"`
		User            string         `json:"user,omitempty"`
		ReservationType string         `json:"reservation_type,omitempty"`
		Duration        int64          `json:"duration,omitempty"`
		LastHeartbeat   *time.Time     `json:"last_heartbeat,omitempty"`
		ExpiryTime      *time.Time     `json:"expiry_time,omitempty"`
		LastReleased    *time.Time     `json:"last_released,omitempty"`
		ValidationInfo  string         `json:"validation_info,omitempty"`
		UnreservedUsers []string       `json:"unreserved_users,omitempty"`
		ProcessInfo     string         `json:"process_info,omitempty"`
		Error           string         `json:"error,omitempty"`
		ModelInfo       *gpu.ModelInfo `json:"model_info,omitempty"`
		Provider        string         `json:"provider,omitempty"`
		GPUModel        string         `json:"gpu_model,omitempty"`
	}

	jsonStatuses := make([]jsonGPUStatus, len(statuses))
	for i, status := range statuses {
		js := jsonGPUStatus{
			GPUID:           status.GPUID,
			Status:          status.Status,
			User:            status.User,
			ReservationType: status.ReservationType,
			Duration:        int64(status.Duration),
			ValidationInfo:  status.ValidationInfo,
			UnreservedUsers: status.UnreservedUsers,
			ProcessInfo:     status.ProcessInfo,
			Error:           status.Error,
			ModelInfo:       status.ModelInfo,
			Provider:        status.Provider,
			GPUModel:        status.GPUModel,
		}

		if !status.LastHeartbeat.IsZero() {
			js.LastHeartbeat = &status.LastHeartbeat
		}
		if !status.ExpiryTime.IsZero() {
			js.ExpiryTime = &status.ExpiryTime
		}
		if !status.LastReleased.IsZero() {
			js.LastReleased = &status.LastReleased
		}

		jsonStatuses[i] = js
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jsonStatuses); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
}

func (ws *webServer) handleAPIReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse days parameter
	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	var reportData reportData

	if ws.demo {
		// Use demo data
		reportData = ws.generateDemoReport(days)
	} else {
		// Calculate time range
		endTime := time.Now()
		startTime := endTime.AddDate(0, 0, -days)

		// Get historical usage data
		historicalRecords, err := ws.client.GetUsageHistory(ctx, startTime, endTime)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get usage history: %v", err), http.StatusInternalServerError)
			return
		}

		// Get current GPU states for in-progress usage
		currentStatuses, err := ws.engine.GetGPUStatus(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get current GPU status: %v", err), http.StatusInternalServerError)
			return
		}

		// Add current usage to records
		currentRecords := getCurrentUsageRecordsWeb(currentStatuses, endTime)
		allRecords := append(historicalRecords, currentRecords...)

		// Generate report data
		reportData = generateReportData(allRecords, startTime, endTime, days)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reportData); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
}

type reportData struct {
	Users             []userReport `json:"users"`
	TotalGPUHours     float64      `json:"total_gpu_hours"`
	TotalReservations int          `json:"total_reservations"`
	UniqueUsers       int          `json:"unique_users"`
	StartDate         string       `json:"start_date"`
	EndDate           string       `json:"end_date"`
	Days              int          `json:"days"`
}

type userReport struct {
	Name        string  `json:"name"`
	GPUHours    float64 `json:"gpu_hours"`
	Percentage  float64 `json:"percentage"`
	RunCount    int     `json:"run_count"`
	ManualCount int     `json:"manual_count"`
}

func generateReportData(records []*types.UsageRecord, startTime, endTime time.Time, days int) reportData {
	// Aggregate usage by user
	userUsage := make(map[string]float64)
	userRunCount := make(map[string]int)
	userManualCount := make(map[string]int)

	var totalDuration float64

	for _, record := range records {
		userUsage[record.User] += record.Duration
		totalDuration += record.Duration

		if record.ReservationType == types.ReservationTypeRun {
			userRunCount[record.User]++
		} else {
			userManualCount[record.User]++
		}
	}

	// Create sorted user list
	var users []userReport
	for user, duration := range userUsage {
		users = append(users, userReport{
			Name:        user,
			GPUHours:    duration / 3600.0,
			Percentage:  (duration / totalDuration) * 100,
			RunCount:    userRunCount[user],
			ManualCount: userManualCount[user],
		})
	}

	// Sort by GPU hours descending
	for i := 0; i < len(users); i++ {
		for j := i + 1; j < len(users); j++ {
			if users[j].GPUHours > users[i].GPUHours {
				users[i], users[j] = users[j], users[i]
			}
		}
	}

	return reportData{
		Users:             users,
		TotalGPUHours:     totalDuration / 3600.0,
		TotalReservations: len(records),
		UniqueUsers:       len(userUsage),
		StartDate:         startTime.Format("2006-01-02"),
		EndDate:           endTime.Format("2006-01-02"),
		Days:              days,
	}
}

func getCurrentUsageRecordsWeb(statuses []gpu.GPUStatusInfo, endTime time.Time) []*types.UsageRecord {
	var records []*types.UsageRecord

	for _, status := range statuses {
		if status.User != "" {
			duration := status.Duration.Seconds()
			startTime := endTime.Add(-status.Duration)
			records = append(records, &types.UsageRecord{
				User:            status.User,
				GPUID:           status.GPUID,
				StartTime:       types.FlexibleTime{Time: startTime},
				EndTime:         types.FlexibleTime{Time: endTime},
				Duration:        duration,
				ReservationType: status.ReservationType,
			})
		}
	}

	return records
}

// Demo mode data generation
func (ws *webServer) generateDemoStatus() []gpu.GPUStatusInfo {
	now := time.Now()
	statuses := make([]gpu.GPUStatusInfo, 8)

	// GPU 0: Available, free for 2 hours
	statuses[0] = gpu.GPUStatusInfo{
		GPUID:          0,
		Status:         "AVAILABLE",
		LastReleased:   now.Add(-2 * time.Hour),
		ValidationInfo: "[validated: 45MB used]",
		Provider:       "NVIDIA",
		GPUModel:       "H100",
	}

	// GPU 1: alice running meta-llama/Llama-3.1-8B-Instruct for 1h 15m
	statuses[1] = gpu.GPUStatusInfo{
		GPUID:           1,
		Status:          "IN_USE",
		User:            "alice",
		ReservationType: types.ReservationTypeRun,
		LastHeartbeat:   now.Add(-30 * time.Second),
		Duration:        75 * time.Minute,
		ValidationInfo:  "[validated: 8452MB, 2 processes]",
		Provider:        "NVIDIA",
		GPUModel:        "H100",
		ModelInfo: &gpu.ModelInfo{
			Model:    "meta-llama/Llama-3.1-8B-Instruct",
			Provider: "meta-llama",
		},
	}

	// GPU 2: bob running deepseek-ai/deepseek-v2 (part 1 of 2)
	statuses[2] = gpu.GPUStatusInfo{
		GPUID:           2,
		Status:          "IN_USE",
		User:            "bob",
		ReservationType: types.ReservationTypeRun,
		LastHeartbeat:   now.Add(-15 * time.Second),
		Duration:        45 * time.Minute,
		ValidationInfo:  "[validated: 15234MB, 1 processes]",
		Provider:        "NVIDIA",
		GPUModel:        "A100",
		ModelInfo: &gpu.ModelInfo{
			Model:    "deepseek-ai/deepseek-v2",
			Provider: "deepseek-ai",
		},
	}

	// GPU 3: bob running deepseek-ai/deepseek-v2 (part 2 of 2)
	statuses[3] = gpu.GPUStatusInfo{
		GPUID:           3,
		Status:          "IN_USE",
		User:            "bob",
		ReservationType: types.ReservationTypeRun,
		LastHeartbeat:   now.Add(-15 * time.Second),
		Duration:        45 * time.Minute,
		ValidationInfo:  "[validated: 15234MB, 1 processes]",
		Provider:        "NVIDIA",
		GPUModel:        "A100",
		ModelInfo: &gpu.ModelInfo{
			Model:    "deepseek-ai/deepseek-v2",
			Provider: "deepseek-ai",
		},
	}

	// GPU 4: charlie running qwen/Qwen2.5-72B-Instruct manually
	statuses[4] = gpu.GPUStatusInfo{
		GPUID:           4,
		Status:          "IN_USE",
		User:            "charlie",
		ReservationType: types.ReservationTypeManual,
		ExpiryTime:      now.Add(6 * time.Hour),
		Duration:        2 * time.Hour,
		ValidationInfo:  "[validated: 23045MB, 1 processes]",
		Provider:        "NVIDIA",
		GPUModel:        "RTX 4090",
		ModelInfo: &gpu.ModelInfo{
			Model:    "qwen/Qwen2.5-72B-Instruct",
			Provider: "qwen",
		},
	}

	// GPU 5: david running mistralai/Mistral-Large-2
	statuses[5] = gpu.GPUStatusInfo{
		GPUID:           5,
		Status:          "IN_USE",
		User:            "david",
		ReservationType: types.ReservationTypeRun,
		LastHeartbeat:   now,
		Duration:        30 * time.Minute,
		ValidationInfo:  "[validated: 19532MB, 1 processes]",
		Provider:        "NVIDIA",
		GPUModel:        "RTX 4090",
		ModelInfo: &gpu.ModelInfo{
			Model:    "mistralai/Mistral-Large-2",
			Provider: "mistralai",
		},
	}

	// GPU 6: eve running redhatai/granite-20b-multilingual
	statuses[6] = gpu.GPUStatusInfo{
		GPUID:           6,
		Status:          "IN_USE",
		User:            "eve",
		ReservationType: types.ReservationTypeRun,
		LastHeartbeat:   now.Add(-45 * time.Second),
		Duration:        90 * time.Minute,
		ValidationInfo:  "[validated: 12856MB, 2 processes]",
		Provider:        "AMD",
		GPUModel:        "",
		ModelInfo: &gpu.ModelInfo{
			Model:    "redhatai/granite-20b-multilingual",
			Provider: "redhatai",
		},
	}

	// GPU 7: Available, never used
	statuses[7] = gpu.GPUStatusInfo{
		GPUID:          7,
		Status:         "AVAILABLE",
		ValidationInfo: "[validated: 0MB used]",
		Provider:       "AMD",
		GPUModel:       "",
	}

	return statuses
}

func (ws *webServer) generateDemoReport(days int) reportData {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -days)

	// Generate some realistic usage data
	users := []userReport{
		{
			Name:        "alice",
			GPUHours:    324.50,
			Percentage:  28.5,
			RunCount:    156,
			ManualCount: 12,
		},
		{
			Name:        "bob",
			GPUHours:    245.25,
			Percentage:  21.5,
			RunCount:    98,
			ManualCount: 45,
		},
		{
			Name:        "charlie",
			GPUHours:    186.75,
			Percentage:  16.4,
			RunCount:    67,
			ManualCount: 89,
		},
		{
			Name:        "david",
			GPUHours:    172.00,
			Percentage:  15.1,
			RunCount:    145,
			ManualCount: 5,
		},
		{
			Name:        "eve",
			GPUHours:    134.50,
			Percentage:  11.8,
			RunCount:    89,
			ManualCount: 23,
		},
		{
			Name:        "frank",
			GPUHours:    76.00,
			Percentage:  6.7,
			RunCount:    45,
			ManualCount: 12,
		},
	}

	totalHours := 0.0
	totalRun := 0
	totalManual := 0
	for _, u := range users {
		totalHours += u.GPUHours
		totalRun += u.RunCount
		totalManual += u.ManualCount
	}

	return reportData{
		Users:             users,
		TotalGPUHours:     totalHours,
		TotalReservations: totalRun + totalManual,
		UniqueUsers:       len(users),
		StartDate:         startTime.Format("2006-01-02"),
		EndDate:           endTime.Format("2006-01-02"),
		Days:              days,
	}
}
