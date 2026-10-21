# flexibee-mcp

A local **Model Context Protocol (MCP) server** for **ABRA Flexi** (formerly FlexiBee), enabling Claude Desktop, Claude Code, and OpenCode to read and write accounting data through a conversational interface.

## Quick start

```bash
# Build
make build

# Configure (add to your MCP client config)
FLEXIBEE_URL=https://your-server:5434
FLEXIBEE_USERNAME=mcpbot
FLEXIBEE_PASSWORD_FILE=~/.flexibee-password
FLEXIBEE_COMPANY=your-company-db-id
FLEXIBEE_AUTH_MODE=session
```

**Full tutorial**: [docs/tutorial-linux.md](docs/tutorial-linux.md) —
step-by-step setup for OpenCode, Claude Code, Claude Desktop, Codex CLI,
and Antigravity, including skill installation.

**Install skills automatically**:
```bash
bash scripts/install-skills.sh   # installs to OpenCode, Claude Code, Agent Skills, Codex
```

See [examples/](examples/) for ready-to-use config files for each client.

## Features

### 18 MCP tools

| Tool | Purpose |
|------|---------|
| `flexi_get_connection_info` | Verify connectivity, show active transport tier |
| `flexi_list_companies` | List available companies |
| `flexi_list_evidences` | Browse the evidence (table) catalog |
| `flexi_describe_evidence` | Schema: fields, relations, reports, actions |
| `flexi_query_records` | Filter, sort and page any evidence |
| `flexi_get_record` | Fetch a single record by id / code |
| `flexi_get_related_records` | Traverse evidence relations (line items, payments, …) |
| `flexi_sum_records` | Aggregate totals for document evidences |
| `flexi_list_actions` | List available business workflow actions |
| `flexi_get_report` | Download a report as base64 (PDF, XLSX, CSV, XML) |
| `flexi_list_attachments` | List attachments on a record |
| `flexi_get_attachment` | Download an attachment as base64 |
| `flexi_get_changes` | Incremental sync via the Changes API |
| `flexi_get_company_stats` | Company statistics (record counts, DB size) |
| `flexi_list_saved_queries` | List named saved queries |
| `flexi_run_saved_query` | Execute a named saved query |
| `flexi_upsert_record` | Create or update a record (dry_run support) |
| `flexi_delete_record` | Delete a record |
| `flexi_execute_action` | Fire a business action (lock, cancel, send, pay, …) |
| `flexi_get_guide` | Workflow guidance and API tips (8 topics) |

### 7 Agent Skills

| Skill | Purpose |
|-------|---------|
| `flexi-explore` | Guided discovery of companies, evidences and schema |
| `flexi-receivables` | Overdue/unpaid invoices, partial payments, currency, period |
| `flexi-invoice` | Create/update/preview invoices, dry-run, actions, PDF |
| `flexi-contacts` | Search, create and deduplicate contacts by ICO/DIC |
| `flexi-banking` | Bank statements, payment matching, cash movements |
| `flexi-inventory` | Stock cards, levels, movements, warehouses |
| `flexi-orders` | Sales/purchase orders, fulfilment, invoice conversion |

### Three-tier transport (no API licence required)

```
FLEXIBEE_AUTH_MODE=auto (default)
  │
  ├─ 1. API (Basic Auth REST)      – requires rest-api-rw licence
  ├─ 2. Session (v2 HTML auth)     – no licence required ✓ proven on basic2016
  └─ 3. Java subprocess            – native Java client fallback (needs JDK 11+)
```

The server automatically tries each tier and falls back on HTTP 402. Session auth uses `/v2/` endpoints and mimics the native ABRA Flexi desktop client, bypassing the API licence check.

## Configuration

### Required

| Variable | Description |
|----------|-------------|
| `FLEXIBEE_URL` | FlexiBee server URL, e.g. `https://server:5434` |
| `FLEXIBEE_USERNAME` | Username |
| `FLEXIBEE_PASSWORD` | Password (mutually exclusive with `FLEXIBEE_PASSWORD_FILE`) |

### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `FLEXIBEE_PASSWORD_FILE` | – | Path to a file containing the password |
| `FLEXIBEE_COMPANY` | – | Default company db identifier |
| `FLEXIBEE_AUTH_MODE` | `auto` | `auto`, `api`, `session`, or `java` |
| `FLEXIBEE_TIMEOUT_MS` | 30000 | HTTP timeout in milliseconds |
| `FLEXIBEE_PAGE_SIZE` | 50 | Default page size (max 200) |
| `FLEXIBEE_CA_FILE` | – | PEM file for self-signed certificates |
| `FLEXIBEE_ALLOW_HTTP` | false | Allow plain HTTP to loopback hosts |
| `FLEXIBEE_SKIP_TLS_VERIFY` | false | Skip TLS verification (dev only) |
| `FLEXIBEE_JAVA_HOME` | auto | Path to JDK root for Java transport |
| `FLEXIBEE_CONFIG_FILE` | – | Path to a JSON profiles file |
| `FLEXIBEE_PROFILE` | – | Named profile to load from `FLEXIBEE_CONFIG_FILE` |

### Multiple profiles (FLEXIBEE_CONFIG_FILE)

Store multiple named server profiles in a JSON file:

```json
{
  "profiles": {
    "production": {
      "url": "https://flexi.example.com:5434",
      "username": "mcpbot",
      "password_file": "/etc/flexibee/password",
      "company": "my-company"
    },
    "staging": {
      "url": "https://flexi-dev.example.com:5434",
      "username": "mcpbot-dev",
      "password": "devpass",
      "company": "dev-company",
      "auth_mode": "session"
    }
  }
}
```

Select a profile with `FLEXIBEE_PROFILE=production`. Individual env vars override profile values.

### Session mode (recommended — no licence required)

```bash
FLEXIBEE_AUTH_MODE=session   # or leave as default "auto"
FLEXIBEE_USERNAME=mcpbot     # use a dedicated user, not your browser account
```

One active session is maintained per MCP process with automatic keep-alive (4-minute ping) and re-login on expiry. Use a dedicated user to avoid conflicting with browser sessions.

## Build

```bash
# Current platform
make build

# Linux amd64 (static)
make build-linux

# Windows amd64 (static)
make build-windows

# Both + checksums
make dist
```

Requires Go 1.23+. The binary has no runtime dependencies (CGO_ENABLED=0).

## Test

```bash
make test          # unit + integration tests
make vet           # go vet
make fmt-check     # gofmt check
make ci            # all of the above
```

115 tests pass (config, transport, metadata registry, MCP integration via mock Flexi server).

## Client configuration

### OpenCode (`opencode.json`)

```json
{
  "mcp": {
    "flexibee": {
      "type": "local",
      "command": ["/path/to/flexibee-mcp"],
      "environment": {
        "FLEXIBEE_URL": "https://your-server:5434",
        "FLEXIBEE_USERNAME": "mcpbot",
        "FLEXIBEE_PASSWORD_FILE": "/home/user/.flexibee-password",
        "FLEXIBEE_COMPANY": "your-company-db-id"
      }
    }
  }
}
```

See [examples/opencode.json](examples/opencode.json), [examples/claude-desktop.json](examples/claude-desktop.json) and [examples/claude-code.json](examples/claude-code.json) for complete examples.

## Deployment

For long-running server-side deployment, see [deploy/](deploy/) for a systemd unit file and Docker Compose example.

## Security

- Credentials are never logged or included in any MCP response.
- Use `FLEXIBEE_PASSWORD_FILE` to avoid exposing passwords in process environment.
- `FLEXIBEE_ALLOW_HTTP` and `FLEXIBEE_SKIP_TLS_VERIFY` are for development only.
- For session mode, use a dedicated FlexiBee user that does not share browser sessions.

## Architecture

```
Claude Desktop / Claude Code / OpenCode
         │ stdio JSON-RPC
flexibee-mcp (Go binary)
         │
         ├── AutoTransport
         │     ├── APITransport (Basic Auth)
         │     ├── SessionTransport (v2 session, browser mimic)
         │     └── JavaTransport (Java subprocess)
         │
         └── Remote ABRA Flexi Server (HTTPS)
```

## Platform targets

Confirmed: **Windows 11, Debian 13, openSUSE, Fedora**. macOS later.

## Licence

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE).

Repository: [opentreecz/flexibee-mcp](https://github.com/opentreecz/flexibee-mcp)
