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

func cmdTest(args []string) {
	if len(args) < 1 {
		printTestUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "cases":
		cmdTestCases(args[1:])
	case "scripts":
		cmdTestScripts(args[1:])
	case "run":
		cmdTestRun(args[1:])
	case "generate":
		cmdTestGenerate(args[1:])
	case "status":
		cmdTestStatus(args[1:])
	case "help", "--help", "-h":
		printTestUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown test subcommand: %s\n\n", sub)
		printTestUsage()
		os.Exit(1)
	}
}

func printTestUsage() {
	fmt.Println(`Usage: qmax test <subcommand> [flags]

Subcommands:
  cases      List test cases for a project
  scripts    List automation scripts for a project
  run        Execute test scripts (single or batch)
  generate   Generate Playwright code for a test case
  status     Check execution status

Examples:
  qmax test cases --project-id 42
  qmax test scripts --project-id 42
  qmax test run --script-id 101
  qmax test run --script-ids 101,102,103 --headless
  qmax test generate --test-case-id 55
  qmax test status --execution-id agent_exec_101_1710000000`)
}

// --- test cases ---

func cmdTestCases(args []string) {
	fs := flag.NewFlagSet("test cases", flag.ExitOnError)
	projectID := fs.Int("project-id", 0, "Project ID (required)")
	limit := fs.Int("limit", 50, "Max results to return")
	search := fs.String("search", "", "Search in title/description")
	category := fs.String("category", "", "Filter by category")
	status := fs.String("status", "", "Filter by status")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	url := fmt.Sprintf("%s/api/test-cases/project/%d?limit=%d", apiURL, *projectID, *limit)
	if *search != "" {
		url += "&search=" + *search
	}
	if *category != "" {
		url += "&category=" + *category
	}
	if *status != "" {
		url += "&status=" + *status
	}

	body := authGet(cfg, url)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success   bool   `json:"success"`
		Count     int    `json:"count"`
		Project   struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		} `json:"project"`
		TestCases []struct {
			ID       json.Number `json:"id"`
			Title    string      `json:"title"`
			Category string      `json:"category"`
			Priority int         `json:"priority"`
			Status   string      `json:"status"`
			Automated bool       `json:"automated"`
		} `json:"test_cases"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Project: %s (#%s)\n", resp.Project.Name, resp.Project.ID.String())
	fmt.Printf("Test cases: %d\n\n", resp.Count)

	if resp.Count == 0 {
		fmt.Println("No test cases found.")
		return
	}

	fmt.Printf("%-6s  %-10s  %-4s  %-10s  %-4s  %s\n", "ID", "Category", "Pri", "Status", "Auto", "Title")
	fmt.Printf("%-6s  %-10s  %-4s  %-10s  %-4s  %s\n", "------", "----------", "----", "----------", "----", "-----")
	for _, tc := range resp.TestCases {
		auto := " "
		if tc.Automated {
			auto = "Y"
		}
		title := tc.Title
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		fmt.Printf("%-6s  %-10s  %-4d  %-10s  %-4s  %s\n",
			tc.ID.String(), truncate(tc.Category, 10), tc.Priority, truncate(tc.Status, 10), auto, title)
	}
}

// --- test scripts ---

func cmdTestScripts(args []string) {
	fs := flag.NewFlagSet("test scripts", flag.ExitOnError)
	projectID := fs.Int("project-id", 0, "Project ID (required)")
	limit := fs.Int("limit", 50, "Max results to return")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	url := fmt.Sprintf("%s/api/automation/scripts/project/%d?limit=%d", apiURL, *projectID, *limit)
	body := authGet(cfg, url)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success bool `json:"success"`
		Count   int  `json:"count"`
		Scripts []struct {
			ID         json.Number `json:"id"`
			TestCaseID json.Number `json:"test_case_id"`
			Name       string      `json:"name"`
			Framework  string      `json:"framework"`
			CreatedAt  string      `json:"created_at"`
		} `json:"scripts"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Scripts: %d\n\n", resp.Count)

	if resp.Count == 0 {
		fmt.Println("No scripts found.")
		return
	}

	fmt.Printf("%-6s  %-8s  %-12s  %s\n", "ID", "TC ID", "Framework", "Name")
	fmt.Printf("%-6s  %-8s  %-12s  %s\n", "------", "--------", "------------", "----")
	for _, s := range resp.Scripts {
		name := s.Name
		if len(name) > 50 {
			name = name[:47] + "..."
		}
		fmt.Printf("%-6s  %-8s  %-12s  %s\n",
			s.ID.String(), s.TestCaseID.String(), s.Framework, name)
	}
}

// --- test run ---

func cmdTestRun(args []string) {
	fs := flag.NewFlagSet("test run", flag.ExitOnError)
	scriptID := fs.Int("script-id", 0, "Single script ID to execute")
	scriptIDs := fs.String("script-ids", "", "Comma-separated script IDs for batch execution")
	baseURL := fs.String("base-url", "", "Base URL for the application under test")
	headless := fs.Bool("headless", true, "Run in headless mode (default: true)")
	browser := fs.String("browser", "chromium", "Browser: chromium, firefox, webkit")
	wait := fs.Bool("wait", false, "Wait for execution to complete and show result")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *scriptID == 0 && *scriptIDs == "" {
		fmt.Fprintln(os.Stderr, "Error: --script-id or --script-ids is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	if *scriptIDs != "" {
		// Batch execution
		ids := parseIntList(*scriptIDs)
		if len(ids) == 0 {
			fmt.Fprintln(os.Stderr, "Error: --script-ids must be comma-separated integers")
			os.Exit(1)
		}

		payload := map[string]interface{}{
			"script_ids": ids,
		}
		if *baseURL != "" {
			payload["custom_url"] = *baseURL
		}

		url := fmt.Sprintf("%s/api/automation/execute-batch", apiURL)
		body := authPost(cfg, url, payload)

		if *jsonOut {
			fmt.Println(string(body))
			return
		}

		var resp struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		mustUnmarshal(body, &resp)

		if resp.Success {
			fmt.Printf("Batch execution started for %d scripts\n", len(ids))
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Message)
			os.Exit(1)
		}
		return
	}

	// Single script execution via playwright-execution endpoint
	payload := map[string]interface{}{
		"headless":       *headless,
		"browser":        *browser,
		"record_video":   true,
		"viewport_width": 1280,
		"viewport_height": 720,
	}
	if *baseURL != "" {
		payload["base_url"] = *baseURL
	}

	url := fmt.Sprintf("%s/api/playwright-execution/run/%d", apiURL, *scriptID)
	body := authPost(cfg, url, payload)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Status      string `json:"status"`
		ExecutionID string `json:"execution_id"`
		Message     string `json:"message"`
		ScriptID    int    `json:"script_id"`
		ScriptName  string `json:"script_name"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Execution started: %s\n", resp.ExecutionID)
	fmt.Printf("Script: %s (#%d)\n", resp.ScriptName, resp.ScriptID)

	if *wait {
		fmt.Println("\nWaiting for completion...")
		pollExecution(cfg, apiURL, resp.ExecutionID, *jsonOut)
	} else {
		fmt.Printf("\nCheck status: qmax test status --execution-id %s\n", resp.ExecutionID)
	}
}

// --- test generate ---

func cmdTestGenerate(args []string) {
	fs := flag.NewFlagSet("test generate", flag.ExitOnError)
	testCaseID := fs.Int("test-case-id", 0, "Test case ID to generate code for (required)")
	force := fs.Bool("force", false, "Regenerate even if code already exists")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *testCaseID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --test-case-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	payload := map[string]interface{}{}
	if *force {
		payload["force"] = true
	}

	url := fmt.Sprintf("%s/api/test-cases/%d/generate-code", apiURL, *testCaseID)
	body := authPost(cfg, url, payload)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success  bool        `json:"success"`
		Message  string      `json:"message"`
		ScriptID json.Number `json:"script_id"`
		Code     string      `json:"code"`
	}
	mustUnmarshal(body, &resp)

	if resp.Success {
		fmt.Printf("Code generated for test case #%d\n", *testCaseID)
		if resp.ScriptID.String() != "" {
			fmt.Printf("Script ID: %s\n", resp.ScriptID.String())
		}
		fmt.Printf("\nRun it: qmax test run --script-id %s\n", resp.ScriptID.String())
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Message)
		os.Exit(1)
	}
}

// --- test status ---

func cmdTestStatus(args []string) {
	fs := flag.NewFlagSet("test status", flag.ExitOnError)
	executionID := fs.String("execution-id", "", "Execution ID to check (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *executionID == "" {
		fmt.Fprintln(os.Stderr, "Error: --execution-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()
	pollExecution(cfg, apiURL, *executionID, *jsonOut)
}

// --- polling ---

func pollExecution(cfg *Config, apiURL, executionID string, jsonOut bool) {
	url := fmt.Sprintf("%s/api/playwright-execution/status/%s", apiURL, executionID)

	maxAttempts := 120 // 10 minutes at 5s intervals
	for i := 0; i < maxAttempts; i++ {
		body := authGet(cfg, url)

		var resp struct {
			Status       string   `json:"status"`
			Phase        string   `json:"phase"`
			Passed       *bool    `json:"passed"`
			ErrorMessage string   `json:"error_message"`
			Screenshots  []string `json:"screenshots"`
			VideoURL     string   `json:"video_url"`
			Duration     float64  `json:"duration_seconds"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			// Might be an error response
			fmt.Fprintf(os.Stderr, "Error parsing status: %v\n", err)
			os.Exit(1)
		}

		if jsonOut {
			fmt.Println(string(body))
		}

		switch resp.Status {
		case "completed", "passed":
			if !jsonOut {
				fmt.Printf("Status: PASSED\n")
				if resp.Duration > 0 {
					fmt.Printf("Duration: %.1fs\n", resp.Duration)
				}
				if len(resp.Screenshots) > 0 {
					fmt.Printf("Screenshots: %d captured\n", len(resp.Screenshots))
				}
				if resp.VideoURL != "" {
					fmt.Printf("Video: %s\n", resp.VideoURL)
				}
			}
			return
		case "failed":
			if !jsonOut {
				fmt.Printf("Status: FAILED\n")
				if resp.ErrorMessage != "" {
					fmt.Printf("Error: %s\n", resp.ErrorMessage)
				}
				if resp.Duration > 0 {
					fmt.Printf("Duration: %.1fs\n", resp.Duration)
				}
			}
			os.Exit(1)
		default:
			if !jsonOut && i == 0 {
				fmt.Printf("Status: %s", resp.Status)
				if resp.Phase != "" {
					fmt.Printf(" (%s)", resp.Phase)
				}
				fmt.Println()
			} else if !jsonOut && i%6 == 0 { // Print every 30s
				fmt.Printf("  ...still %s", resp.Status)
				if resp.Phase != "" {
					fmt.Printf(" (%s)", resp.Phase)
				}
				fmt.Println()
			}
		}

		time.Sleep(5 * time.Second)
	}

	fmt.Fprintln(os.Stderr, "Timed out waiting for execution to complete")
	os.Exit(1)
}

// =============================================================================
// Helpers
// =============================================================================

func mustLoadConfig() *Config {
	cfg, err := LoadConfig()
	if err != nil || cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: not logged in. Run `qmax login` first.")
		os.Exit(1)
	}
	return cfg
}

func authGet(cfg *Config, url string) []byte {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Token))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	const maxBody = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: %d - %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	return body
}

func authPost(cfg *Config, url string, payload interface{}) []byte {
	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Token))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	const maxBody = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Error: %d - %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	return body
}

func mustUnmarshal(data []byte, v interface{}) {
	if err := json.Unmarshal(data, v); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}
}

func parseIntList(s string) []int {
	parts := strings.Split(s, ",")
	var result []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result = append(result, n)
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
