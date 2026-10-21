# Verification

This file records tested configurations. It is updated as new combinations are verified.

## Server versions

| FlexiBee version | Licence | Transport | Status |
|-----------------|---------|-----------|--------|
| 2026.6.2 (Docker) | basic2016 (apiUsers=0) | Session (v2) | ✅ Reads + writes confirmed 2026-10-17 |
| 2026.6.2 (Docker) | basic2016 (apiUsers=0) | API (Basic Auth) | ❌ HTTP 402 — licence required |

## Client versions

| Client | Version | Platform | Status |
|--------|---------|----------|--------|
| OpenCode | TBD | Linux x86-64 | Pending smoke test |
| Claude Code | TBD | Linux x86-64 | Pending smoke test |
| Claude Desktop | TBD | Windows 11 | Pending |

## Platform build status

| OS | Architecture | Build | Tests |
|----|-------------|-------|-------|
| Linux (Debian/Ubuntu) | x86-64 | ✅ | ✅ 110 pass |
| Windows | x86-64 | ✅ (cross-compile from Linux) | Pending native run |
| macOS | x86-64 / arm64 | Pending | Pending |

## Transport verification matrix

| Transport | Endpoint | Read | Write | Notes |
|-----------|----------|------|-------|-------|
| Session (v2) | `/v2/c/{co}/{evidence}.json` | ✅ | ✅ | CSRF token required for writes |
| API (Basic) | `/c/{co}/{evidence}.json` | ❌ 402 | ❌ 402 | Requires rest-api-rw licence |
| Java subprocess | `/v2/c/{co}/{evidence}.json` | Pending | Pending | Requires JDK 11+ |

## Evidence types verified

The following evidence types were successfully queried against FlexiBee 2026.6.2:

- `faktura-vydana` — outbound invoices (read + write)
- `adresar` — contacts (read + write)
- `evidence-list` — evidence catalog (read)

All 317 evidence types from `EvidenceType` enum are expected to work; only the above have been explicitly tested.

## Known limitations

- **API licence**: Basic Auth REST requires `rest-api-rw` licence. Session auth bypasses this check.
- **Session conflicts**: Only one active session per user per client type (`FLEXI`). Use a dedicated user (e.g. `mcpbot`) for the MCP server.
- **Java transport**: Requires JDK 11+ (`javac` must be on PATH or in `FLEXIBEE_JAVA_HOME`). Not tested in production yet.
- **Cloud compatibility**: Not yet tested against ABRA Flexi Cloud (only self-hosted Docker instance).
- **Period filtering**: `faktura-vydana` applies implicit period filters. Pass `filtrovat-platnost=false` in the filter to retrieve all-time records.
