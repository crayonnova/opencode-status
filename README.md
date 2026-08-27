# opencode-status

Single-binary monitor for **opencode free models**. Polls OpenRouter + models.dev every
5 min, records availability + uptime to SQLite, renders history in a TUI, and serves
JSON over HTTP for dashboards.

```
STATUS     PROVIDER         MODEL                          UPTIME  SAMPLES  SPARKLINE
● FREE     nvidia           nvidia/nemotron-3.5-lightning  100.0%    4      ████████████████···
● FREE     cohere           cohere/north-mini-code:free    100.0%    4      ████████████████···
● FREE     openrouter       minimax/minimax-m3:free        100.0%    4      ████████████████···
✗ GONE     poolside         poolside/laguna-s-2.1:free      25.0%    4      ████░░░░░░░░░░░░░░
· paid      anthropic        claude-sonnet-4-5                -      0      ················
```

## Features

- **TUI** — bubbletea, sparklines per model, free models highlighted green, disappeared models flagged red.
- **HTTP API** — `/healthz`, `/api/snapshot`, `/api/history?model=…&window=24h`, `/api/stats`.
- **History** — 30 d × 5 min granularity (~8.6k samples/model), SQLite WAL, automatic daily prune.
- **Source of truth** — OpenRouter `/api/v1/models` (live catalog). Optional `--openrouter-key` improves rate-limit.
- **Cross-reference** — also pulls models.dev for paid/free cost checks.

## Install

### Nix package

```nix
# flake.nix
inputs.opencode-status.url = "github:nova/opencode-status";

outputs = { self, nixpkgs, opencode-status }: {
  environment.systemPackages = [ opencode-status.packages.${system}.default ];
};
```

### NixOS module

```nix
imports = [ inputs.opencode-status.nixosModules.default ];

services.opencode-status = {
  enable = true;
  webAddr = ":8080";          # exposes JSON API
  pollInterval = "5m";
  retentionDays = 30;
  openFirewall = true;        # optional
  # openRouterKeyFile = "/run/secrets/openrouter";  # optional
};
systemd services auto-start on boot, runs as `opencode-status` system user, DB at
`/var/lib/opencode-status/history.db`.

### Standalone Docker

```bash
cd deploy
docker compose up -d
# open http://localhost:8080/api/snapshot
```

The container runs as uid=1000 with a `read_only` rootfs + tmpfs, persistent `data/` volume, and drops all caps.

## Run locally

```bash
go run ./cmd/opencode-status --interval 30s
# or with explicit config:
go run ./cmd/opencode-status --no-tui --web :8080 --db ./history.db
```

## HTTP API

| Path | Method | Notes |
|---|---|---|
| `/healthz` | GET | `{"status":"ok"}` |
| `/api/stats` | GET | model count, check count, window |
| `/api/snapshot` | GET | full list with uptime%, last-checked |
| `/api/history?model=<id>&window=24h&buckets=24` | GET | raw points for charting |

## CLI flags

```
--db PATH              SQLite db path (default /var/lib/opencode-status/history.db)
--interval DURATION    poll cadence (default 5m)
--retention-days N     keep N days of history (default 30)
--web ADDR             HTTP listen (default :8080; empty disables)
--no-tui               daemon mode (HTTP only)
--no-web               TUI only, no HTTP
--show-paid            also track paid models
--openrouter-key KEY   optional, raises rate-limit
--log-json             machine-readable logs
```

## Architecture

```
┌──────────┐    GET /api/v1/models   ┌────────────┐
│  Poller  │ ──────────────────────▶ │ OpenRouter │
│ (5m)     │ ──────────────────────▶ │ models.dev │
└────┬─────┘                          └────────────┘
     │ upsert + record_check
     ▼
┌──────────┐
│  SQLite  │ ◀── /api/history, /api/snapshot, /api/stats
└──────────┘
     ▲
     │ read
┌────┴─────┐    ┌────────┐
│  TUI     │    │  HTTP  │
│(terminal)│    | :8080  │
└──────────┘    └────────┘
```

## First-time Nix build: vendorHash

`flake.nix` and `nix/default.nix` ship with a placeholder
`vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="`.
On your first `nix build`, nix will print the correct hash. Patch it into both
files, commit, rebuild — reproducibility locked.

```bash
nix build .#default 2>&1 | grep "got:    sha256-" | head -1
# copy the hash after "got:    " into vendorHash
```

Alternatively, pre-vendor locally and set `vendorHash = "";` (disables
reproducibility, not recommended).

## License

MIT
