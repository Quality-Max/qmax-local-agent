package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func cmdCrawl(args []string) {
	if len(args) < 1 {
		printCrawlUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "start":
		cmdCrawlStart(args[1:])
	case "status":
		cmdCrawlStatus(args[1:])
	case "results":
		cmdCrawlResults(args[1:])
	case "jobs":
		cmdCrawlJobs(args[1:])
	case "help", "--help", "-h":
		printCrawlUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown crawl subcommand: %s\n\n", sub)
		printCrawlUsage()
		os.Exit(1)
	}
}

func printCrawlUsage() {
	fmt.Println(`Usage: qmax crawl <subcommand> [flags]

Subcommands:
  start      Start an AI-powered crawl to discover and generate tests
  status     Check crawl job status
  results    Get results of a completed crawl
  jobs       List recent crawl jobs

Examples:
  qmax crawl start --project-id 42 --url https://app.example.com
  qmax crawl start --project-id 42 --url https://app.example.com --depth 5 --pages 20
  qmax crawl status --crawl-id abc123
  qmax crawl results --crawl-id abc123
  qmax crawl jobs --limit 10`)
}

// --- crawl start ---

func cmdCrawlStart(args []string) {
	fs := flag.NewFlagSet("crawl start", flag.ExitOnError)
	projectID := fs.Int("project-id", 0, "Project ID (required)")
	url := fs.String("url", "", "URL to crawl (required)")
	depth := fs.Int("depth", 3, "Maximum crawl depth")
	pagesLimit := fs.Int("pages", 10, "Maximum number of pages to crawl")
	testType := fs.String("test-type", "e2e", "Test type: e2e, functional, ui, integration")
	instructions := fs.String("instructions", "", "Custom AI instructions for the crawl")
	wait := fs.Bool("wait", false, "Wait for crawl to complete")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}
	if *url == "" {
		fmt.Fprintln(os.Stderr, "Error: --url is required")
		os.Exit(1)
	}

	validTestTypes := map[string]bool{"e2e": true, "functional": true, "ui": true, "integration": true}
	if !validTestTypes[*testType] {
		fmt.Fprintf(os.Stderr, "Error: --test-type must be one of: e2e, functional, ui, integration\n")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	payload := map[string]interface{}{
		"project_id":  *projectID,
		"url":         *url,
		"depth":       *depth,
		"pages_limit": *pagesLimit,
		"test_type":   *testType,
		"framework":   "playwright",
	}
	if *instructions != "" {
		payload["custom_instructions"] = *instructions
	}

	body := authPost(cfg, fmt.Sprintf("%s/api/ai-crawl/start", apiURL), payload)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		CrawlID   string `json:"crawl_id"`
		ProjectID int    `json:"project_id"`
		Status    string `json:"status"`
		Message   string `json:"message"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Crawl started: %s\n", resp.CrawlID)
	fmt.Printf("URL: %s\n", *url)
	fmt.Printf("Project: #%d\n", *projectID)

	if *wait {
		fmt.Println("\nWaiting for completion...")
		pollCrawl(cfg, apiURL, resp.CrawlID, *jsonOut)
	} else {
		fmt.Printf("\nCheck status: qmax crawl status --crawl-id %s\n", resp.CrawlID)
	}
}

// --- crawl status ---

func cmdCrawlStatus(args []string) {
	fs := flag.NewFlagSet("crawl status", flag.ExitOnError)
	crawlID := fs.String("crawl-id", "", "Crawl job ID (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *crawlID == "" {
		fmt.Fprintln(os.Stderr, "Error: --crawl-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	body := authGet(cfg, fmt.Sprintf("%s/api/ai-crawl/status/%s", apiURL, *crawlID))

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success      bool    `json:"success"`
		Status       string  `json:"status"`
		Phase        string  `json:"phase"`
		Progress     float64 `json:"progress"`
		PagesVisited int     `json:"pages_visited"`
		Message      string  `json:"message"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Crawl: %s\n", *crawlID)
	fmt.Printf("Status: %s\n", resp.Status)
	if resp.Phase != "" {
		fmt.Printf("Phase: %s\n", resp.Phase)
	}
	if resp.Progress > 0 {
		fmt.Printf("Progress: %.0f%%\n", resp.Progress*100)
	}
	if resp.PagesVisited > 0 {
		fmt.Printf("Pages visited: %d\n", resp.PagesVisited)
	}
	if resp.Message != "" {
		fmt.Printf("Message: %s\n", resp.Message)
	}
}

// --- crawl results ---

func cmdCrawlResults(args []string) {
	fs := flag.NewFlagSet("crawl results", flag.ExitOnError)
	crawlID := fs.String("crawl-id", "", "Crawl job ID (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *crawlID == "" {
		fmt.Fprintln(os.Stderr, "Error: --crawl-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	body := authGet(cfg, fmt.Sprintf("%s/api/ai-crawl/results/%s", apiURL, *crawlID))

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success bool `json:"success"`
		Results struct {
			TestCases []struct {
				ID    json.Number `json:"id"`
				Title string      `json:"title"`
			} `json:"test_cases"`
			Scripts []struct {
				ID   json.Number `json:"id"`
				Name string      `json:"name"`
			} `json:"scripts"`
			PagesVisited int `json:"pages_visited"`
		} `json:"results"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	mustUnmarshal(body, &resp)

	if !resp.Success {
		fmt.Printf("Status: %s\n", resp.Status)
		if resp.Message != "" {
			fmt.Printf("Message: %s\n", resp.Message)
		}
		os.Exit(1)
	}

	fmt.Printf("Crawl: %s — COMPLETED\n\n", *crawlID)

	if len(resp.Results.TestCases) > 0 {
		fmt.Printf("Test cases generated: %d\n", len(resp.Results.TestCases))
		for _, tc := range resp.Results.TestCases {
			title := tc.Title
			if len(title) > 70 {
				title = title[:67] + "..."
			}
			fmt.Printf("  #%-6s  %s\n", tc.ID.String(), title)
		}
	}

	if len(resp.Results.Scripts) > 0 {
		fmt.Printf("\nScripts generated: %d\n", len(resp.Results.Scripts))
		for _, s := range resp.Results.Scripts {
			name := s.Name
			if len(name) > 70 {
				name = name[:67] + "..."
			}
			fmt.Printf("  #%-6s  %s\n", s.ID.String(), name)
		}

		// Build script IDs for the run hint
		var ids []string
		for _, s := range resp.Results.Scripts {
			ids = append(ids, s.ID.String())
		}
		if len(ids) > 0 && len(ids) <= 10 {
			fmt.Printf("\nRun all: qmax test run --script-ids %s\n", joinStrings(ids, ","))
		}
	}
}

// --- crawl jobs ---

func cmdCrawlJobs(args []string) {
	fs := flag.NewFlagSet("crawl jobs", flag.ExitOnError)
	limit := fs.Int("limit", 20, "Max jobs to return")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	body := authGet(cfg, fmt.Sprintf("%s/api/ai-crawl/jobs?limit=%d", apiURL, *limit))

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success bool `json:"success"`
		Count   int  `json:"count"`
		Jobs    []struct {
			CrawlID   string `json:"crawl_id"`
			Status    string `json:"status"`
			URL       string `json:"url"`
			ProjectID int    `json:"project_id"`
			CreatedAt string `json:"created_at"`
		} `json:"jobs"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Crawl jobs: %d\n\n", resp.Count)

	if resp.Count == 0 {
		fmt.Println("No crawl jobs found.")
		return
	}

	fmt.Printf("%-20s  %-12s  %-8s  %s\n", "Crawl ID", "Status", "Project", "URL")
	fmt.Printf("%-20s  %-12s  %-8s  %s\n", "--------------------", "------------", "--------", "---")
	for _, j := range resp.Jobs {
		crawlID := j.CrawlID
		if len(crawlID) > 20 {
			crawlID = crawlID[:17] + "..."
		}
		urlStr := j.URL
		if len(urlStr) > 50 {
			urlStr = urlStr[:47] + "..."
		}
		fmt.Printf("%-20s  %-12s  %-8d  %s\n", crawlID, truncate(j.Status, 12), j.ProjectID, urlStr)
	}
}

// --- polling ---

func pollCrawl(cfg *Config, apiURL, crawlID string, jsonOut bool) {
	url := fmt.Sprintf("%s/api/ai-crawl/status/%s", apiURL, crawlID)

	maxAttempts := 120 // 10 minutes at 5s intervals
	for i := 0; i < maxAttempts; i++ {
		body := authGet(cfg, url)

		var resp struct {
			Status       string  `json:"status"`
			Phase        string  `json:"phase"`
			Progress     float64 `json:"progress"`
			PagesVisited int     `json:"pages_visited"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing status: %v\n", err)
			os.Exit(1)
		}

		if jsonOut {
			fmt.Println(string(body))
		}

		switch resp.Status {
		case "completed":
			if !jsonOut {
				fmt.Printf("Status: COMPLETED (pages visited: %d)\n", resp.PagesVisited)
				fmt.Printf("\nGet results: qmax crawl results --crawl-id %s\n", crawlID)
			}
			return
		case "failed":
			if !jsonOut {
				fmt.Printf("Status: FAILED\n")
			}
			os.Exit(1)
		default:
			if !jsonOut && i%6 == 0 { // Print every 30s
				msg := fmt.Sprintf("  ...%s", resp.Status)
				if resp.Phase != "" {
					msg += fmt.Sprintf(" (%s)", resp.Phase)
				}
				if resp.Progress > 0 {
					msg += fmt.Sprintf(" %.0f%%", resp.Progress*100)
				}
				fmt.Println(msg)
			}
		}

		time.Sleep(5 * time.Second)
	}

	fmt.Fprintln(os.Stderr, "Timed out waiting for crawl to complete")
	os.Exit(1)
}

// joinStrings joins a string slice with a separator.
func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
