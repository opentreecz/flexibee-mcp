# Proposed architecture

Status: **Go/local-stdio direction approved; implementation pending**. Latest user decisions: x86-64, Windows 11/Debian 13/openSUSE/Fedora; macOS later. One FlexiBee account per local process. Hosted HTTP and multiple profiles are backlog features. See [decisions](decisions.md), [API research](research.md), and [language comparison](language-comparison.md). Names, limits, and paths below describe intended implementation, not existing functionality.

## 1. Core decisions

1. **Go with the official MCP Go SDK**, schema validation, and a small direct HTTP adapter. Pin a supported toolchain and published SDK with `go.mod`/`go.sum`. Prefer pure-Go dependencies for native Windows/Linux distribution.
2. **Local stdio in M1**. Native Windows 11 and Linux x86-64 executables, verified on Debian 13/openSUSE/Fedora. Keep the service layer reusable for future Streamable HTTP; macOS later.
3. **A compact schema-aware tool set**, with dedicated tools for domain actions. Evidence count must not dictate tool count.
4. **Live instance metadata** with lazy bounded caching. Model output must not invent evidence names, fields, document types, or enumerations.
5. **Portable skills shipped as release artifacts**, alongside MCP prompts/resources backed by the same guidance content.
6. **Cloud-compatible connection settings from the outset**, with cloud support declared only after cloud testing.

Go is recommended for the balance of maintainability, native distribution, concurrency and footprint. Rust and Python remain viable alternatives; no language is mandated by MCP. Direct SQL access would bypass Flexi business logic and would not give the same cloud deployment path.

## 2. Components

```text
Claude Desktop / Claude Code / OpenCode
                 |
              MCP stdio             future: HTTPS / Streamable HTTP
                 |                               |
                 +-------- MCP server -----------+
                            |
              tools / resources / prompts
                            |
            query and business-operation services
                            |
            schema registry + endpoint adapters
                            |
            Flexi HTTP client + authentication
                            |
            self-hosted Flexi / cloud Flexi

Portable skills ---> client skill loader ---> MCP tools
```

Proposed layout:

```text
cmd/flexibee-mcp/          entry point and transport selection
internal/
  config/                 validated settings and secret-file loading
  flexi/                  HTTP client, identifiers, queries, envelopes
  metadata/               bounded schema registry/cache
  service/                business operations independent of MCP
  mcpserver/              tool/resource/prompt registration
  transport/              stdio startup/lifecycle; HTTP later
  observability/          redacted logging, metrics and health
skills/                   canonical distributable skill directories
testdata/                 sanitized fixtures; Go tests beside packages
examples/                 client setup templates
deploy/                   future container, systemd and Helm examples
docs/
```

Use one module and one executable rather than microservices. Avoid a database dependency in M1. Later durable event processing may need a separate persistence layer.

## 3. Configuration and company context

Proposed settings:

| Setting | Meaning |
| --- | --- |
| `FLEXIBEE_URL` | Base server URL, preserving configured path prefix; no hardcoded port |
| `FLEXIBEE_COMPANY` | Default database identifier, optional when explicitly choosing a company |
| `FLEXIBEE_USERNAME` / `FLEXIBEE_PASSWORD` | Dedicated API credentials supplied outside committed files |
| `FLEXIBEE_PASSWORD_FILE` | Alternative secret-file input; mutually exclusive with password |
| `FLEXIBEE_TIMEOUT_MS` | Bounded upstream request timeout |
| `FLEXIBEE_PAGE_SIZE` | Default bounded page size |
| `FLEXIBEE_CA_FILE` | Append a private CA bundle to the OS trust roots for Flexi requests |

Company-scoped tools accept an optional `company`; resolve it against the default and the authenticated user's available companies. If neither is supplied, report that a company is required. Return the resolved company in every response. Avoid a mutable global “current company” tool: concurrent calls must not switch one another's targets.

A single process connects to one configured FlexiBee server using one account. Multiple instances can be registered as separate named MCP servers. FlexiBee enforces the configured user's permissions; company selection never changes credentials. Multiple account profiles in one process are a requested backlog feature. A future remote deployment will additionally need a caller-to-company access policy.

## 4. Initial MCP surface

### M1: discovery and read-only queries

| Proposed tool | Core input | Output / role |
| --- | --- | --- |
| `flexi_get_connection_info` | optional company | Reachability, configured context, capability diagnostics without secrets |
| `flexi_list_companies` | none | Available company identifiers and names |
| `flexi_list_evidences` | company, search, paging | Compact metadata summary and import status |
| `flexi_describe_evidence` | company, evidence, section, field filter | Fields/types, relations, or report metadata; bounded output |
| `flexi_query_records` | company, evidence, filter, fields, sort, limit, start | Records, paging, optional count, query context |
| `flexi_get_record` | company, evidence, identifier, fields | One resolved record |
| `flexi_get_related_records` | company, evidence, identifier, relation, paging | A discovered relation's records |
| `flexi_sum_records` | company, supported evidence, filter | Server-provided document totals and context |
| `flexi_get_guide` | topic | Tool-accessible workflow/API guidance |

Discovery returns compact summaries, not raw 253-entry schemas. Full field descriptions can be retrieved selectively. All M1 tools have read-only semantics; any internal POST `/query` must be an explicit known read endpoint.

Generic queries initially cover normal record evidences. Discovery may list advanced reports whose required parameters need dedicated adapters; listing an evidence does not imply all its operations already work.

### M2: useful writes and documents

Add preview/create/update services for contacts and outgoing invoices, with explicit create-only/update-only semantics, known document/line mappings, returned warnings, and post-write readback. Expose dry-run preview independently from persistence. Support incoming invoices after their field/default rules are covered by fixtures and sandbox tests.

Add report export, attachment metadata/download, and artifact retrieval. Upload/replacement needs its own tested operation because attachment bytes are immutable in place.

M3 adds typed business tools for payment matching, order realization, stock/report operations. Separate tools make action semantics and MCP annotations accurate; an arbitrary HTTP request tool would obscure those semantics.

## 5. Adapter contracts and correctness

- Build URLs from validated base URL, company, discovered evidence, and encoded identifiers. Resolve identifier redirects only within the configured instance boundary, with a hop limit. Do not forward credentials to a different origin or downgrade HTTPS.
- Validate field and relation names against metadata. Use a typed filter tree for common expressions and an explicitly documented advanced expression input if needed; do not concatenate unescaped user values.
- Keep operation-specific query parameters allowlisted. Report endpoints can silently ignore typos, so validation is essential to correctness.
- Default to 50 records, initially cap pages at 200, and enforce a serialized response-size budget. Return continuation information rather than silently dropping data.
- Preserve identifiers, monetary precision, original date values, and currency labels. Use `json.Number`/raw JSON where needed rather than decoding arbitrary numbers into float64. Retain monetary strings; use an exact decimal library if local calculations become necessary.
- Separate envelope parsing for records, companies, evidences, properties, relations, reports, and mutation results. Normalize string flags only where their schemas call for it.
- Cache metadata per instance/company/authenticated identity with a configurable TTL and explicit refresh. Record data remains fresh by default. Do not reuse schema visibility across credentials.
- Retry transient reads with bounded backoff. Surface uncertain mutation outcomes with identifiers sufficient for reconciliation. Never promise exactly-once writes.
- Model create-only as an explicit import mode that fails if already present; model update-only against existing identifiers. Keep nested operations within each dedicated service's supported scope.
- Return application warnings/errors even on HTTP 200. Separate authentication, permissions/licensing, unsupported capability, validation, conflict, and transport failures.
- Use structured tool outputs plus a concise text rendering for client compatibility. Include `company`, evidence, timestamp, filter/paging context, `complete`/`truncated` indicators where applicable.
- Keep stdout exclusively for MCP protocol traffic. Log redacted diagnostics to stderr, without response bodies or credentials by default.

## 6. Skills and guidance

Canonical release directories under `skills/`:

| Skill | Planned milestone | Workflow |
| --- | --- | --- |
| `flexi-explore` | M1 | Choose company, discover evidence/schema, construct bounded queries, explain source context |
| `flexi-receivables` | M1 | Identify overdue/unpaid invoices, account for partial payments/currency and query completeness |
| `flexi-invoice` | M2 | Resolve customer/type/tax references, preview, create or update, read back result |
| `flexi-reconcile` | M3 | Find payment candidates and use explicit matching/remainder policies |
| `flexi-inventory` | M3 | Inspect stock context and apply tested stock/order workflows |
| `flexi-accounting-reports` | M3 | Resolve report-specific periods, posting status, grouping, currencies, and output formats |

Use standard `name`, `description`, optional compatibility metadata, and portable Markdown instructions. Keep client-specific features out of the canonical skill body. Document how actual client tool-name prefixes map to the server's tool names.

Skills call connected tools; they do not contain secrets or require direct REST calls from a cloud code-execution sandbox. They should explain how to distinguish business data from instructions and how to report ambiguous company/period/document references.

Distribute individual skill ZIPs for Claude and directories for CLI clients. Claude Code and OpenCode can both discover `.claude/skills`; OpenCode also has native `.opencode/skills` and `.agents/skills`. Installation should be explicit and leave existing client configuration intact.

Expose corresponding guidance as MCP prompts and `flexi://guides/{topic}` resources, with `flexi_get_guide` as a tool-accessible path. Useful prompt proposals: `review_receivables`, `prepare_invoice`, and `inspect_inventory`. Resource discovery/display is client-dependent, so it is not the only route to required guidance.

## 7. Client distribution and hosting

Initial distribution: versioned native Windows 11/Linux x86-64 executables, portable skills, and three separate client setup examples. Proposed command: `flexibee-mcp serve --transport stdio` (not implemented yet). Windows runs `flexibee-mcp.exe` directly without WSL or a language runtime. Verify Linux on Debian 13, selected openSUSE and Fedora releases. macOS, Linux OCI image and hosted HTTP follow later.

- **Claude Desktop:** command + args + env in its `mcpServers` configuration, absolute executable paths where necessary; test on a supported Desktop OS. Linux developer tests alone cannot establish Desktop compatibility.
- **Claude Code:** project `.mcp.json` or `claude mcp add`; document credential injection without committing values.
- **OpenCode:** `mcp` entry using `type: local`, command array, and `environment`; verify against the actual installed version.

Later distribution can include a Desktop extension and Claude Code plugin bundling MCP setup with skills. These are packaging layers over the same server.

Streamable HTTP hosting is independent of Flexi Cloud support: a local stdio server can call Flexi Cloud, and a hosted MCP can call a self-hosted Flexi instance if its network permits. Only stdio belongs in M1.

### Future remote deployment design — backlog

```text
Claude Code / OpenCode ---- HTTPS --------------------+
                                                     |
Claude Desktop remote connector                      v
  -> Anthropic infrastructure -> HTTPS -> Ingress/Gateway
                                                     |
                                              ClusterIP Service
                                                     |
                                      Go MCP Deployment (/mcp)
                                        | auth + request scope
                                        | shared application layer
                                        v
                                   private Flexi HTTPS endpoint

Local Desktop/Code/OpenCode -> stdio binary -> LAN/VPN Flexi
```

Claude's official remote-connector documentation says requests originate from Anthropic infrastructure even when configured in Desktop. Therefore that connector needs an externally reachable HTTPS MCP endpoint. The Flexi endpoint can remain private. Direct CLI HTTP connections can use the client's own VPN/network; document these paths separately.

Proposed remote authentication:

- For an initial owner-operated deployment, a dedicated fixed bearer credential supplied through client request headers is the simplest proposal. Current Claude connector documentation explicitly describes request-header credentials; verify the installed client's UI and configuration.
- Keep this token separate from upstream Flexi credentials and resolve its company scope server-side. Never pass incoming MCP tokens to Flexi.
- If individual identities are required, use an external OAuth authorization server with the MCP acting as resource server. Implement protected-resource metadata and token issuer/audience/scope verification. Select and test client registration support (published identity, pre-registration, or legacy DCR) against the chosen provider.
- An ingress login-page redirect alone is not an MCP OAuth implementation. The SDK does not supply a complete identity provider. Authentication model needs confirmation before remote implementation.

Use the SDK's stateless HTTP mode for the initial request/response tools, resources and prompts. Carry caller identity and resolved company in request context, not globals. Verify legacy initialize-based clients and the current protocol against the pinned SDK; do not build a custom protocol stack. Avoid server-initiated requests/subscriptions in M1.

### Kubernetes and service lifecycle

- The future hosted-HTTP milestone includes a Linux container and Helm/deployment examples: Deployment, Service, optional Ingress, ConfigMap, references to existing Secrets, private-CA mount, probes and configurable resource limits.
- Start with one replica. Stateless operations can support additional replicas after cross-replica tests; upstream concurrency/request budgets must account for the total replica count.
- Reuse one bounded upstream HTTP pool per credential context. Propagate cancellation, deadlines and shutdown signals. Expire metadata caches independently per replica.
- `/health/live` checks process health; `/health/ready` checks initialization/readiness. A temporary Flexi outage must not cause a liveness restart loop. Avoid quota-consuming upstream calls on every probe.
- Handle SIGTERM with readiness withdrawal and bounded request draining; exercise native Windows shutdown separately. Configure proxy buffering/timeouts for any negotiated streamed responses.
- Run non-root, keep CA roots available, and use mounted secrets/configuration. Determine CPU/memory requests from profiling rather than language-based estimates.
- Later artifacts must be reproducible from scoped references or use shared storage before multi-replica artifact serving. Local temporary files cannot be assumed available on another pod.

Native Debian HTTP hosting can use systemd with the same executable. Native Windows supports running HTTP as a foreground process initially; a Windows Service installer is a separate packaging enhancement.

## 8. Verification strategy

1. **Adapter contracts:** fixture-based tests for actual response roots/types, filter/ID encoding, redirects, pagination, body-level errors, permission/licensing failures, decimal/date preservation.
2. **MCP integration:** launch a real stdio child process, negotiate with SDK clients, enumerate tools/prompts/resources, call against a mocked Flexi HTTP service, verify stderr/stdout separation and upstream authentication. Add HTTP listener/auth/isolation tests with the future remote milestone.
3. **Workflow tests:** sandbox-only invoice preview/save/readback, repeated external identifiers, partial payments, item updates, attachments, report parameter semantics.
4. **Human-facing skill evaluation:** task scenarios with expected tool usage and grounded answers, including partial pages and ambiguous context.
5. **Client matrix:** named versions of Claude Desktop, Claude Code, and OpenCode over stdio in M1. Windows 11 native client tests; Debian 13/openSUSE/Fedora tests for the server and available CLI clients. Do not imply an official Claude Desktop Linux release. HTTP gets a separate later matrix.
6. **Deployment matrix:** at least the user's actual self-hosted version and one cloud tenant before declaring cloud support; custom CA and reverse-proxy tests.

Ordinary CI must not depend on or mutate the shared public demo. Live integration tests are opt-in and require an explicitly configured disposable company.

Platform verification includes Windows paths with spaces/Unicode, Linux filesystem permissions, system and private CA roots, process launch/shutdown, and proxy behavior. Confirmed: x86-64 and Debian 13. openSUSE edition/version and Fedora release need specification. macOS gets its own later runtime/client test matrix. See [language comparison](language-comparison.md) for sources and measurement plan.
