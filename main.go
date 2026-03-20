package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "run":
		cmdRun(os.Args[2:])
	case "login":
		cmdLogin(os.Args[2:])
	case "capture":
		cmdCapture(os.Args[2:])
	case "projects":
		cmdProjects(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "token":
		cmdToken(os.Args[2:])
	case "logout":
		cmdLogout(os.Args[2:])
	case "test":
		cmdTest(os.Args[2:])
	case "crawl":
		cmdCrawl(os.Args[2:])
	case "repo":
		cmdRepo(os.Args[2:])
	case "import":
		cmdImport(os.Args[2:])
	case "pr":
		cmdPR(os.Args[2:])
	case "sast":
		cmdSast(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	case "version", "--version", "-v":
		fmt.Printf("qmax v%s\n", Version)
	default:
		// Backward compat: if first arg starts with "-", treat as `run` with all args
		// e.g. qmax --cloud-url URL → qmax run --cloud-url URL
		if strings.HasPrefix(cmd, "-") {
			cmdRun(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Printf(`qmax v%s — QualityMax Local Agent CLI

Usage:
  qmax <command> [flags]

Commands:
  run        Start the agent daemon (poll for test assignments)
  login      Authenticate with QualityMax via browser OAuth
  capture    Launch Chrome, capture cookies, upload as auth data
  projects   List available projects
  test       Test operations (cases, scripts, run, generate, status)
  crawl      AI-powered crawl (start, status, results, jobs)
  repo       Repository operations (list, review, coverage, quality)
  import     Import repositories or documents for test generation
  pr         Create pull requests with generated tests
  status     Show current auth and agent status
  token      Print the saved OAuth token to stdout
  logout     Remove saved credentials
  sast       SAST security scanning (verify, install, scan, setup)

Flags:
  --help     Show this help message
  --version  Show version

Backward compatibility:
  qmax --cloud-url URL   (equivalent to: qmax run --cloud-url URL)
`, Version)
}
