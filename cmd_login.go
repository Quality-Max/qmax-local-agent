package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	defaultCallbackPort = 9876
	loginTimeout        = 5 * time.Minute
	defaultAPIURL       = "https://app.qualitymax.io"
)

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	port := fs.Int("port", defaultCallbackPort, "Local callback server port")
	apiURL := fs.String("api-url", defaultAPIURL, "QualityMax app URL")
	fs.Parse(args)

	tokenCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token parameter", http.StatusBadRequest)
			return
		}

		// Basic token sanity: reject obviously invalid values
		if len(token) < 10 || len(token) > 8192 {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>QualityMax CLI Login</title></head>
<body style="font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f8f9fa;">
<div style="text-align: center; padding: 2rem; background: white; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">
<h1 style="color: #22c55e;">Login Successful</h1>
<p>You can close this window and return to the terminal.</p>
</div>
</body>
</html>`)

		tokenCh <- token
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot listen on %s: %v\n", addr, err)
		os.Exit(1)
	}

	server := &http.Server{Handler: mux}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	callbackURL := fmt.Sprintf("http://localhost:%d/callback", *port)
	loginURL := fmt.Sprintf("%s#/cli-login?callback=%s", *apiURL, callbackURL)

	fmt.Println("Opening browser for login...")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", loginURL)
	fmt.Println("Waiting for authentication...")

	openBrowser(loginURL)

	ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
	defer cancel()

	select {
	case token := <-tokenCh:
		server.Shutdown(context.Background())

		cfg, err := LoadConfig()
		if err != nil {
			cfg = &Config{}
		}
		cfg.Token = token
		if cfg.APIURL == "" {
			cfg.APIURL = *apiURL
		}
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}

		path, _ := ConfigPath()
		fmt.Printf("Login successful! Token saved to %s\n", path)

	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)

	case <-ctx.Done():
		server.Shutdown(context.Background())
		fmt.Fprintf(os.Stderr, "Error: login timed out after %s\n", loginTimeout)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		return
	}
	cmd.Start()
}
