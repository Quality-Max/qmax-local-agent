# qmax

Cross-platform CLI for running [QualityMax](https://qualitymax.io) Playwright tests locally.

- **Run** as a daemon to poll and execute Playwright tests from QualityMax cloud
- **Crawl** websites behind firewalls, VPNs, and localhost with AI-powered discovery
- **Capture** browser cookies for authenticated test scenarios
- **Authenticate** via browser-based OAuth login
- **Manage** projects and credentials locally

Single binary, no runtime dependencies (Node.js/npm required only for test execution).

## Install

### One-liner (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/Quality-Max/qamax-local-agent/main/install.sh | bash
```

This detects your OS and architecture, downloads the correct binary from GitHub Releases, and installs it to `~/.qmax/`.

To install a specific version:

```bash
QAMAX_VERSION=v3.0.0 curl -fsSL https://raw.githubusercontent.com/Quality-Max/qamax-local-agent/main/install.sh | bash
```

### Download binary manually

Download the latest release for your platform from [Releases](https://github.com/Quality-Max/qamax-local-agent/releases/latest):

```bash
# macOS Apple Silicon
curl -fsSL -o qmax https://github.com/Quality-Max/qamax-local-agent/releases/latest/download/qmax-darwin-arm64

# macOS Intel
curl -fsSL -o qmax https://github.com/Quality-Max/qamax-local-agent/releases/latest/download/qmax-darwin-amd64

# Linux x86_64
curl -fsSL -o qmax https://github.com/Quality-Max/qamax-local-agent/releases/latest/download/qmax-linux-amd64

chmod +x qmax
sudo mv qmax /usr/local/bin/
```

### Build from source

Requires Go 1.23+:

```bash
git clone https://github.com/Quality-Max/qamax-local-agent.git
cd qamax-local-agent
make build
```

Cross-compile for all platforms:

```bash
make build-all
```

## Quick Start

```bash
qmax login                                              # Authenticate via browser
qmax projects                                           # List your projects
qmax run --cloud-url https://app.qualitymax.io          # Start the agent daemon
```

## Commands

### `login`

Authenticate with QualityMax via browser OAuth. Opens your browser and saves the token to `~/.qamax/config.json`.

```bash
qmax login                        # Default (port 9876)
qmax login --port 8080            # Custom callback port
qmax login --api-url URL          # Custom QualityMax URL
```

### `run`

Start the agent daemon to poll for and execute test assignments and AI crawl discovery sessions.

```bash
qmax run --cloud-url https://app.qualitymax.io
qmax run --cloud-url https://app.qualitymax.io --registration-secret SECRET
qmax run --poll-interval 10 --heartbeat-interval 30
```

After the first successful registration, credentials are saved. Subsequent runs use saved values as defaults.

**Backward compatibility** — the old flag-based invocation still works:

```bash
qmax --cloud-url https://app.qualitymax.io --registration-secret SECRET
```

#### AI Crawl Discovery (v3.0)

When running, the agent automatically polls for **AI crawl discovery sessions** alongside test assignments. When QualityMax assigns a crawl:

1. The agent launches a local Chrome browser (via [chromedp](https://github.com/chromedp/chromedp))
2. Navigates to the target URL — including sites behind firewalls, VPNs, or localhost
3. Captures page snapshots (DOM elements, forms, selectors, screenshots)
4. Sends snapshots to the QualityMax server for LLM-powered navigation decisions
5. Executes the returned actions (click, fill, select) and repeats
6. When discovery completes, QualityMax generates Playwright test code from the captured flow

This enables AI-powered test generation for internal applications that the cloud cannot reach.

Set `QAMAX_CRAWL_HEADED=true` to see the browser during crawl sessions (useful for debugging):

```bash
QAMAX_CRAWL_HEADED=true qmax run --cloud-url https://app.qualitymax.io
```

### `capture`

Launch Chrome, navigate to a URL, wait for manual login, then capture all cookies and localStorage, and upload them as authentication data.

```bash
qmax capture --url https://example.com --project-id ID --name "Production Auth"
qmax capture --url https://example.com --project-id ID --name "Staging" --output cookies.json
```

Captures are stored as Playwright-compatible storage state JSON. Requires prior `qmax login` and Google Chrome installed.

### `projects`

List available projects.

```bash
qmax projects
```

### `status`

Show current authentication and agent registration status.

```bash
qmax status
```

### `token`

Print the saved OAuth token to stdout (useful for piping).

```bash
qmax token
qmax token | pbcopy    # Copy to clipboard on macOS
```

### `logout`

Remove saved credentials.

```bash
qmax logout
```

## Configuration

Config is stored at `~/.qamax/config.json` (mode `0600`):

```json
{
  "token": "eyJ...",
  "api_url": "https://app.qualitymax.io",
  "agent_id": "uuid",
  "api_key": "hex-key",
  "registration_secret": ""
}
```

| Field | Purpose |
|-------|---------|
| `token` | OAuth JWT from `login`, used by `capture` and `projects` |
| `api_url` | QualityMax server URL |
| `agent_id` / `api_key` | Agent daemon credentials, saved after first `run` registration |
| `registration_secret` | Server-side secret for agent registration |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `QAMAX_CRAWL_HEADED` | `false` | Set to `true` to show the browser during AI crawl sessions |

## Running as a Service

See [INSTALLATION.md](INSTALLATION.md) for macOS LaunchAgent and Linux systemd setup instructions.

## Prerequisites

| Requirement | Used by |
|-------------|---------|
| Node.js + npm | `run` (Playwright test execution) |
| Google Chrome | `capture` (cookie extraction), `run` (AI crawl discovery) |

## Security

- All communication uses HTTPS/TLS
- Config file permissions are restricted to `0600` (owner read/write only)
- Config directory permissions are `0700`
- HTTP response bodies are size-limited to prevent memory exhaustion
- Login callback validates request method and token length
- AI crawl sessions are authenticated via agent API key
- Crawl browser sessions have a 10-minute timeout
- HTTP retries use exponential backoff (3 attempts max)

## License

Apache-2.0 -- see [LICENSE](LICENSE).
