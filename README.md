# Gitmera

A high-performance CLI orchestrator for managing multiple Git repositories simultaneously with an elegant terminal UI.

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)

## Overview

Gitmera allows you to execute Git commands across multiple repositories in parallel, making it ideal for managing monorepos, polyrepos, or fleets of microservices. It features an interactive TUI for real-time progress tracking and falls back to sequential logs in CI/non-interactive environments.

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Configuration](#configuration)
- [Architecture](#architecture)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Concurrent Execution** — Run Git operations across multiple repos simultaneously
- **Interactive TUI** — Real-time progress visualization with Bubble Tea
- **Non-Interactive Mode** — CI/CD friendly with plain sequential logs
- **Smart Configuration** — Auto-discovers `.gitmera.yaml` in the current directory
- **Cross-Platform** — Supports macOS and Linux

## Installation

### Quick Install (Linux & macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/raferreira96/gitmera/main/install.sh | sh
```

Or with `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/raferreira96/gitmera/main/install.sh | sh
```

This installs gitmera into `~/.gitmera/bin` and adds it to your `PATH` in `~/.bashrc`, `~/.zshrc`, and/or `~/.profile` (whichever already exist). Open a new terminal afterward, or `source` the relevant file, for `gitmera` to be available.

### Binary Releases

Download the latest release for your platform from the [Releases](https://github.com/raferreira96/gitmera/releases) page.

### Build from Source

```bash
git clone https://github.com/raferreira96/gitmera.git
cd gitmera
go install
```

## Quick Start

### 1. Create a configuration file

Create `.gitmera.yaml` in your working directory:

```yaml
version: "1"
projects:
  api:
    repo: https://github.com/your-org/api
    path: ./api
  web:
    repo: https://github.com/your-org/web
    path: ./web
  mobile:
    repo: https://github.com/your-org/mobile
    path: ./mobile
```

### 2. Validate configuration

```bash
gitmera
# or with explicit config
gitmera --config path/to/config.yaml
```

### 3. Clone all repositories

```bash
gitmera clone
```

### 4. Pull latest changes

```bash
gitmera pull
```

### 5. Checkout branches

```bash
gitmera checkout feature/new-feature
```

### 6. Push changes

```bash
gitmera push
```

## Usage

```
gitmera [command]
```

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to configuration file |
| `--verbose` | `-v` | Enable verbose output |
| `--no-color` | | Disable colored output |
| `--non-interactive` | | Disable TUI, use plain logs |
| `--plain` | | Alias for --non-interactive |

### Commands

| Command | Description |
|---------|-------------|
| `gitmera init` | Initialize a new configuration file |
| `gitmera clone` | Clone all configured repositories |
| `gitmera pull` | Pull latest changes from remote |
| `gitmera checkout` | Checkout branches across all repos |
| `gitmera push` | Push changes to remote |
| `gitmera status` | Show status of all repositories |
| `gitmera update` | Update gitmera to the latest GitHub release |

## Configuration

Gitmera uses YAML configuration. Default file names (in order of precedence):

1. `.gitmera.yaml`
2. `.gitmera.yml`

### Example Configuration

```yaml
repositories:
  - path: ./api
    url: https://github.com/org/api
    branch: main
  - path: ./web
    url: https://github.com/org/web
    branch: develop
  - path: ./shared-lib
    url: https://github.com/org/shared-lib
```

## Architecture

```
gitmera/
├── cmd/           # CLI commands (Cobra)
├── pkg/
│   ├── config/    # Configuration loading
│   ├── git/       # Git operations wrapper
│   ├── runner/    # Concurrent task execution
│   └── ui/        # TUI components (Bubble Tea)
└── main.go
```

## Development

### Requirements

- Go 1.26+
- Bubble Tea v2
- Lipgloss v2

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o gitmera .
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) for details.
