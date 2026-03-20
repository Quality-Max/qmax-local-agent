package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// qmax ci — headless CI runner for GitHub Actions / CI pipelines
// Bundles: authenticate -> run tests -> wait -> output results -> exit code
//
// Usage:
//   qmax ci run --project-id 42 [--script-ids 1,2,3] [--base-url https://staging.app.com]
//   qmax ci run --project-id 42 --all    (run all scripts in project)
//
// Auth: uses --token flag or QMAX_TOKEN env var (no browser needed)
// Output: markdown summary + GitHub Actions step outputs

const (
	defaultCITimeout = 600
	ciMaxBody        = 10 * 1024 * 1024 // 10MB
)

// ciTestResult holds the result of a single test execution.
type ciTestResult struct {
	ScriptID    int
	ScriptName  string
	Status      string // passed, failed, error
	Duration    float64
	Error       string
	ExecutionID string
}

func cmdCI(args []string) {
	if len(args) < 1 {
		printCIUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "run":
		cmdCIRun(args[1:])
	case "help", "--help", "-h":
		printCIUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown ci subcommand: %s\n\n", sub)
		printCIUsage()
		os.Exit(1)
	}
}

func printCIUsage() {
	fmt.Println(`Usage: qmax ci <subcommand> [flags]

Subcommands:
  run        Execute tests in CI mode (headless, no interactive prompts)

Run flags:
  --project-id    Project ID (required)
  --script-ids    Comma-separated script IDs to execute
  --all           Run all scripts in the project
  --base-url      Override base URL for all tests
  --token         Auth token (or set QMAX_TOKEN env var)
  --api-url       API base URL (default: https://app.qualitymax.io)
  --headless      Run in headless mode (default: true)
  --browser       Browser to use (default: chromium)
  --timeout       Overall timeout in seconds (default: 600)
  --format        Output format: markdown or json (default: markdown)

Examples:
  qmax ci run --project-id 42 --all
  qmax ci run --project-id 42 --script-ids 1,2,3 --base-url https://staging.app.com
  QMAX_TOKEN=xxx qmax ci run --project-id 42 --all`)
}

func cmdCIRun(args []string) {
	fs := flag.NewFlagSet("ci run", flag.ExitOnError)

	projectID := fs.Int("project-id", 0, "Project ID (required)")
	scriptIDsStr := fs.String("script-ids", "", "Comma-separated script IDs")
	allScripts := fs.Bool("all", false, "Run all scripts in the project")
	baseURL := fs.String("base-url", "", "Override base URL for tests")
	token := fs.String("token", "", "Auth token (or QMAX_TOKEN env var)")
	apiURL := fs.String("api-url", "", "API base URL")
	headless := fs.Bool("headless", true, "Run in headless mode")
	browser := fs.String("browser", "chromium", "Browser to use")
	timeout := fs.Int("timeout", defaultCITimeout, "Overall timeout in seconds")
	format := fs.String("format", "markdown", "Output format: markdown or json")

	_ = fs.Parse(args)

	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		fs.Usage()
		os.Exit(1)
	}

	if *scriptIDsStr == "" && !*allScripts {
		fmt.Fprintln(os.Stderr, "Error: either --script-ids or --all is required")
		fs.Usage()
		os.Exit(1)
	}

	// Resolve token: flag > env > config
	resolvedToken := *token
	if resolvedToken == "" {
		resolvedToken = os.Getenv("QMAX_TOKEN")
	}

	cfg, err := LoadConfig()
	if err != nil {
		cfg = &Config{}
	}

	if resolvedToken == "" {
		resolvedToken = cfg.Token
	}
	if resolvedToken == "" {
		fmt.Fprintln(os.Stderr, "Error: no auth token. Use --token flag, QMAX_TOKEN env var, or run `qmax login` first.")
		os.Exit(1)
	}

	// Set token in config for this session
	cfg.Token = resolvedToken

	// Resolve API URL: flag > config > default
	resolvedAPIURL := *apiURL
	if resolvedAPIURL == "" {
		resolvedAPIURL = cfg.GetAPIBaseURL()
	}
	if resolvedAPIURL == "" {
		resolvedAPIURL = defaultAPIURL
	}

	client := &http.Client{Timeout: time.Duration(*timeout) * time.Second}

	startTime := time.Now()

	// Resolve script IDs
	var scriptIDs []int
	if *allScripts {
		scriptIDs, err = ciFetchProjectScripts(client, resolvedAPIURL, resolvedToken, *projectID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching project scripts: %v\n", err)
			os.Exit(1)
		}
		if len(scriptIDs) == 0 {
			fmt.Fprintln(os.Stderr, "Error: no scripts found in project")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Found %d scripts in project #%d\n", len(scriptIDs), *projectID)
	} else {
		scriptIDs, err = ciParseIntList(*scriptIDsStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing --script-ids: %v\n", err)
			os.Exit(1)
		}
	}

	// Execute tests
	fmt.Fprintf(os.Stderr, "Executing %d test(s)...\n", len(scriptIDs))

	executionIDs, err := ciExecuteTests(client, resolvedAPIURL, resolvedToken, scriptIDs, *baseURL, *headless, *browser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting test executions: %v\n", err)
		os.Exit(1)
	}

	// Poll for results
	deadline := time.Now().Add(time.Duration(*timeout) * time.Second)
	results := ciPollAllExecutions(client, resolvedAPIURL, resolvedToken, executionIDs, deadline)

	totalDuration := time.Since(startTime).Seconds()

	// Generate output
	switch *format {
	case "json":
		ciOutputJSON(results, *projectID, totalDuration)
	default:
		ciOutputMarkdown(results, *projectID, totalDuration)
	}

	// Write GitHub Actions outputs
	ciWriteGitHubOutputs(results, *projectID, totalDuration)

	// Exit code
	for _, r := range results {
		if r.Status != "passed" {
			os.Exit(1)
		}
	}
}

// ciFetchProjectScripts fetches all script IDs for a project.
func ciFetchProjectScripts(client *http.Client, apiURL, token string, projectID int) ([]int, error) {
	url := fmt.Sprintf("%s/api/automation/scripts/project/%d?limit=500", apiURL, projectID)
	body, err := ciAuthGet(client, url, token)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Scripts []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse scripts response: %w", err)
	}

	ids := make([]int, len(resp.Scripts))
	for i, s := range resp.Scripts {
		ids[i] = s.ID
	}
	return ids, nil
}

// ciExecuteTests starts execution for each script and returns execution IDs.
func ciExecuteTests(client *http.Client, apiURL, token string, scriptIDs []int, baseURL string, headless bool, browser string) ([]ciExecution, error) {
	var executions []ciExecution

	for _, sid := range scriptIDs {
		url := fmt.Sprintf("%s/api/playwright-execution/run/%d", apiURL, sid)

		payload := map[string]interface{}{
			"headless": headless,
			"browser":  browser,
		}
		if baseURL != "" {
			payload["base_url"] = baseURL
		}

		body, err := ciAuthPost(client, url, token, payload)
		if err != nil {
			// Record as error but continue with other scripts
			executions = append(executions, ciExecution{
				ScriptID:    sid,
				ExecutionID: "",
				Error:       err.Error(),
			})
			continue
		}

		var resp struct {
			Success     bool   `json:"success"`
			ExecutionID string `json:"execution_id"`
			Message     string `json:"message"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			executions = append(executions, ciExecution{
				ScriptID: sid,
				Error:    fmt.Sprintf("parse response: %v", err),
			})
			continue
		}

		if !resp.Success {
			executions = append(executions, ciExecution{
				ScriptID: sid,
				Error:    resp.Message,
			})
			continue
		}

		fmt.Fprintf(os.Stderr, "  Started script #%d -> execution %s\n", sid, resp.ExecutionID)
		executions = append(executions, ciExecution{
			ScriptID:    sid,
			ExecutionID: resp.ExecutionID,
		})
	}

	return executions, nil
}

type ciExecution struct {
	ScriptID    int
	ExecutionID string
	Error       string
}

// ciPollAllExecutions polls all executions until they complete or timeout.
func ciPollAllExecutions(client *http.Client, apiURL, token string, executions []ciExecution, deadline time.Time) []ciTestResult {
	results := make([]ciTestResult, len(executions))

	// Initialize results for executions that already errored
	for i, exec := range executions {
		if exec.Error != "" {
			results[i] = ciTestResult{
				ScriptID: exec.ScriptID,
				Status:   "error",
				Error:    exec.Error,
			}
		}
	}

	// Poll active executions
	pollInterval := 5 * time.Second

	for {
		allDone := true
		for i, exec := range executions {
			if exec.Error != "" || exec.ExecutionID == "" {
				continue // already done
			}
			if results[i].Status != "" {
				continue // already resolved
			}

			allDone = false

			status, err := ciGetExecutionStatus(client, apiURL, token, exec.ExecutionID)
			if err != nil {
				// Transient error, keep polling
				continue
			}

			if status.isTerminal() {
				results[i] = ciTestResult{
					ScriptID:    exec.ScriptID,
					ScriptName:  status.ScriptName,
					Status:      status.resultStatus(),
					Duration:    status.Duration,
					Error:       status.ErrorMessage,
					ExecutionID: exec.ExecutionID,
				}
				statusIcon := "PASS"
				if results[i].Status != "passed" {
					statusIcon = "FAIL"
				}
				fmt.Fprintf(os.Stderr, "  [%s] Script #%d (%s) - %.1fs\n",
					statusIcon, exec.ScriptID, results[i].ScriptName, status.Duration)
			}
		}

		if allDone {
			break
		}

		if time.Now().After(deadline) {
			// Timeout remaining executions
			for i, exec := range executions {
				if results[i].Status == "" && exec.Error == "" {
					results[i] = ciTestResult{
						ScriptID:    exec.ScriptID,
						Status:      "error",
						Error:       "timeout waiting for execution to complete",
						ExecutionID: exec.ExecutionID,
					}
				}
			}
			break
		}

		time.Sleep(pollInterval)
	}

	return results
}

type ciExecutionStatus struct {
	Status       string  `json:"status"`
	Progress     int     `json:"progress"`
	Success      bool    `json:"success"`
	Duration     float64 `json:"execution_time"`
	ScriptName   string  `json:"script_name"`
	ErrorMessage string  `json:"error_message"`
	Errors       []string `json:"errors"`
	TestErrors   string  `json:"test_errors"`
}

func (s *ciExecutionStatus) isTerminal() bool {
	switch s.Status {
	case "completed", "failed", "error", "cancelled":
		return true
	}
	return s.Progress >= 100
}

func (s *ciExecutionStatus) resultStatus() string {
	if s.Success || s.Status == "completed" {
		// Check if it actually passed
		if len(s.Errors) > 0 || s.TestErrors != "" {
			return "failed"
		}
		return "passed"
	}
	return "failed"
}

// ciGetExecutionStatus fetches the current status of an execution.
func ciGetExecutionStatus(client *http.Client, apiURL, token, executionID string) (*ciExecutionStatus, error) {
	url := fmt.Sprintf("%s/api/playwright-execution/status/%s", apiURL, executionID)
	body, err := ciAuthGet(client, url, token)
	if err != nil {
		return nil, err
	}

	var status ciExecutionStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	// Build error message from available fields
	if status.ErrorMessage == "" && len(status.Errors) > 0 {
		status.ErrorMessage = strings.Join(status.Errors, "\n")
	}
	if status.ErrorMessage == "" && status.TestErrors != "" {
		status.ErrorMessage = status.TestErrors
	}

	return &status, nil
}

// ciOutputMarkdown generates a markdown summary of the test results.
func ciOutputMarkdown(results []ciTestResult, projectID int, totalDuration float64) {
	var sb strings.Builder

	passed := 0
	failed := 0
	for _, r := range results {
		if r.Status == "passed" {
			passed++
		} else {
			failed++
		}
	}

	overallStatus := "Passed"
	statusIcon := "\u2705"
	if failed > 0 {
		overallStatus = "Failed"
		statusIcon = "\u274c"
	}

	sb.WriteString("## \U0001f9ea QualityMax Test Results\n\n")
	sb.WriteString(fmt.Sprintf("**Project:** #%d | **Status:** %s %s | **Duration:** %.1fs\n\n",
		projectID, statusIcon, overallStatus, totalDuration))

	sb.WriteString("| Test | Status | Duration |\n")
	sb.WriteString("|------|--------|----------|\n")

	for _, r := range results {
		name := r.ScriptName
		if name == "" {
			name = fmt.Sprintf("Script #%d", r.ScriptID)
		}
		icon := "\u2705 Passed"
		if r.Status != "passed" {
			icon = "\u274c Failed"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %.1fs |\n", name, icon, r.Duration))
	}

	// Failed test details
	if failed > 0 {
		sb.WriteString("\n### \u274c Failed Tests\n\n")
		for _, r := range results {
			if r.Status == "passed" {
				continue
			}
			name := r.ScriptName
			if name == "" {
				name = fmt.Sprintf("Script #%d", r.ScriptID)
			}
			errMsg := r.Error
			if errMsg == "" {
				errMsg = "Unknown error"
			}
			// Truncate long errors
			if len(errMsg) > 500 {
				errMsg = errMsg[:500] + "..."
			}
			sb.WriteString(fmt.Sprintf("**%s** (%.1fs)\n```\n%s\n```\n\n", name, r.Duration, errMsg))
		}
	}

	sb.WriteString("---\n*Powered by [QualityMax](https://qualitymax.io) — AI-powered test automation*\n")

	summary := sb.String()
	fmt.Print(summary)
}

// ciOutputJSON generates JSON output of the test results.
func ciOutputJSON(results []ciTestResult, projectID int, totalDuration float64) {
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Status == "passed" {
			passed++
		} else {
			failed++
		}
	}

	status := "passed"
	if failed > 0 {
		status = "failed"
	}

	output := map[string]interface{}{
		"project_id": projectID,
		"status":     status,
		"total":      len(results),
		"passed":     passed,
		"failed":     failed,
		"duration":   totalDuration,
		"results":    results,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
}

// ciWriteGitHubOutputs writes outputs for GitHub Actions.
func ciWriteGitHubOutputs(results []ciTestResult, projectID int, totalDuration float64) {
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Status == "passed" {
			passed++
		} else {
			failed++
		}
	}

	status := "passed"
	if failed > 0 {
		status = "failed"
	}

	// Write to GITHUB_OUTPUT
	if ghOutput := os.Getenv("GITHUB_OUTPUT"); ghOutput != "" {
		f, err := os.OpenFile(ghOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			fmt.Fprintf(f, "status=%s\n", status)
			fmt.Fprintf(f, "total=%d\n", len(results))
			fmt.Fprintf(f, "passed=%d\n", passed)
			fmt.Fprintf(f, "failed=%d\n", failed)
			fmt.Fprintf(f, "duration=%.1f\n", totalDuration)

			// For multiline summary, use heredoc delimiter
			fmt.Fprintf(f, "summary<<QMAX_EOF\n")
			// Re-generate markdown for the summary value
			var sb strings.Builder
			statusIcon := "\u2705"
			overallLabel := "Passed"
			if failed > 0 {
				statusIcon = "\u274c"
				overallLabel = "Failed"
			}
			sb.WriteString(fmt.Sprintf("**Project:** #%d | **Status:** %s %s | **Passed:** %d/%d | **Duration:** %.1fs",
				projectID, statusIcon, overallLabel, passed, len(results), totalDuration))
			fmt.Fprintf(f, "%s\n", sb.String())
			fmt.Fprintf(f, "QMAX_EOF\n")
		}
	}

	// Write to GITHUB_STEP_SUMMARY
	if ghSummary := os.Getenv("GITHUB_STEP_SUMMARY"); ghSummary != "" {
		f, err := os.OpenFile(ghSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()

			statusIcon := "\u2705"
			overallLabel := "Passed"
			if failed > 0 {
				statusIcon = "\u274c"
				overallLabel = "Failed"
			}

			fmt.Fprintf(f, "## \U0001f9ea QualityMax Test Results\n\n")
			fmt.Fprintf(f, "**Project:** #%d | **Status:** %s %s | **Duration:** %.1fs\n\n",
				projectID, statusIcon, overallLabel, totalDuration)
			fmt.Fprintf(f, "| Test | Status | Duration |\n")
			fmt.Fprintf(f, "|------|--------|----------|\n")

			for _, r := range results {
				name := r.ScriptName
				if name == "" {
					name = fmt.Sprintf("Script #%d", r.ScriptID)
				}
				icon := "\u2705 Passed"
				if r.Status != "passed" {
					icon = "\u274c Failed"
				}
				fmt.Fprintf(f, "| %s | %s | %.1fs |\n", name, icon, r.Duration)
			}

			if failed > 0 {
				fmt.Fprintf(f, "\n### \u274c Failed Tests\n\n")
				for _, r := range results {
					if r.Status == "passed" {
						continue
					}
					name := r.ScriptName
					if name == "" {
						name = fmt.Sprintf("Script #%d", r.ScriptID)
					}
					errMsg := r.Error
					if errMsg == "" {
						errMsg = "Unknown error"
					}
					if len(errMsg) > 500 {
						errMsg = errMsg[:500] + "..."
					}
					fmt.Fprintf(f, "**%s** (%.1fs)\n```\n%s\n```\n\n", name, r.Duration, errMsg)
				}
			}

			fmt.Fprintf(f, "\n---\n*Powered by [QualityMax](https://qualitymax.io) — AI-powered test automation*\n")
		}
	}
}

// --- HTTP helpers for CI ---

func ciAuthGet(client *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, ciMaxBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func ciAuthPost(client *http.Client, url, token string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, ciMaxBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func ciParseIntList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	var ids []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid script ID %q: %w", p, err)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no valid script IDs provided")
	}
	return ids, nil
}
