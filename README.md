# ports

A terminal UI for managing listening ports. Built for developers who end up with invisible dev servers piling up — especially when using AI coding assistants that spawn `npm dev` / `nuxt dev` / `next dev` and never clean up.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)

## Features

- **Live dashboard** — auto-refreshes every 3s, shows all TCP listeners
- **Resource visibility** — CPU and memory per process, human-readable (M/G)
- **Orphan detection** — flags processes with no controlling terminal (⚠ orphan), common with AI-spawned dev servers
- **Bulk kill by project** — kill all processes from the same project directory in one keystroke
- **Kill all orphans** — clean up every forgotten server at once
- **Smart command shortening** — turns `/Users/you/.nvm/versions/node/v20/bin/node /path/to/project/node_modules/.bin/nuxt dev` into `nuxt`

## Install

```bash
go install github.com/gmagnanat/ports@latest
```

Or build from source:

```bash
git clone https://github.com/gmagnanat/ports.git
cd ports
go build -o ports .
cp ports ~/.local/bin/  # or anywhere in your PATH
```

## Usage

```bash
ports
```

## Keybindings

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate |
| `k` / `x` / `Del` | Kill selected process (SIGTERM) |
| `K` | Kill all processes from the same project |
| `O` | Kill all orphan processes |
| `r` | Force refresh |
| `q` / `Ctrl+C` | Quit |

## Why orphan detection matters

AI coding tools (Claude Code, Cursor, Copilot) routinely start dev servers in the background. When the session ends, the process keeps running with no terminal attached. Over a day you can accumulate 5+ identical servers eating CPU and RAM. The orphan flag (`⚠`) identifies these instantly so you can clean them up with a single `O`.

## Requirements

- macOS (uses `lsof` and `ps` — Linux support straightforward to add)
- Go 1.21+

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — table component
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling

## License

MIT
