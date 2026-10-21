# FlexiBee MCP — Linux Setup Tutorial

This tutorial walks you through building and installing `flexibee-mcp` on Linux and
connecting it to five AI coding assistants:

- [OpenCode](#opencode)
- [Claude Code (claude CLI)](#claude-code)
- [Claude Desktop](#claude-desktop)
- [Codex (OpenAI CLI)](#codex-openai-cli)
- [Antigravity / Agent Skills-compatible clients](#antigravity--agent-skills-clients)

---

## Prerequisites

| Requirement | Version | Check |
|---|---|---|
| Go | 1.23+ | `go version` |
| git | any | `git --version` |
| FlexiBee server | 2026+ | reachable over HTTPS |
| FlexiBee user | dedicated MCP account | see note below |

> **Dedicated user**: create a separate FlexiBee account (e.g. `mcpbot`) that you
> use only for this MCP server. This prevents session conflicts when you also
> use the FlexiBee web UI with a browser.

---

## Step 1 — Clone and build

```bash
git clone https://github.com/opentreecz/flexibee-mcp.git
cd flexibee-mcp
make build
```

The `make build` command produces a single static binary: `./flexibee-mcp`.

Install it system-wide or keep it local:

```bash
# System-wide (requires sudo)
sudo install -m 755 ./flexibee-mcp /usr/local/bin/flexibee-mcp

# Or local install
mkdir -p ~/.local/bin
install -m 755 ./flexibee-mcp ~/.local/bin/flexibee-mcp
# Add ~/.local/bin to PATH if not already there:
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

Verify:

```bash
flexibee-mcp --version
```

---

## Step 2 — Store your password securely

Never put your password in a config file or environment variable visible in
`/proc`. Use a dedicated password file instead:

```bash
install -m 600 /dev/null ~/.flexibee-password
echo 'YourMCPBotPassword' > ~/.flexibee-password
```

Verify the permissions:

```bash
ls -la ~/.flexibee-password
# -rw------- 1 you you 20 ...
```

---

## Step 3 — Find your company identifier

The `company` field is the FlexiBee database name (`dbNazev`), not the display name.
Run this one-time probe to find it (replace the URL and credentials):

```bash
FLEXIBEE_URL=https://your-server:5434 \
FLEXIBEE_USERNAME=mcpbot \
FLEXIBEE_PASSWORD=YourMCPBotPassword \
FLEXIBEE_AUTH_MODE=session \
flexibee-mcp &
sleep 1
# Then in another terminal, run your MCP client and call flexi_list_companies
# Or use curl against the FlexiBee API directly:
curl -sk -u mcpbot:YourMCPBotPassword \
  https://your-server:5434/v2/c.json | python3 -m json.tool | grep dbNazev
```

Note the `dbNazev` value — that is your `FLEXIBEE_COMPANY`.

---

## Step 4 — Configure the MCP server

All configuration is done through environment variables. The server binary reads
them at startup — there is no config file to write.

| Variable | Required | Example |
|---|---|---|
| `FLEXIBEE_URL` | Yes | `https://flexi.example.com:5434` |
| `FLEXIBEE_USERNAME` | Yes | `mcpbot` |
| `FLEXIBEE_PASSWORD_FILE` | Yes | `~/.flexibee-password` |
| `FLEXIBEE_COMPANY` | Recommended | `my-company-2024` |
| `FLEXIBEE_AUTH_MODE` | No (default: `auto`) | `session` |
| `FLEXIBEE_CA_FILE` | For self-signed TLS | `/etc/ssl/my-ca.pem` |

**Auth mode recommendation**: use `session` explicitly if you know your server
has a basic2016 or similar licence without `rest-api-rw`. The `auto` mode tries
API first and falls back automatically, but session is faster for known setups.

Test the binary directly before wiring it into any client:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' | \
FLEXIBEE_URL=https://your-server:5434 \
FLEXIBEE_USERNAME=mcpbot \
FLEXIBEE_PASSWORD_FILE=~/.flexibee-password \
FLEXIBEE_COMPANY=your-company \
FLEXIBEE_AUTH_MODE=session \
flexibee-mcp
```

You should see a JSON-RPC response with `"result":{"serverInfo":...}` on stdout.
All log output goes to stderr.

---

## Step 5 — Install skills globally

Skills are SKILL.md files placed in directories that AI clients scan.
The `scripts/install-skills.sh` helper installs all 7 skills to every
supported location:

```bash
bash scripts/install-skills.sh
```

To install manually or to a specific client only, see the per-client sections below.

---

## OpenCode

**Config file**: `~/.config/opencode/opencode.jsonc` (global) or
`./opencode.json` / `./opencode.jsonc` in your project directory.

### Register the MCP server

Add the `flexibee` block to `~/.config/opencode/opencode.jsonc`:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "flexibee": {
      "type": "local",
      "command": ["/usr/local/bin/flexibee-mcp"],
      "environment": {
        "FLEXIBEE_URL": "https://your-server:5434",
        "FLEXIBEE_USERNAME": "mcpbot",
        "FLEXIBEE_PASSWORD_FILE": "/home/YOU/.flexibee-password",
        "FLEXIBEE_COMPANY": "your-company",
        "FLEXIBEE_AUTH_MODE": "session"
      },
      "enabled": true,
      "timeout": 15000
    }
  }
}
```

Replace `/home/YOU/` with your actual home directory — tilde expansion is not
supported in all OpenCode versions.

### Install skills for OpenCode

OpenCode searches three locations for skills:

```
.opencode/skills/<name>/SKILL.md       ← project-local (walk up from cwd)
~/.config/opencode/skills/<name>/SKILL.md  ← global
.agents/skills/<name>/SKILL.md         ← Agent Skills compatible
~/.agents/skills/<name>/SKILL.md       ← Agent Skills global
```

Install all FlexiBee skills globally:

```bash
SKILL_DST="$HOME/.config/opencode/skills"
REPO_DIR="$HOME/git/flexibee-mcp/skills"   # adjust to your clone path
for skill in "$REPO_DIR"/*/; do
  name=$(basename "$skill")
  mkdir -p "$SKILL_DST/$name"
  cp "$skill/SKILL.md" "$SKILL_DST/$name/SKILL.md"
done
```

Or use the provided helper:

```bash
bash scripts/install-skills.sh --opencode
```

### Verify in OpenCode

```
opencode
```

Type `/mcp` in the TUI to confirm `flexibee` appears as a connected server.
Ask a question like "What companies are available in FlexiBee?" — OpenCode
will call `flexi_list_companies` automatically.

To load a skill explicitly:

```
/flexi-explore
```

Or ask: "Use the flexi-receivables skill to show me overdue invoices for company my-company."

---

## Claude Code

**Claude Code** is the `claude` CLI from Anthropic.

### Register the MCP server (global)

```bash
claude mcp add flexibee \
  -e FLEXIBEE_URL=https://your-server:5434 \
  -e FLEXIBEE_USERNAME=mcpbot \
  -e FLEXIBEE_PASSWORD_FILE="$HOME/.flexibee-password" \
  -e FLEXIBEE_COMPANY=your-company \
  -e FLEXIBEE_AUTH_MODE=session \
  -- /usr/local/bin/flexibee-mcp
```

Verify:

```bash
claude mcp list
# flexibee: /usr/local/bin/flexibee-mcp (stdio)
```

### Register via .mcp.json (project-local)

Place a `.mcp.json` in your project root — Claude Code picks it up automatically
when you `cd` into that directory:

```json
{
  "mcpServers": {
    "flexibee": {
      "command": "/usr/local/bin/flexibee-mcp",
      "env": {
        "FLEXIBEE_URL": "https://your-server:5434",
        "FLEXIBEE_USERNAME": "mcpbot",
        "FLEXIBEE_PASSWORD_FILE": "/home/YOU/.flexibee-password",
        "FLEXIBEE_COMPANY": "your-company",
        "FLEXIBEE_AUTH_MODE": "session"
      }
    }
  }
}
```

Claude Code will ask you to approve the `.mcp.json` on first use.

### Install skills for Claude Code

Claude Code searches:

```
.claude/skills/<name>/SKILL.md         ← project-local
~/.claude/skills/<name>/SKILL.md       ← global
.agents/skills/<name>/SKILL.md         ← Agent Skills compatible
```

Install globally:

```bash
SKILL_DST="$HOME/.claude/skills"
REPO_DIR="$HOME/git/flexibee-mcp/skills"
for skill in "$REPO_DIR"/*/; do
  name=$(basename "$skill")
  mkdir -p "$SKILL_DST/$name"
  cp "$skill/SKILL.md" "$SKILL_DST/$name/SKILL.md"
done
```

### Verify in Claude Code

```bash
claude
```

Ask: "List all available FlexiBee companies." The agent will call
`flexi_list_companies` through the MCP server.

To invoke a skill by slash command (if supported in your version):

```
/flexi-invoice
```

Or ask: "Use the flexi-contacts skill to find the company with ICO 12345678."

---

## Claude Desktop

On Linux, Claude Desktop uses:

**Config file**: `~/.config/Claude/claude_desktop_config.json`

If the file doesn't exist, create it:

```bash
mkdir -p ~/.config/Claude
```

### Register the MCP server

```json
{
  "mcpServers": {
    "flexibee": {
      "command": "/usr/local/bin/flexibee-mcp",
      "env": {
        "FLEXIBEE_URL": "https://your-server:5434",
        "FLEXIBEE_USERNAME": "mcpbot",
        "FLEXIBEE_PASSWORD_FILE": "/home/YOU/.flexibee-password",
        "FLEXIBEE_COMPANY": "your-company",
        "FLEXIBEE_AUTH_MODE": "session"
      }
    }
  }
}
```

**Important**: Claude Desktop does not support tilde (`~`) in paths — use the
full absolute path for `FLEXIBEE_PASSWORD_FILE`.

Restart Claude Desktop after saving. In **Settings → Developer**, confirm
`flexibee` appears in the MCP servers list with a green status indicator.

### Install skills for Claude Desktop

Claude Desktop does not natively load SKILL.md files from the filesystem.
The recommended approach is to use the content of a SKILL.md as a **Custom
System Prompt** or **Project Instructions** in a Claude Project:

1. Open Claude Desktop → **Projects** → create a project for FlexiBee work.
2. Open the project → **Project Instructions** (pencil icon).
3. Copy-paste the contents of the relevant SKILL.md file(s):
   - `skills/flexi-explore/SKILL.md` for general exploration
   - `skills/flexi-receivables/SKILL.md` for accounts receivable
   - etc.
4. Save — the instructions apply to all conversations in that project.

Alternatively, use the **flexi_get_guide** MCP tool: Claude Desktop will call it
automatically when you ask for FlexiBee workflow guidance.

---

## Codex (OpenAI CLI)

**Codex** is OpenAI's open-source coding agent CLI (`@openai/codex`).

Install it:

```bash
npm install -g @openai/codex
# or: curl -fsSL https://chatgpt.com/codex/install.sh | sh
```

### MCP configuration file

Codex reads MCP servers from `~/.codex/config.toml` (global) or
`.codex/config.toml` in your project directory.

Create or edit `~/.codex/config.toml`:

```toml
# FlexiBee MCP server
[mcp_servers.flexibee]
command = ["/usr/local/bin/flexibee-mcp"]
env = {
  FLEXIBEE_URL = "https://your-server:5434",
  FLEXIBEE_USERNAME = "mcpbot",
  FLEXIBEE_PASSWORD_FILE = "/home/YOU/.flexibee-password",
  FLEXIBEE_COMPANY = "your-company",
  FLEXIBEE_AUTH_MODE = "session"
}
```

Or use the CLI to add it:

```bash
codex mcp add flexibee \
  --env FLEXIBEE_URL=https://your-server:5434 \
  --env FLEXIBEE_USERNAME=mcpbot \
  --env "FLEXIBEE_PASSWORD_FILE=$HOME/.flexibee-password" \
  --env FLEXIBEE_COMPANY=your-company \
  --env FLEXIBEE_AUTH_MODE=session \
  -- /usr/local/bin/flexibee-mcp
```

Verify:

```bash
codex mcp list
```

Type `/mcp` in the Codex TUI to confirm the server is connected.

### Skills for Codex — AGENTS.md

Codex uses `AGENTS.md` files (not SKILL.md) for persistent instructions. Place
an `AGENTS.md` in your project root or `~/.codex/AGENTS.md` for global
instructions:

```bash
cat > ~/.codex/AGENTS.md << 'EOF'
# FlexiBee MCP

You have access to the `flexibee` MCP server with 21 tools for the ABRA FlexiBee
accounting system. Always start with `flexi_get_connection_info` to verify
connectivity and confirm the company identifier before any data query.

Key tools:
- flexi_list_companies — discover available companies
- flexi_list_evidences — browse evidence (table) catalog
- flexi_describe_evidence — inspect field schema (section: fields/relations/actions)
- flexi_query_records — filter, sort and page records
- flexi_get_record — single record by id or code
- flexi_upsert_record — create or update (use dry_run=true first)
- flexi_execute_action — trigger workflow actions (lock, cancel, send)
- flexi_get_report — download PDF/XLSX reports (base64)
- flexi_get_changes — incremental sync via Changes API

Use the `flexi_get_guide` tool for detailed workflow guidance on invoices,
contacts, banking, inventory, orders, filtering syntax, and auth configuration.
EOF
```

To use a full skill, copy its SKILL.md content into the AGENTS.md, or reference
it with `@` in the Codex TUI:

```bash
# Copy a skill's workflow into AGENTS.md
cat skills/flexi-receivables/SKILL.md >> ~/.codex/AGENTS.md
```

### Verify Codex

```bash
codex
```

Ask: "What FlexiBee companies are available?" — Codex will call
`flexi_list_companies` via the MCP server.

---

## Antigravity / Agent Skills-compatible clients

Tools that implement the [Agent Skills specification](https://agentskills.io/specification)
(including Antigravity and other compatible clients) discover skills from:

```
.agents/skills/<name>/SKILL.md         ← project-local
~/.agents/skills/<name>/SKILL.md       ← global
```

The skills in this repository use the Agent Skills frontmatter format and work
with any compliant client.

### Install globally for all Agent Skills clients

```bash
SKILL_DST="$HOME/.agents/skills"
REPO_DIR="$HOME/git/flexibee-mcp/skills"
for skill in "$REPO_DIR"/*/; do
  name=$(basename "$skill")
  mkdir -p "$SKILL_DST/$name"
  cp "$skill/SKILL.md" "$SKILL_DST/$name/SKILL.md"
done
```

### Register MCP server in Antigravity

The exact configuration format depends on your Antigravity version. The general
pattern for any Agent Skills-compatible client that supports local stdio MCP
servers:

```json
{
  "mcp": {
    "flexibee": {
      "type": "stdio",
      "command": "/usr/local/bin/flexibee-mcp",
      "env": {
        "FLEXIBEE_URL": "https://your-server:5434",
        "FLEXIBEE_USERNAME": "mcpbot",
        "FLEXIBEE_PASSWORD_FILE": "/home/YOU/.flexibee-password",
        "FLEXIBEE_COMPANY": "your-company",
        "FLEXIBEE_AUTH_MODE": "session"
      }
    }
  }
}
```

Consult your client's documentation for the exact config file location and format.

---

## All-at-once: automated install script

Use `scripts/install-skills.sh` to install all skills to every supported location
in one command:

```bash
# Install to all discovered locations
bash scripts/install-skills.sh

# Install to specific client only
bash scripts/install-skills.sh --opencode
bash scripts/install-skills.sh --claude
bash scripts/install-skills.sh --agents
bash scripts/install-skills.sh --all   # default: all
```

The script reports which directories it writes to.

---

## Troubleshooting

### "server unreachable" in flexi_get_connection_info

```bash
# Test the FlexiBee server directly
curl -sk https://your-server:5434/status.json | python3 -m json.tool
```

If you get a certificate error, add the CA to the config:

```
FLEXIBEE_CA_FILE=/path/to/ca.pem
```

Or for testing only (never production):

```
FLEXIBEE_SKIP_TLS_VERIFY=true
```

### HTTP 402 Payment Required

Your FlexiBee licence does not include `rest-api-rw`. Switch to session auth:

```
FLEXIBEE_AUTH_MODE=session
```

Session auth uses the HTML login path which bypasses the API licence check.
This is the recommended mode for most self-hosted instances.

### "active_transport" shows "api" but calls fail

The auto transport tried API first and it worked (your server has the licence).
If it then fails on data calls, check that your user account has the right
FlexiBee permissions for the evidences you are querying.

### Session conflict — "remoteLogoutDemand"

Another session for the same user and client type was created (e.g. you opened
the FlexiBee web UI). The MCP server will automatically re-login on the next
call. To prevent this entirely, use a dedicated `mcpbot` account that you never
use in the browser.

### MCP server not appearing in client

- Verify the binary path is absolute and executable: `ls -la /usr/local/bin/flexibee-mcp`
- Check the binary produces JSON-RPC output: see Step 4 test command above.
- Restart the client after changing the config.
- For Claude Code, ensure the `.mcp.json` is in your current working directory
  and you have approved it.

### Skills not showing up

- File must be named exactly `SKILL.md` (all caps).
- Frontmatter must include `name:` and `description:`.
- Skill name must match the directory name.
- Run the install script to check paths: `bash scripts/install-skills.sh --dry-run`

---

## Multiple FlexiBee instances (profiles)

Use a JSON profiles file to manage multiple servers:

```bash
cat > ~/.flexibee-profiles.json << 'EOF'
{
  "profiles": {
    "production": {
      "url": "https://flexi.example.com:5434",
      "username": "mcpbot",
      "password_file": "/home/you/.flexibee-password",
      "company": "my-company",
      "auth_mode": "session"
    },
    "staging": {
      "url": "https://flexi-dev.example.com:5434",
      "username": "mcpbot-dev",
      "password_file": "/home/you/.flexibee-dev-password",
      "company": "dev-company",
      "auth_mode": "session"
    }
  }
}
EOF
```

Configure a client to use the production profile:

```json
{
  "env": {
    "FLEXIBEE_CONFIG_FILE": "/home/YOU/.flexibee-profiles.json",
    "FLEXIBEE_PROFILE": "production"
  }
}
```

Individual env vars override profile values, so you can switch profiles per
project by setting `FLEXIBEE_PROFILE` in the project-level config.

---

## Summary

| Client | MCP config | Skill path |
|---|---|---|
| OpenCode | `~/.config/opencode/opencode.jsonc` | `~/.config/opencode/skills/` |
| Claude Code | `claude mcp add` or `.mcp.json` | `~/.claude/skills/` |
| Claude Desktop | `~/.config/Claude/claude_desktop_config.json` | Project Instructions (paste) |
| Codex CLI | `~/.codex/config.toml` or `codex mcp add` | `~/.codex/AGENTS.md` |
| Antigravity | client config (stdio MCP) | `~/.agents/skills/` |

All clients that support the Agent Skills specification also pick up skills from
`~/.agents/skills/<name>/SKILL.md`.
