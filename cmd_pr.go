package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func cmdPR(args []string) {
	if len(args) < 1 {
		printPRUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "create":
		cmdPRCreate(args[1:])
	case "help", "--help", "-h":
		printPRUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown pr subcommand: %s\n\n", sub)
		printPRUsage()
		os.Exit(1)
	}
}

func printPRUsage() {
	fmt.Println(`Usage: qmax pr <subcommand> [flags]

Subcommands:
  create     Create a PR with generated test suite on the repository

Examples:
  qmax pr create --repo-id 101 --project-id 42`)
}

// --- pr create ---

func cmdPRCreate(args []string) {
	fs := flag.NewFlagSet("pr create", flag.ExitOnError)
	repoID := fs.Int("repo-id", 0, "Repository ID (required)")
	projectID := fs.Int("project-id", 0, "Project ID (required)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *repoID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --repo-id is required")
		os.Exit(1)
	}
	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	url := fmt.Sprintf("%s/api/repositories/%d/create-test-suite-pr?project_id=%d",
		apiURL, *repoID, *projectID)

	body := authPost(cfg, url, map[string]interface{}{})

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success    bool     `json:"success"`
		PRURL      string   `json:"pr_url"`
		PRNumber   int      `json:"pr_number"`
		Branch     string   `json:"branch"`
		TestCount  int      `json:"test_count"`
		Categories []string `json:"categories"`
		TotalFiles int      `json:"total_files"`
	}
	mustUnmarshal(body, &resp)

	if !resp.Success {
		fmt.Fprintln(os.Stderr, "Error: PR creation failed")
		os.Exit(1)
	}

	fmt.Printf("Pull request created!\n\n")
	fmt.Printf("  PR: %s\n", resp.PRURL)
	fmt.Printf("  Branch: %s\n", resp.Branch)
	fmt.Printf("  Tests: %d files\n", resp.TestCount)
	fmt.Printf("  Total files: %d\n", resp.TotalFiles)

	if len(resp.Categories) > 0 {
		fmt.Printf("  Categories: %s\n", strings.Join(resp.Categories, ", "))
	}
}
