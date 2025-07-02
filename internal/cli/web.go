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

	"github.com/spf13/cobra"
	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
)

var (
	webPort   int
	webHost   string
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
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	
	// Initialize Redis client
	config := getConfig()
	client := redis_client.NewClient(config)
	defer client.Close()

	// Test connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create server
	server := &webServer{
		client: client,
		engine: gpu.NewAllocationEngine(client, config),
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
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0f0f0f;
            color: #e0e0e0;
            line-height: 1.6;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background: #1a1a1a;
            padding: 20px 0;
            margin-bottom: 30px;
            border-bottom: 1px solid #333;
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
        .header-icons a {
            color: #888;
            text-decoration: none;
            padding: 8px;
            border-radius: 4px;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .header-icons a:hover {
            color: #4CAF50;
            background: #2a2a2a;
        }
        .header-icons svg {
            width: 24px;
            height: 24px;
            fill: currentColor;
        }
        h1 {
            color: #4CAF50;
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        .subtitle {
            color: #888;
            font-size: 1.2em;
        }
        .section {
            background: #1a1a1a;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 30px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.3);
        }
        h2 {
            color: #4CAF50;
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
            background: #252525;
            border-radius: 6px;
            padding: 12px;
            border: 1px solid #333;
            transition: all 0.3s ease;
            cursor: pointer;
        }
        .gpu-card:hover {
            border-color: #4CAF50;
            box-shadow: 0 4px 12px rgba(76, 175, 80, 0.2);
        }
        .gpu-card.expanded {
            padding: 15px;
        }
        .gpu-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            position: relative;
            min-height: 40px;
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
            fill: #4CAF50;
            opacity: 0.8;
        }
        .expand-icon {
            width: 20px;
            height: 20px;
            fill: #666;
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
            color: #888;
            margin-left: 5px;
            text-align: center;
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
            color: #aaa;
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid #333;
            display: none;
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
            color: #888;
            font-weight: 500;
        }
        select, button {
            background: #252525;
            color: #e0e0e0;
            border: 1px solid #444;
            padding: 8px 15px;
            border-radius: 4px;
            font-size: 14px;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        select:hover, button:hover {
            border-color: #4CAF50;
            background: #2a2a2a;
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
            border-bottom: 1px solid #333;
        }
        .usage-table th {
            background: #252525;
            color: #4CAF50;
            font-weight: 600;
        }
        .usage-table tr:hover {
            background: #252525;
        }
        .usage-bar {
            display: inline-block;
            height: 20px;
            background: #4CAF50;
            border-radius: 3px;
            margin-right: 10px;
            vertical-align: middle;
        }
        .loading {
            text-align: center;
            padding: 40px;
            color: #888;
        }
        .error {
            background: #d32f2f;
            color: white;
            padding: 15px;
            border-radius: 6px;
            margin: 20px 0;
        }
        .timestamp {
            color: #666;
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
    </style>
</head>
<body>
    <header>
        <div class="container">
            <div class="header-content">
                <div class="header-text">
                    <h1>canhazgpu Dashboard</h1>
                    <div class="subtitle">GPU Reservation System Monitor - {{.Hostname}}</div>
                </div>
                <div class="header-icons">
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
            
            if (diff < 60000) return 'just now';
            if (diff < 3600000) return Math.floor(diff / 60000) + ' minutes ago';
            if (diff < 86400000) return Math.floor(diff / 3600000) + ' hours ago';
            return Math.floor(diff / 86400000) + ' days ago';
        }

        function getProviderIcon(provider) {
            switch (provider) {
                case 'openai':
                    return '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z" fill="currentColor"/></svg>';
                case 'meta-llama':
                    return '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M12 2C6.477 2 2 6.477 2 12s4.477 10 10 10 10-4.477 10-10S17.523 2 12 2zm0 18c-4.411 0-8-3.589-8-8s3.589-8 8-8 8 3.589 8 8-3.589 8-8 8zm-1-13h2v6h-2zm0 8h2v2h-2z" fill="currentColor"/><circle cx="12" cy="8" r="1.5" fill="currentColor"/></svg>';
                case 'qwen':
                    return '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>';
                case 'deepseek-ai':
                    return '<svg height="1em" style="flex:none;line-height:1" viewBox="0 0 24 24" width="1em" xmlns="http://www.w3.org/2000/svg"><title>DeepSeek</title><path d="M23.748 4.482c-.254-.124-.364.113-.512.234-.051.039-.094.09-.137.136-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.156-.708-.311-.955-.65-.172-.241-.219-.51-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.093.172.187.129.323-.082.28-.18.552-.266.833-.055.179-.137.217-.329.14a5.526 5.526 0 01-1.736-1.18c-.857-.828-1.631-1.742-2.597-2.458a11.365 11.365 0 00-.689-.471c-.985-.957.13-1.743.388-1.836.27-.098.093-.432-.779-.428-.872.004-1.67.295-2.687.684a3.055 3.055 0 01-.465.137 9.597 9.597 0 00-2.883-.102c-1.885.21-3.39 1.102-4.497 2.623C.082 8.606-.231 10.684.152 12.85c.403 2.284 1.569 4.175 3.36 5.653 1.858 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.133-.284 4.994-1.86.47.234.962.327 1.78.397.63.059 1.236-.03 1.705-.128.735-.156.684-.837.419-.961-2.155-1.004-1.682-.595-2.113-.926 1.096-1.296 2.746-2.642 3.392-7.003.05-.347.007-.565 0-.845-.004-.17.035-.237.23-.256a4.173 4.173 0 001.545-.475c1.396-.763 1.96-2.015 2.093-3.517.02-.23-.004-.467-.247-.588zM11.581 18c-2.089-1.642-3.102-2.183-3.52-2.16-.392.024-.321.471-.235.763.09.288.207.486.371.739.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.167-1.361-.802-2.5-1.86-3.301-3.307-.774-1.393-1.224-2.887-1.298-4.482-.02-.386.093-.522.477-.592a4.696 4.696 0 011.529-.039c2.132.312 3.946 1.265 5.468 2.774.868.86 1.525 1.887 2.202 2.891.72 1.066 1.494 2.082 2.48 2.914.348.292.625.514.891.677-.802.09-2.14.11-3.054-.614zm1-6.44a.306.306 0 01.415-.287.302.302 0 01.2.288.306.306 0 01-.31.307.303.303 0 01-.304-.308zm3.11 1.596c-.2.081-.399.151-.59.16a1.245 1.245 0 01-.798-.254c-.274-.23-.47-.358-.552-.758a1.73 1.73 0 01.016-.588c.07-.327-.008-.537-.239-.727-.187-.156-.426-.199-.688-.199a.559.559 0 01-.254-.078c-.11-.054-.2-.19-.114-.358.028-.054.16-.186.192-.21.356-.202.767-.136 1.146.016.352.144.618.408 1.001.782.391.451.462.576.685.914.176.265.336.537.445.848.067.195-.019.354-.25.452z" fill="#4D6BFE"></path></svg>';
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
                
                html += '<div class="gpu-card" onclick="toggleCard(this)">';
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
            
            document.getElementById('status-timestamp').textContent = 'Last updated: ' + new Date().toLocaleTimeString();
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
            
            document.getElementById('report-timestamp').textContent = 'Last updated: ' + new Date().toLocaleTimeString();
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
    </script>
</body>
</html>`

	t, err := template.New("index").Parse(tmpl)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	t.Execute(w, struct {
		Hostname string
	}{
		Hostname: hostname,
	})
}

func (ws *webServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Clean up expired reservations first
	if err := ws.engine.CleanupExpiredReservations(ctx); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: Failed to cleanup expired reservations: %v\n", err)
	}

	statuses, err := ws.engine.GetGPUStatus(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get GPU status: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to JSON-friendly format
	type jsonGPUStatus struct {
		GPUID             int               `json:"gpu_id"`
		Status            string            `json:"status"`
		User              string            `json:"user,omitempty"`
		ReservationType   string            `json:"reservation_type,omitempty"`
		Duration          int64             `json:"duration,omitempty"`
		LastHeartbeat     *time.Time        `json:"last_heartbeat,omitempty"`
		ExpiryTime        *time.Time        `json:"expiry_time,omitempty"`
		LastReleased      *time.Time        `json:"last_released,omitempty"`
		ValidationInfo    string            `json:"validation_info,omitempty"`
		UnreservedUsers   []string          `json:"unreserved_users,omitempty"`
		ProcessInfo       string            `json:"process_info,omitempty"`
		Error             string            `json:"error,omitempty"`
		ModelInfo         *gpu.ModelInfo    `json:"model_info,omitempty"`
	}

	jsonStatuses := make([]jsonGPUStatus, len(statuses))
	for i, status := range statuses {
		js := jsonGPUStatus{
			GPUID:             status.GPUID,
			Status:            status.Status,
			User:              status.User,
			ReservationType:   status.ReservationType,
			Duration:          int64(status.Duration),
			ValidationInfo:    status.ValidationInfo,
			UnreservedUsers:   status.UnreservedUsers,
			ProcessInfo:       status.ProcessInfo,
			Error:             status.Error,
			ModelInfo:         status.ModelInfo,
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
	json.NewEncoder(w).Encode(jsonStatuses)
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
	currentRecords := getCurrentUsageRecords(currentStatuses, endTime)
	allRecords := append(historicalRecords, currentRecords...)

	// Generate report data
	reportData := generateReportData(allRecords, startTime, endTime, days)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reportData)
}

type reportData struct {
	Users            []userReport `json:"users"`
	TotalGPUHours    float64     `json:"total_gpu_hours"`
	TotalReservations int        `json:"total_reservations"`
	UniqueUsers      int         `json:"unique_users"`
	StartDate        string      `json:"start_date"`
	EndDate          string      `json:"end_date"`
	Days             int         `json:"days"`
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