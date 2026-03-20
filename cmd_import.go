package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func cmdImport(args []string) {
	if len(args) < 1 {
		printImportUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "repo":
		cmdImportRepo(args[1:])
	case "doc":
		cmdImportDoc(args[1:])
	case "help", "--help", "-h":
		printImportUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown import subcommand: %s\n\n", sub)
		printImportUsage()
		os.Exit(1)
	}
}

func printImportUsage() {
	fmt.Println(`Usage: qmax import <subcommand> [flags]

Subcommands:
  repo       Import a GitHub/GitLab repository for analysis
  doc        Import test cases from text content (requirements, specs, etc.)

Examples:
  qmax import repo --url https://github.com/user/repo --project-id 42
  qmax import repo --url https://github.com/user/repo --create-project --project-name "My App"
  qmax import doc --project-id 42 --text "User can login with email and password"
  qmax import doc --project-id 42 --file requirements.txt`)
}

// --- import repo ---

func cmdImportRepo(args []string) {
	fs := flag.NewFlagSet("import repo", flag.ExitOnError)
	repoURL := fs.String("url", "", "Repository URL (required)")
	projectID := fs.Int("project-id", 0, "Existing project ID to associate with")
	createProject := fs.Bool("create-project", false, "Create a new project for this repo")
	projectName := fs.String("project-name", "", "Name for the new project (with --create-project)")
	branch := fs.String("branch", "", "Branch to import (defaults to repo default)")
	consent := fs.String("consent", "opt_out", "Training consent: opt_in or opt_out")
	baseURL := fs.String("base-url", "", "Base URL for testing (saved as project main_url)")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *repoURL == "" {
		fmt.Fprintln(os.Stderr, "Error: --url is required")
		os.Exit(1)
	}
	if !strings.HasPrefix(*repoURL, "https://") && !strings.HasPrefix(*repoURL, "http://") {
		fmt.Fprintln(os.Stderr, "Error: --url must be a valid HTTP(S) URL")
		os.Exit(1)
	}
	if *projectID == 0 && !*createProject {
		fmt.Fprintln(os.Stderr, "Error: --project-id or --create-project is required")
		os.Exit(1)
	}
	if *createProject && *projectName == "" {
		fmt.Fprintln(os.Stderr, "Error: --project-name is required with --create-project")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	payload := map[string]interface{}{
		"repo_url":          *repoURL,
		"training_consent":  *consent,
		"create_new_project": *createProject,
	}
	if *projectID != 0 {
		payload["project_id"] = *projectID
	}
	if *projectName != "" {
		payload["new_project_name"] = *projectName
	}
	if *branch != "" {
		payload["branch"] = *branch
	}
	if *baseURL != "" {
		payload["base_url"] = *baseURL
	}

	body := authPost(cfg, fmt.Sprintf("%s/api/repositories/import", apiURL), payload)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success    bool   `json:"success"`
		Message    string `json:"message"`
		Project    *struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
			Key  string      `json:"key"`
		} `json:"project"`
		Repository struct {
			ID            json.Number `json:"id"`
			ProjectID     json.Number `json:"project_id"`
			RepoURL       string      `json:"repo_url"`
			DefaultBranch string      `json:"default_branch"`
			Summary       string      `json:"summary"`
		} `json:"repository"`
	}
	mustUnmarshal(body, &resp)

	fmt.Printf("Repository imported successfully\n")
	fmt.Printf("  Repo ID: %s\n", resp.Repository.ID.String())
	fmt.Printf("  URL: %s\n", resp.Repository.RepoURL)
	fmt.Printf("  Branch: %s\n", resp.Repository.DefaultBranch)

	if resp.Project != nil {
		fmt.Printf("  Project: %s (#%s)\n", resp.Project.Name, resp.Project.ID.String())
	} else {
		fmt.Printf("  Project ID: %s\n", resp.Repository.ProjectID.String())
	}

	if resp.Repository.Summary != "" {
		summary := resp.Repository.Summary
		if len(summary) > 200 {
			summary = summary[:197] + "..."
		}
		fmt.Printf("\nSummary: %s\n", summary)
	}

	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  qmax repo review --repo-id %s --wait\n", resp.Repository.ID.String())
	fmt.Printf("  qmax repo coverage --repo-id %s\n", resp.Repository.ID.String())
}

// --- import doc ---

func cmdImportDoc(args []string) {
	fs := flag.NewFlagSet("import doc", flag.ExitOnError)
	projectID := fs.Int("project-id", 0, "Project ID (required)")
	text := fs.String("text", "", "Text content to import")
	file := fs.String("file", "", "File path to read text from")
	sourceName := fs.String("source", "", "Name for the import source")
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	if *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}

	textContent := *text
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		textContent = string(data)
	}

	if textContent == "" {
		fmt.Fprintln(os.Stderr, "Error: --text or --file is required")
		os.Exit(1)
	}

	cfg := mustLoadConfig()
	apiURL := cfg.GetAPIBaseURL()

	payload := map[string]interface{}{
		"project_id":   *projectID,
		"text_content": textContent,
	}
	if *sourceName != "" {
		payload["source_name"] = *sourceName
	}

	body := authPost(cfg, fmt.Sprintf("%s/api/import/document/text", apiURL), payload)

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var resp struct {
		Success        bool   `json:"success"`
		Message        string `json:"message"`
		ExtractedCount int    `json:"extracted_count"`
		TestCases      []struct {
			ID    json.Number `json:"id"`
			Title string      `json:"title"`
		} `json:"test_cases"`
	}
	mustUnmarshal(body, &resp)

	if !resp.Success {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Message)
		os.Exit(1)
	}

	fmt.Printf("Imported %d test cases from document\n\n", resp.ExtractedCount)

	for _, tc := range resp.TestCases {
		title := tc.Title
		if len(title) > 70 {
			title = title[:67] + "..."
		}
		fmt.Printf("  #%-6s  %s\n", tc.ID.String(), title)
	}

	if resp.ExtractedCount > 0 {
		fmt.Printf("\nGenerate code: qmax test generate --test-case-id <ID>\n")
	}
}
