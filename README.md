# ScanRay Pupp

Remote scanning agent for [ScanRay Console](https://github.com/NCLGISA/The-ScanRay-Console). Deploys to airgapped or isolated networks and streams asset discovery and vulnerability scan results back to the Console in real time via WebSocket over port 443.

## How It Works

```
┌──────────────────────┐          WebSocket (wss://443)          ┌──────────────────────┐
│   Remote Network     │  ◄──── Cloudflare Tunnel ────►          │   ScanRay Console    │
│                      │                                          │                      │
│  ┌────────────────┐  │   heartbeat ──────────────────►          │  ┌────────────────┐  │
│  │  ScanRay Pupp  │  │   scan results ───────────────►          │  │   API Server   │  │
│  │                │  │   ◄─────────────── scan commands          │  │                │  │
│  │  • Scanray     │  │                                          │  │  • WebSocket   │  │
│  │  • Nuclei      │  │                                          │  │  • REST API    │  │
│  └────────────────┘  │                                          │  │  • Database    │  │
│                      │                                          │  └────────────────┘  │
└──────────────────────┘                                          └──────────────────────┘
```

- **No inbound ports required** on the remote network
- Pupp initiates outbound WebSocket to Console via Cloudflare Tunnel
- Console pushes scan commands; Pupp streams results back
- 30-second heartbeat with CPU, memory, disk, and uptime metrics

## Prerequisites

1. A running ScanRay Console instance
2. A Cloudflare Tunnel configured to route WebSocket traffic to the Console API
3. A registered Pupp (from the Console UI under **Pupps** page) to obtain the auth token

## Installation

### Docker (Recommended)

```bash
docker volume create scanray-pupp-data
docker run -d \
  --name scanray-pupp \
  --restart unless-stopped \
  --cap-add NET_RAW \
  -v scanray-pupp-data:/opt/scanray/data \
  -e PUPP_ID=<YOUR_PUPP_UUID> \
  -e PUPP_AUTH_TOKEN=<YOUR_TOKEN> \
  -e PUPP_CONSOLE_URL=wss://<YOUR_DOMAIN>/api/v1/ws/pupp/<YOUR_PUPP_UUID> \
  ghcr.io/nclgisa/scanray-pupp:latest
```

The `scanray-pupp-data` named volume persists Nuclei templates across container recreations. See [Nuclei Templates](#nuclei-templates) below for details.

### Build from Source (Docker)

```bash
git clone https://github.com/NCLGISA/ScanRay-Pupp.git
cd ScanRay-Pupp
docker build -t scanray-pupp .
docker volume create scanray-pupp-data
docker run -d \
  --name scanray-pupp \
  --restart unless-stopped \
  --cap-add NET_RAW \
  -v scanray-pupp-data:/opt/scanray/data \
  -e PUPP_ID=<YOUR_PUPP_UUID> \
  -e PUPP_AUTH_TOKEN=<YOUR_TOKEN> \
  -e PUPP_CONSOLE_URL=wss://<YOUR_DOMAIN>/api/v1/ws/pupp/<YOUR_PUPP_UUID> \
  scanray-pupp
```

### Bare-metal (Linux)

```bash
curl -sSL https://raw.githubusercontent.com/NCLGISA/ScanRay-Pupp/main/install.sh | \
  bash -s -- \
    --id <YOUR_PUPP_UUID> \
    --token <YOUR_TOKEN> \
    --url wss://<YOUR_DOMAIN>/api/v1/ws/pupp/<YOUR_PUPP_UUID>
```

This installs Scanray, Nuclei, and the Pupp agent to `/opt/scanray/` and creates a `scanray-pupp` systemd service.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PUPP_ID` | Yes | - | Pupp UUID from Console registration |
| `PUPP_AUTH_TOKEN` | Yes | - | Auth token from Console registration |
| `PUPP_CONSOLE_URL` | Yes | - | WebSocket URL (`wss://domain/api/v1/ws/pupp/{id}`) |
| `SCANRAY_BINARY` | No | `/opt/scanray/bin/scanray` | Path to Scanray binary |
| `NUCLEI_BINARY` | No | `/opt/scanray/bin/nuclei` | Path to Nuclei binary |
| `PUPP_DATA_DIR` | No | `/opt/scanray/data` | Working directory for scan data |
| `NUCLEI_TEMPLATES_DIR` | No | `/opt/scanray/data/nuclei-templates` | Directory holding Nuclei vuln templates (persisted on the data volume) |

## Nuclei Templates

Nuclei templates live on the persistent data volume at `$NUCLEI_TEMPLATES_DIR` (default `/opt/scanray/data/nuclei-templates`). They are **not** baked into the container image, so they survive container recreation and upgrades.

- **First-run seed**: The container entrypoint detects an empty templates directory and runs `nuclei -update-templates -ud $NUCLEI_TEMPLATES_DIR` once before starting the agent. This fetches the latest official templates from `github.com/projectdiscovery/nuclei-templates`.
- **24-hour auto-update**: While the agent is running, it refreshes templates every 24 hours from the official GitHub repo — no manual intervention needed.
- **On-demand update**: The ScanRay Console can also trigger an immediate refresh by sending the `update_templates` WebSocket command.
- **Vuln scans**: Each scan invokes Nuclei with `-t $NUCLEI_TEMPLATES_DIR` so it always uses the templates on the volume.

For bare-metal installs, `install.sh` seeds templates into `${INSTALL_DIR}/data/nuclei-templates` and the agent's 24h loop keeps them current automatically.

## Management

```bash
# Docker
docker logs -f scanray-pupp
docker restart scanray-pupp

# Bare-metal (systemd)
sudo systemctl status scanray-pupp
sudo journalctl -u scanray-pupp -f
sudo systemctl restart scanray-pupp

# Configuration (bare-metal)
sudo nano /etc/scanray-pupp.env
sudo systemctl restart scanray-pupp
```

## Architecture

The Pupp agent is written in Go and consists of:

| Package | Description |
|---------|-------------|
| `cmd/pupp` | Entry point |
| `internal/config` | Environment-based configuration |
| `internal/version` | Pupp CalVer constant (independent of Console) |
| `internal/ws` | WebSocket client with auto-reconnect and exponential backoff |
| `internal/agent` | Main agent loop, heartbeat, scan execution, result streaming, 24h Nuclei template refresh |

### WebSocket Message Types

**Pupp → Console:**
| Type | Description |
|------|-------------|
| `register` | System info + scanner versions on connect |
| `heartbeat` | Health metrics every 30 seconds |
| `scan_result_asset` | Asset discovery results (Scanray JSON) |
| `scan_result_vuln` | Vulnerability findings (Nuclei JSONL, batched) |
| `scan_status` | Scan lifecycle events (running, completed, failed) |

**Console → Pupp:**
| Type | Description |
|------|-------------|
| `start_scan` | Trigger asset or vulnerability scan with targets |
| `cancel_scan` | Kill active scan process |
| `update_templates` | Update Nuclei templates |
| `ping` | Connectivity check |

## Building

```bash
# Build Go binary
go build -o pupp ./cmd/pupp

# Build Docker image
docker build -t scanray-pupp .

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o pupp-linux-amd64 ./cmd/pupp
GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o pupp-linux-arm64 ./cmd/pupp
```

## License

Apache License 2.0 - see [LICENSE](LICENSE)
