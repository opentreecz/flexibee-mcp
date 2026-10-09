# Phase 1 — read-only local MCP

Prepared: 2026-10-09. Status: **ready to implement, not started.** No Go code exists in the repository yet.

Phase 1 implements the M1 scope from the [roadmap](roadmap.md) under the decisions in [decisions.md](decisions.md) and the design in [architecture.md](architecture.md). Flexi API behavior referenced here is sourced in [research.md](research.md).

## 1. Goal

A user configures one FlexiBee account, registers the server in Claude Desktop, Claude Code, or OpenCode, and can ask grounded questions about their accounting data — which companies exist, what fields an evidence has, which invoices are unpaid or overdue — with answers traceable to actual API responses.

## 2. Scope

**In scope:** local stdio transport, single-account configuration, metadata discovery, bounded read queries, relation reads, document sums, guidance (prompts/resources/tool), two skills, native Windows/Linux x86-64 builds, tests, client setup examples.

**Out of scope for Phase 1:** any write or mutation path, dry-run previews, PDF/XLSX export, attachments, hosted HTTP transport, multiple account profiles, Changes API/webhooks, container/Helm packaging, macOS builds, Flexi Cloud certification. These remain in later milestones.

A tool that mutates data must not appear in Phase 1, not even disabled. All tools are read-only, and the only non-GET upstream call permitted is `POST /{evidence}/query.json`, which Flexi defines as a read operation.

## 3. Prerequisites

| Prerequisite | State | Note |
| --- | --- | --- |
| Go toolchain | **missing on this machine** | `go` is not on `PATH`. The official SDK's tagged `go.mod` declares `go 1.25.0`, so install Go 1.25 or newer before WP1. |
| Official Go MCP SDK | available | `github.com/modelcontextprotocol/go-sdk`; pin an exact published version in `go.mod`/`go.sum`. |
| Target Flexi instance | **not supplied** | Needed for smoke tests in WP9, not for WP1–WP8. |
| Test company + API account | **not supplied** | Must be a disposable company; credentials configured locally, never committed or pasted into chat. |
| Client versions / OS builds | **not recorded** | Needed before claiming client or platform verification. |
| openSUSE edition, Fedora release | **unspecified** | Needed before claiming those platforms verified; Debian 13 and Windows 11 are confirmed targets. |

Missing items block verification claims, not core development. WP1–WP8 can be completed and tested against fixtures and a mock HTTP server with no Flexi access.

## 4. Work packages

Each package lists its deliverable and how it is proven. Packages are ordered by dependency; WP6 and WP7 can proceed in parallel with WP5.

### WP1 — Project skeleton

Go module, directory layout from [architecture.md](architecture.md) §2, pinned SDK, `Makefile` or task scripts, `.gitignore`, `.editorconfig`.

*Proven by:* `go build ./...`, `go vet ./...`, `gofmt -l` clean, and a committed lockfile.

### WP2 — Configuration

Load and validate `FLEXIBEE_URL`, `FLEXIBEE_COMPANY`, `FLEXIBEE_USERNAME`, `FLEXIBEE_PASSWORD`, `FLEXIBEE_PASSWORD_FILE`, `FLEXIBEE_TIMEOUT_MS`, `FLEXIBEE_PAGE_SIZE`, `FLEXIBEE_CA_FILE`.

Rules: require a base URL and credentials; reject `FLEXIBEE_PASSWORD` together with `FLEXIBEE_PASSWORD_FILE`; preserve any configured path prefix; never hardcode port 5434; refuse plain `http://` unless the host is loopback and the user set an explicit override flag; trim trailing whitespace from secret files; never log secret values.

*Proven by:* table-driven tests for valid and invalid combinations, including a secret file with a trailing newline and a URL with a path prefix.

### WP3 — Flexi HTTP client

One `*http.Client` with bounded timeouts, connection reuse, Basic authentication, `Accept: application/json`, a descriptive `User-Agent`, optional private CA appended to the system pool, and context cancellation.

Redirects: Flexi answers non-internal identifiers with `301` to the internal-ID URL. Follow redirects only when scheme, host, and port match the configured origin, with a small hop limit. Never forward credentials cross-origin and never downgrade HTTPS to HTTP.

*Proven by:* `httptest` tests for timeout, cancellation, same-origin redirect followed, cross-origin redirect refused, HTTPS-to-HTTP refused, custom CA accepted.

### WP4 — Envelopes, types, and errors

Separate parsers for the record envelope (`winstrom`) and the metadata envelopes (`companies`, `evidences`, `properties`, `relations`, `reports`). Decode numbers with `json.Number` or raw strings so identifiers, monetary values, and `@rowCount` keep their exact text. Convert documented string flags such as `"true"`/`"false"`explicitly; never rely on truthiness. Keep business dates as date-only values and retain offsets on timestamps.

Error taxonomy, each mapped to an actionable message: configuration, transport/timeout, authentication (401), permission or licensing (402/403, and 404 where licensing hides a capability), not found (404), validation (400), unsupported format or report (406), conflict (409), maintenance (503), and application-level failure carried in a 200 body. Detect an HTML login or proxy page and report it as a configuration problem rather than a parse failure.

*Proven by:* fixtures captured from the public demo and hand-built error bodies; assertions that a decimal string survives round-trip unchanged and that `"false"` does not become `true`.

### WP5 — Metadata registry

Fetch and cache company list, evidence catalog, properties, relations, and reports per instance, company, and credential identity, with a configurable TTL and explicit refresh. Record data is never cached. Validate requested fields and relations against metadata before building a request.

The demo catalog exposed 253 evidences; discovery output must stay compact and must not inline full schemas. Import status indicates import support, not read permission, and must not be presented as a write guarantee.

*Proven by:* tests for cache hit, expiry, isolation between credentials, unknown-field rejection, and compact summary size.

### WP6 — Query builder

Typed filter tree producing Flexi filter expressions (comparison, `and`/`or`/`not`, `like`, `like similar`, `in`, null and empty tests, dates, `now()`, dotted relation paths), correct URL-component encoding, `detail=custom:` projections, ordering, and paging with `limit`/`start`/`add-row-count`.

Defaults: 50 records per page, hard cap 200, plus a serialized response-size budget. Always return the resolved filter, paging position, and an explicit completeness indicator. Treat negative comparisons on nested relations as excluding rows without that relation, and document that in the tool output. Evidences carrying `platiOd`/`platiDo` are implicitly period-filtered unless `filtrovat-platnost=false` is set, so historical queries must be explicit. For `POST /query.json`, put `limit` in the body because the URL value is ignored.

*Proven by:* encoding tests including quotes, diacritics, slashes in identifiers, and a filter that would be ambiguous without parentheses.

### WP7 — MCP tool surface

Register the nine Phase 1 tools over stdio with JSON Schema inputs, structured output plus a short text rendering, and read-only annotations.

| Tool | Input | Returns |
| --- | --- | --- |
| `flexi_get_connection_info` | optional `company` | reachability, resolved context, capability diagnostics, no secrets |
| `flexi_list_companies` | — | database identifiers and display names |
| `flexi_list_evidences` | `company`, `search`, paging | compact catalog summary with import status |
| `flexi_describe_evidence` | `company`, `evidence`, `section`, field filter | fields and types, relations, or report metadata |
| `flexi_query_records` | `company`, `evidence`, `filter`, `fields`, `sort`, `limit`, `start` | records, paging, optional count, query context |
| `flexi_get_record` | `company`, `evidence`, `identifier`, `fields` | one resolved record |
| `flexi_get_related_records` | `company`, `evidence`, `identifier`, `relation`, paging | records from a discovered relation |
| `flexi_sum_records` | `company`, `evidence`, `filter` | server-provided document totals with context |
| `flexi_get_guide` | `topic` | workflow and API guidance |

Every company-scoped response echoes the resolved company. Company selection never changes credentials. There is no mutable "current company" tool, so concurrent calls cannot retarget each other. `flexi_sum_records` is valid only for document evidences; other evidences return 404 and must produce a clear capability error.

Keep stdout exclusively for protocol traffic and send redacted diagnostics to stderr.

*Proven by:* an integration test that launches the compiled binary as a real stdio child process, negotiates with an SDK client, enumerates tools, prompts, and resources, calls each tool against a mock Flexi server, and asserts stdout contains only protocol frames.

### WP8 — Guidance and skills

MCP prompts `review_receivables`, `prepare_invoice`, `inspect_inventory`; resources under `flexi://guides/{topic}`; the same content reachable through `flexi_get_guide` because resource surfacing is client-dependent.

Skills `flexi-explore` and `flexi-receivables` as portable `SKILL.md` directories plus per-skill ZIPs. Skills call tools, contain no credentials, and explain how to report ambiguous company, period, or document references. They must state that receivables answers depend on partial payments, currency, and whether a page or the full set was returned.

*Proven by:* frontmatter and naming validation, ZIP reproducibility, and scripted scenarios checking expected tool usage and grounded answers.

### WP9 — Release artifacts and verification

Native `windows/amd64` and `linux/amd64` builds with `CGO_ENABLED=0` where supported, checksums, version and commit stamped into `--version`, and client setup examples for Claude Desktop, Claude Code, and OpenCode that inject credentials without committing them.

CI runs build, vet, format check, unit and contract tests, and the stdio protocol test on Linux and Windows runners. CI must never call or mutate the public demo; live tests are opt-in and require an explicitly configured disposable company.

Platform checks: Windows paths containing spaces and non-ASCII characters, Linux file permissions on secret files, system and private CA roots, process start and clean shutdown, and behavior behind a reverse proxy.

*Proven by:* a recorded verification matrix naming the actual OS builds, client versions, and Flexi version. Any environment not available is recorded as unverified, never as passed.

## 5. Definition of done

- [ ] `go build`, `go vet`, format check, and the full test suite pass on Linux and Windows CI.
- [ ] A user can select a company, inspect invoice fields, and list overdue invoices with correct paging and completeness reporting.
- [ ] Reads work across discovered supported evidences without one tool per evidence.
- [ ] Unknown fields and relations, bad configuration, HTML login responses, and unavailable licensed capabilities each produce a distinct actionable error.
- [ ] Monetary and date values are byte-identical to the API response in tool output.
- [ ] stdout carries protocol traffic only; no secret appears in logs or tool output.
- [ ] Both skills install and work in at least one CLI client and one Desktop client.
- [ ] Windows 11 and Debian 13 smoke tests recorded against the real Flexi instance; openSUSE and Fedora recorded once releases are specified.
- [ ] No mutating tool exists in the shipped surface.

## 6. Risks

| Risk | Mitigation |
| --- | --- |
| Go toolchain absent locally | Install Go 1.25+ as the first task; the SDK's declared minimum is not optional. |
| API entitlement or daily request quota unknown on the target | Probe capability at connection time, cache metadata, keep paging bounded, and surface licensing errors explicitly. |
| Two-factor authentication on the account | Basic authentication then needs a current OTP per request. Detect and report this as an unsupported auth mode in Phase 1 rather than appearing to hang. |
| Metadata and import-status assumptions from the demo instance | Treat the demo as one observation; validate against the target instance before documenting support. |
| Response size exhausting model context | Enforce page caps and a size budget, and return continuation data instead of silently truncating. |
| Relation naming mismatch, for example `polozkyDokladu` versus `polozkyFaktury` | Drive navigation from discovered relations; keep read paths and future write field names in separate mappings. |

## 7. Suggested sequence

1. Install Go, then WP1 and WP2.
2. WP3 and WP4 with fixtures.
3. WP5, then WP6.
4. WP7 to first working stdio session.
5. WP8 guidance and skills.
6. WP9 builds, CI, and recorded verification.

Steps 1–5 need no Flexi access. Step 6 needs the instance, accounts, and client environments listed in §3.
