package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func cmdRepo(args []string) {
	if len(args) < 1 {
		printRepoUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "list":
		cmdRepoList(args[1:])
	case "review":
		cmdRepoReview(args[1:])
	case "coverage":
		cmdRepoCoverage(args[1:])
	case "quality":
		cmdRepoQuality(args[1:])
	case "help", "--help", "-h":
		printRepoUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown repo subcommand: %s\n\n", sub)
		printRepoUsage()
		os.Exit(1)
	}
}

func printRepoUsage() {
	fmt.Println(`Usage: qmax repo <subcommand> [flags]

Subcommands:
  list       List repositories for a project
  review     Start AI-powered code review
  coverage   Show test coverage analysis
  quality    Show quality signal snapshot

Examples:
  qmax repo list --project-id 42
  qmax repo review --repo-id 101
  qmax repo review --repo-id 101 --wait
  qmax repo coverage --repo-id 101
  qmax repo quality --repo-id 101`)
}

// --- repo list ---

func cmdRepoList(args []string) {
	fs := flag.NewFlagSet("repo list", flag.ExitOnError)
	projectID := fs.Int("project-id", 0, "Project ID (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	fs.Parse(args)

	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	body := authGet(cfg, fmt.Sprintf("%s/api/repositories/project/%d", apiURL, *projectID))

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success      bool `json:"success"`
		Count        int  `json:"count"`
		Repositories []struct {
			ID            json.Number `json:"id"`
			RepoURL       string      `json:"repo_url"`
			DefaultBranch string      `json:"default_branch"`
			Summary       string      `json:"summary"`
		} `json:"repositories"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Repositories: %d\n\n", resp.Count)

	if resp.Count == 0 {
		fmt.Println("No repositories found.")
		return
	}

	fmt.Printf("%-6s  %-12s  %s\n", "ID", "Branch", "URL")
	fmt.Printf("%-6s  %-12s  %s\n", "------", "------------", "---")
	for _, r := range resp.Repositories {
		fmt.Printf("%-6s  %-12s  %s\n", r.ID.String(), truncate(r.DefaultBranch, 12), r.RepoURL)
	}
}

// --- repo review ---

func cmdRepoReview(args []string) {
	fs := flag.NewFlagSet("repo review", flag.ExitOnError)
	repoID := fs.Int("repo-id", 0, "Repository ID (required)")
	wait := fs.Bool("wait", false, "Wait for review to complete")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	maxSuggestions := fs.Int("max-suggestions", 8, "Max review suggestions")
	scope := fs.String("scope", "all", "Suggestions scope: all, critical, security")
	fs.Parse(args)

	if *repoID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --repo-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	payload := map[string]interface{}{
		"chain_suggestions": true,
		"suggestions_scope": *scope,
		"max_suggestions":   *maxSuggestions,
	}

	body := authPost(cfg, fmt.Sprintf("%s/api/repositories/%d/ai-review", apiURL, *repoID), payload)

	if *jsonOut && !*wait {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		JobID     string `json:"job_id"`
		StatusURL string `json:"status_url"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("AI review started: %s\n", resp.JobID)

	if *wait {
		fmt.Println("Waiting for completion...")
		pollWorkflow(cfg, apiURL, resp.JobID, *jsonOut)
	} else {
		fmt.Printf("\nCheck status: qmax repo review-status --job-id %s\n", resp.JobID)
		fmt.Printf("Or get latest: qmax repo review-latest --repo-id %d\n", *repoID)
	}
}

// --- repo coverage ---

func cmdRepoCoverage(args []string) {
	fs := flag.NewFlagSet("repo coverage", flag.ExitOnError)
	repoID := fs.Int("repo-id", 0, "Repository ID (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	fs.Parse(args)

	if *repoID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --repo-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	body := authGet(cfg, fmt.Sprintf("%s/api/repositories/%d/coverage", apiURL, *repoID))

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success   bool        `json:"success"`
		Coverage  interface{} `json:"coverage"`
		ScannedAt string      `json:"scanned_at"`
	}
	mustUnmarshal(body, &resp)

	if resp.Coverage == nil {
		fmt.Println("No coverage data available.")
		fmt.Println("Run a repository analysis first to generate coverage data.")
		return
	}

	fmt.Printf("Coverage data (scanned: %s)\n\n", resp.ScannedAt)

	// Pretty-print coverage data
	coverageJSON, _ := json.MarshalIndent(resp.Coverage, "", "  ")
	fmt.Println(string(coverageJSON))
}

// --- repo quality ---

func cmdRepoQuality(args []string) {
	fs := flag.NewFlagSet("repo quality", flag.ExitOnError)
	repoID := fs.Int("repo-id", 0, "Repository ID (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	fs.Parse(args)

	if *repoID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --repo-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	body := authGet(cfg, fmt.Sprintf("%s/api/repositories/%d/quality", apiURL, *repoID))

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success   bool        `json:"success"`
		Quality   interface{} `json:"quality"`
		ScannedAt string      `json:"scanned_at"`
	}
	mustUnmarshal(body, &resp)

	if resp.Quality == nil {
		fmt.Println("No quality data available.")
		fmt.Println("Run a repository analysis first to generate quality data.")
		return
	}

	fmt.Printf("Quality signal (scanned: %s)\n\n", resp.ScannedAt)

	qualityJSON, _ := json.MarshalIndent(resp.Quality, "", "  ")
	fmt.Println(string(qualityJSON))
}

// --- workflow polling (used by review) ---

func pollWorkflow(cfg *Config, apiURL, jobID string, jsonOut bool) {
	url := fmt.Sprintf("%s/api/workflow/status/%s", apiURL, jobID)

	maxAttempts := 120 // 10 minutes at 5s intervals
	for i := 0; i < maxAttempts; i++ {
		body := authGet(cfg, url)

		var resp struct {
			Status   string      `json:"status"`
			Progress float64     `json:"progress"`
			Message  string      `json:"message"`
			Result   interface{} `json:"result"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing status: %v\n", err)
			os.Exit(1)
		}

		switch resp.Status {
		case "completed":
			if jsonOut {
				fmt.Println(string(body))
			} else {
				fmt.Println("Status: COMPLETED")
				if resp.Result != nil {
					resultJSON, _ := json.MarshalIndent(resp.Result, "", "  ")
					fmt.Println(string(resultJSON))
				}
			}
			return
		case "failed":
			if jsonOut {
				fmt.Println(string(body))
			} else {
				fmt.Printf("Status: FAILED\n")
				if resp.Message != "" {
					fmt.Printf("Error: %s\n", resp.Message)
				}
			}
			os.Exit(1)
		default:
			if !jsonOut && i%6 == 0 {
				msg := fmt.Sprintf("  ...%s", resp.Status)
				if resp.Progress > 0 {
					msg += fmt.Sprintf(" %.0f%%", resp.Progress*100)
				}
				if resp.Message != "" {
					msg += fmt.Sprintf(" — %s", resp.Message)
				}
				fmt.Println(msg)
			}
		}

		time.Sleep(5 * time.Second)
	}

	fmt.Fprintln(os.Stderr, "Timed out waiting for job to complete")
	os.Exit(1)
}
