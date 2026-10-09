# ABRA Flexi API research

Research date: **2026-10-08**. Scope: official Flexi documentation, a bounded read-only public-demo probe, official MCP/client/Agent Skills documentation. This is an implementation-oriented survey of the API surface, not certification of every endpoint or every historical Flexi version.

**Follow-up:** the original SDK discussion below records the first research pass. The [language comparison](language-comparison.md) adds Go/Rust/Python/C# research. [Latest decisions](decisions.md) confirm Go, local stdio, one account per instance, and x86-64 Windows 11/Debian 13/openSUSE/Fedora; macOS, multi-profile and hosted HTTP follow later. Client cloud-origin behavior and future remote auth are detailed in the [architecture](architecture.md).

Labels used below: **documented** = stated in the cited vendor documentation; **observed** = returned by the public demo during this session; **proposed** = our design choice; **unverified** = needs an integration test or deployment-specific confirmation.

## 1. Product and deployment model

ABRA Flexi is the accounting/economic software formerly known as FlexiBee. It exposes company-scoped REST APIs covering commercial documents, contacts, stock, accounting, and additional licensed modules. It is distinct from ABRA Gen; Gen's API documentation is not a substitute.

Self-hosted and vendor-cloud deployments use the Flexi API model. A configurable origin and company identifier should allow the same adapter to target either. Differences include versions, licenses, roles, network reachability, TLS certificates, and server configuration. Cloud equivalence remains unverified against a customer cloud tenant. [F01–F04, F18]

Self-hosted examples commonly use port `5434`; this must not be hardcoded. Reverse proxies may expose HTTPS on 443 and a path prefix. The MCP process needs network access to Flexi, so a locally spawned MCP is suitable for LAN/VPN-only instances.

### Licensing and request budgets

The licensing article says API access is a purchased entitlement, allowing unlimited separate API users. It states that API use without that entitlement became unavailable from 2025-01-01. The current pricing table lists daily allowances of 5,000 / 10,000 / 20,000 requests across its variants, with larger allowances purchasable for some variants. Do not assume a legacy self-hosted license has the same limits. [F02, F25]

Some older/general endpoint documentation describes read-only licensing differently. Resolve the actual entitlement on the target installation rather than inferring it from one HTTP code or a generic article. `402`, `403`, and even `404` can reflect licensing/availability, not merely a missing record. [F02, F12, F17]

Design consequence: cache metadata, use explicit projections, avoid startup-wide schema crawls, and keep paging/request budgets bounded.

## 2. API addressing and discovery

Documented addressing: [F03]

```text
/c/{company}/{evidence}.{format}
/c/{company}/{evidence}/{id}.{format}
/c/{company}/{evidence}/({filter}).{format}
/c/{company}/{evidence}/properties.json
/c/{company}/{evidence}/relations.json
/c/{company}/{evidence}/reports.json
/c/{company}/{evidence}/{id}/{relation}.json
```

Observed company and evidence discovery:

```text
GET /c.json
GET /c/demo/evidence-list.json
```

Use the company database identifier (`dbNazev` in the observed company response), not its display name. The documentation also links newer `/flexi/...` web UI routes; API requests should use the documented `/c/...` API routes.

`properties` reflects permissions and licensing. It describes property names, data types, writable/mandatory flags, relations, and other metadata. Treat this as helpful schema information, not a complete description of all accounting business rules. Server validation remains authoritative.

### Live read-only observations

Public host: `https://demo.flexibee.eu`; requests used HTTPS with no supplied credentials. No mutations, dry-run writes, or webhook registrations were performed.

| Request | Result | Observation |
| --- | --- | --- |
| `/c.json` | 200 JSON | Root `companies`, array `company`, database identifier `dbNazev` |
| `/c/demo/evidence-list.json` | 200 JSON | Root `evidences`; **253** entries |
| `/c/demo/faktura-vydana/properties.json` | 200 JSON | Root `properties`; array `property`; booleans such as `isWritable` encoded as strings |
| `/c/demo/faktura-vydana/relations.json` | 200 JSON | Root `relations`; invoice item relation URL is `polozkyDokladu` |
| `/c/demo/faktura-vydana/reports.json` | 200 JSON | Root `reports`; report IDs, languages, default flags |
| `/c/demo/adresar.json?limit=1&detail=custom:id,kod&add-row-count=true` | 200 JSON | Root `winstrom`; one record; `id` and `@rowCount` are strings |

Evidence import statuses observed: `SUPPORTED`: **44**; `NOT_DOCUMENTED`: **122**; `NOT_DIRECT`: **28**; `DISALLOWED`: **59**. These counts describe this demo at this time, not a universal API guarantee. A discoverable evidence is not automatically writable, and metadata status alone does not prove read/write permission.

Two important mismatches:

- Invoice relation URLs expose `polozkyDokladu`, while documented invoice import examples use `polozkyFaktury`. Read-navigation paths and write payload fields need separate mappings.
- Some metadata links advertised plain `http://` despite the HTTPS request. Construct requests from the configured trusted base URL rather than following arbitrary metadata URLs.

## 3. Authentication

Documented options: [F04]

- HTTP Basic authentication; `auth=http` can force API authentication behavior rather than an HTML login redirect.
- JSON session login: `POST /login-logout/login.json` with username/password and optional OTP. Returns success, `authSessionId`, `refreshToken`, and `csrfToken` in current documentation.
- Session authentication can use `X-authSessionId`, a cookie, or a query parameter. Prefer a header; the vendor explicitly notes that query tokens appear in server logs.
- Session keepalive is documented at `/login-logout/session-keep-alive.js`; refresh uses a cookie with `/login-logout/check`.
- With two-factor authentication enabled, Basic authentication requires a current OTP on **each request**. Session login accepts OTP. An unattended static-OTP configuration is not a solution.
- SAMLv2/OpenID support is documented for local installations with licensing restrictions. These are not evidence of a generic OAuth2 REST API flow.

Proposal: start with a dedicated API user's Basic credentials over verified TLS. Provide a clear unsupported-auth-mode diagnostic for an OTP/SSO setup until its session lifecycle is implemented and tested. Keep Flexi credentials separate from any future MCP HTTP-client authentication.

## 4. Formats and response shapes

JSON and XML are principal structured interchange formats; endpoints also provide CSV, PDF, XLS/XLSX, and document-specific formats such as ISDOC. Format availability depends on the endpoint. JSON is the proposed default; binary exports need distinct handling. [F01, F05, F15]

Typical records are under `winstrom`, but discovery uses other roots (see probes). Attributes may appear as `@version`, `@rowCount`, `@action`, or `field@attribute`. The documented payment payload also uses `@content`. Do not assume arbitrary XML examples can be transformed by simply renaming tags. [F14, F21]

Preserve decimal strings and identifiers. Normalize known booleans deliberately: JavaScript `Boolean("false")` is wrong. Distinguish missing fields, empty values, and explicit nulls. Use date-only values for business dates and retain offsets on timestamps; avoid converting business dates through UTC and shifting a day.

## 5. Queries, paging, and aggregation

- `limit` defaults to **20**; `0` means unlimited. `start` is an offset independent of the limit. `add-row-count=true` requests a filtered count. [F07]
- Use `detail=custom:...` for selected fields. Includes and relations expand related data. Metadata lookup should precede field selection. [F03]
- Filters are Flexi expressions, not SQL: comparisons, `and`/`or`/`not`, `like`, `like similar`, `in`, null/empty tests, dates, `now()`, and relationship traversal. Encode the expression correctly as a URL component. [F06]
- Negative comparisons on nested relations exclude rows without that relation. Missing values need explicit treatment.
- To query a document's item collection, query the corresponding item evidence or relation; arbitrary collection traversal is not a SQL-style join.
- Evidences with `platiOd`/`platiDo` are implicitly filtered by the current accounting period; `filtrovat-platnost=false` changes that. Historical lookups must be explicit.
- `POST /{evidence}/query.json` is a **read** operation supporting filters and parameters in a `winstrom` body. Its `limit` must be in the body; URL `limit` is ignored. Thus read/write classification cannot be based on HTTP verb alone. [F08]
- `/$sum.json` is supported for documents, not every evidence; other evidences can return 404. `obrat` has specialized required `period`, `fields`, and `group-by` parameters. [F09]

Proposal: bounded pages (default 50, maximum 200 initially), stable sorting, explicit next-offset and truncation indicators. Offset paging is not a consistent snapshot under concurrent edits. Reports must identify their filter, period, currency, and completeness; a page subtotal must not masquerade as the full ledger total.

## 6. Identifiers and writes

Flexi treats **POST and PUT equivalently** for normal record imports. The target URL and identifiers control create/update semantics. Collection imports can upsert; detail updates require an existing record. [F05]

Identifiers include numeric internal IDs, `code:`, `ext:SYSTEM:ID`, `key:`, EAN, PLU, VAT/company identifiers, and hybrid IDs. Support varies by evidence. Codes may change; external IDs are unique within an evidence. Non-internal identifiers may return a **301 redirect** to the internal-ID URL when read. Identifier encoding must handle slashes and Unicode. [F10]

Record modes `create`/`update` accept `ok`, `ignore`, or `fail`. Use explicit semantics for create-only and update-only tools; do not infer them from POST versus PUT. An ignored import can return 200 with no created ID. [F11]

Updates are incremental: omit untouched fields. Empty fields can clear values. Line-item IDs are important; the incremental-update documentation contains differing descriptions of unidentified-item behavior (position-based versus append). Treat this as unresolved and require identifiers for updates. `removeAll=true` deletes omitted items. [F24]

`action=delete` and `action=storno` are separate business actions. Nested actions can delete items through a parent import. A generic writer must not accidentally bypass its own operation scope through nested action fields. [F14]

### Validation and transactional behavior

- `dry-run=true` performs validation and returns the calculated representation without saving. A preview's document number can differ from the eventual saved number. It is not a reservation or an atomic preview-and-commit transaction. [F13]
- `fail-on-warning=true` can make warnings prevent persistence. Return warning details even when successful. [F03]
- A normal import is atomic by default. `atomic=false` permits per-record transactions and partial success. Atomicity applies to that import request, not a series of MCP calls. [F16]
- Flexi calculates domain results such as taxes and totals; avoid reimplementing the accounting engine in the MCP.
- An external ID can help reconcile an uncertain create result, but does not make every workflow idempotent. Never blindly retry a timed-out mutation.

## 7. Errors and operational behavior

Documented codes include 400 validation, 401 authentication, 402/403 licensing or permission problems, 404 absence/availability, 405 unsupported method, 406 unsupported output/report, 409 conflicting operation, and 503 maintenance. Error bodies normally carry structured details; HTML proxy/login errors are also possible in deployment. [F12]

Inspect both HTTP status and application success/error/warning fields. Avoid `no-http-errors=true`, which can turn failures into HTTP 200. Back off on transient reads, respect any `Retry-After`, and report mutation timeouts as an unknown outcome requiring reconciliation.

The consulted sources do not establish universal rate-limit response headers, timeout values, or a maximum normal list page size. These must remain configurable/tested rather than invented API guarantees.

## 8. Capability map and opportunities

The demo catalog confirms substantial breadth. Items marked “catalog” below are discovery observations, not verified end-to-end operations. Dedicated advanced articles are cited where researched.

| Area | Evidences / operations | Useful MCP workflows | Research depth / proposed phase |
| --- | --- | --- | --- |
| Company and schema | companies, evidence list, properties, relations | Inspect instance capabilities; explain fields | Live reads; M1 |
| Contacts / CRM | `adresar`, `kontakt`, `udalost`, tags, address bank accounts | Customer/supplier lookup, contact maintenance | Catalog + core semantics; M1–M2 |
| Invoicing | `faktura-vydana`, `faktura-prijata`, lines, types, series | Find overdue invoices; preview/create invoice; inspect balances | Core docs + live metadata; M1–M2 |
| Sales / purchasing | offers, inquiries, received/issued orders | Order-to-invoice; partial fulfillment | Catalog + realization docs [F22]; M3 |
| Catalog / stock | `cenik`, `skladova-karta`, `skladovy-pohyb`, reservations, batches, serials, inventory | Product lookup, stock availability, movements | Catalog; historical/fulfillment details require tests; M1 reads/M3 workflows |
| Banking / cash | `banka`, `pokladni-pohyb`, payment orders, matching | Payment candidate analysis; reconciliation; statement imports | Matching researched [F21]; other advanced imports backlog; M3 |
| Accounting | `ucetni-denik`, `hlavni-kniha`, `obratova-predvaha`, `stav-uctu`, `saldo` | Ledger queries, account balances, period reports | Accounting output docs [F20]; M3 |
| Financial statements / tax | balance sheet, P&L, cash flow, VAT evidence and reports | Retrieve configured statements and tax-support data | Catalog + output docs; module-specific verification; M3+ |
| Contracts | customer/supplier contracts and invoice generation | Recurring billing assistance | Catalog + documentation index; backlog |
| Assets / leasing | `majetek`, `leasing`, depreciation records | Asset lookup and summaries | Catalog; backlog |
| Payroll / personnel | specialized API articles in official index | Personnel/payroll integration | Index only; entitlement/schema unknown on target; backlog |
| Documents | PDF, XLSX, ISDOC.PDF, attachments | Fetch invoice PDF; attach source documents | Detailed docs [F15, F19]; M2 |
| Customization | saved queries, attributes, tags, user relations | Run named business reports | Saved queries researched [F23]; M3+ |
| Workflow | Activiti integration, workflow starts/tasks | Configured approval/task processes | Partner-dependent [F26]; backlog |
| Synchronization | Changes API, webhooks | Incremental caches, event-driven integration | Detailed docs [F17–F18]; M4 |
| Administration | company creation/restore, users, roles, settings | Deployment administration | Index only; separate future scope |

### Important business-specific constraints

**Payments:** explicit matching supports multiple invoices of the same invoice type, partial amounts, different remainder policies, and currency-related behavior. `zauctovat` can create an internal document. Automatic matching also has accounting side effects and its own defaults. Prefer dedicated input schemas that make these options explicit. [F21]

**Order realization:** supports invoice, stock movement, and sales-document targets; can be partial; quantities use base units rather than package counts. Defaults and document-type settings affect generated stock movements. Treat this as a workflow, not a generic field patch. [F22]

**Accounting reports:** unknown parameters can be silently ignored with HTTP 200. Period identifier formats differ by report (for example some require `code:` and others do not). Posting-state defaults can vary with legislation. Use report-specific parameter adapters. Some statements require prior calculation. [F20]

**Saved queries:** execution uses `/uzivatelsky-dotaz/{id}/call.json`, with results under `winstrom.DotazView`. This is different from ordinary evidence `/query.json`. Start with named, configured queries rather than an arbitrary SQL tool. [F23]

**Workflow:** the vendor refers interested users to integration partner ARIT; the Activiti API is adapted to Flexi authentication and user management. Do not assume every installation has it enabled. [F26]

## 9. Reports and attachments

Discover report IDs rather than hardcoding `faktura`. Request `.pdf`/`.xlsx` with `report-name` and appropriate language. The public demo returned more languages than the general article lists, reinforcing instance-level discovery. PDF invoice output can contain embedded ISDOC. [F15]

Attachment metadata lives under `/{record}/prilohy`; bytes under `/{attachment}/content`. Binary upload uses `PUT .../prilohy/new/{filename}` with Content-Type and raw bytes. Existing attachment content is not updated in place; replacement involves deletion and recreation. Metadata updates are distinct. [F19]

Proposal: return bounded metadata and an artifact handle, with a tool-based retrieval path as well as MCP resources. Avoid putting large base64 PDFs into ordinary query results or assuming a server-local filesystem path is usable by a remote client.

## 10. Changes and webhooks

Changes API tracks monotonically increasing company versions; gaps are allowed. It reports creates, updates, and deletes, with `next` continuation and `globalVersion`. `start` is inclusive, default page size is 100, and maximum is 1,000. Unknown evidence filters are silently ignored, potentially returning all evidences. Validate names locally. [F17]

Activation changes database structure and requires an exclusive lock. A capability check should inspect status, not enable tracking automatically.

Self-hosted webhooks require `<entry key="enableHooks">true</entry>` in `flexibee-server.xml`; cloud enables this server setting. Changes tracking is still a prerequisite. Hooks send all evidence changes, so receivers filter. `X-FB-Hook-SecKey` carries an optional shared secret; it is not a documented cryptographic payload signature. [F18]

Delivery can be duplicated or delayed. Persist quickly, deduplicate versions, and process asynchronously. The vendor recommends processing below 15 seconds and requires completion within 30 seconds. Webhook delivery does not count toward the published daily API request limit.

A short-lived stdio process is not a durable webhook receiver. Later event processing needs a persistent service and a reachable callback URL; polling Changes API is the simpler first synchronization path.

## 11. MCP, clients, and skills

MCP provides tools, resources, and prompts. Portable Agent Skills are separately packaged directories with `SKILL.md`, optionally references/assets/scripts. There is no universal guarantee that connecting an MCP installs filesystem skills. [M01, M06]

| Client | Initial MCP connection | Skill delivery | Verification needed |
| --- | --- | --- | --- |
| Claude Desktop | Local stdio via `claude_desktop_config.json`; optional extension packaging later | Upload skill ZIP through Claude's Skills UI, subject to account settings | Actual Desktop version/OS and skill + local-tool workflow |
| Claude Code | `claude mcp add ... --transport stdio ... -- <command>` or project `.mcp.json` | `.claude/skills/<name>/SKILL.md`; plugin packaging later | Startup, tool results, prompts/resources and workflows |
| OpenCode | `mcp.<name>` with `type: local`, command array, environment | `.opencode/skills`, `.claude/skills`, or `.agents/skills` | Version-specific behavior; docs currently also advertise v2 |

Claude Skills documentation specifies code execution requirements and ZIP upload. Our skills should use connected MCP tools, not scripts assuming Claude's hosted execution environment can directly reach a private Flexi LAN. [M02–M07]

The current official TypeScript SDK repository describes **v2** as stable, with separate `@modelcontextprotocol/server` and `@modelcontextprotocol/client` packages. v1's `@modelcontextprotocol/sdk` is a separate maintenance line. Pin and test the chosen stable version at implementation time, including older-client protocol negotiation. [M08]

## 12. Ecosystem and evidence limits

A GitHub repository search using `gh search repos 'flexibee mcp' --limit 10` returned no matches in this session. That is a narrow search, not proof that no existing connector exists. The official documentation index links integrations and PHP examples; a third-party wrapper evaluation has not been completed. A thin direct HTTP adapter is proposed because the difficult part is domain behavior and client presentation, not sending REST requests.

No target self-hosted instance, credentials, version, license, or disposable company was supplied. Authentication failures, OTP refresh, mutations, PDF bytes, payment actions, concurrency, cloud-tenant behavior, and actual client integration remain untested.

## Sources

All sources below were accessed on 2026-10-08. The documentation index contains 235 articles at access time; advanced areas marked index-only above were not individually validated.

- **F01** [Official API documentation index](https://www.flexibee.eu/api/dokumentace/)
- **F02** [API licensing](https://podpora.flexibee.eu/cs/articles/10097467-licencovani-pristupu-k-api)
- **F03** [URL structure and parameters](https://podpora.flexibee.eu/cs/articles/4713911-sestavovani-url)
- **F04** [Authentication](https://podpora.flexibee.eu/cs/articles/4713880-autentizace)
- **F05** [HTTP operations](https://podpora.flexibee.eu/cs/articles/4720093-podporovane-http-operace)
- **F06** [Filtering](https://podpora.flexibee.eu/cs/articles/4722195-filtrovani-zaznamu)
- **F07** [Pagination](https://podpora.flexibee.eu/cs/articles/4722193-strankovani)
- **F08** [POST query](https://podpora.flexibee.eu/cs/articles/5264924-pouziti-query-v-rest-api)
- **F09** [Aggregation](https://podpora.flexibee.eu/cs/articles/4722199-sumace)
- **F10** [Record identifiers](https://podpora.flexibee.eu/cs/articles/4725798-identifikatory-zaznamu)
- **F11** [Create/update modes](https://podpora.flexibee.eu/cs/articles/4725944-rezim-pro-zalozeni-zmenu)
- **F12** [Error handling](https://podpora.flexibee.eu/cs/articles/4720060-obsluha-chyb)
- **F13** [Dry run](https://podpora.flexibee.eu/cs/articles/4720123-testovaci-ulozeni-dry-run)
- **F14** [Actions](https://podpora.flexibee.eu/cs/articles/4725960-provadeni-akci)
- **F15** [PDF/XLSX reports](https://podpora.flexibee.eu/cs/articles/4720040-export-tiskovych-sestav-pdf-xlsx)
- **F16** [Transactions](https://podpora.flexibee.eu/cs/articles/4726125-transakcni-zpracovani)
- **F17** [Changes API](https://podpora.flexibee.eu/cs/articles/4744362-changes-api)
- **F18** [Webhooks](https://podpora.flexibee.eu/cs/articles/4744379-web-hooks)
- **F19** [Attachments](https://podpora.flexibee.eu/cs/articles/4722200-prilohy)
- **F20** [Accounting outputs](https://podpora.flexibee.eu/cs/articles/4971496-ucetni-vystupy-v-rest-api)
- **F21** [Payment matching](https://podpora.flexibee.eu/cs/articles/4729375-parovani-plateb)
- **F22** [Order realization](https://podpora.flexibee.eu/cs/articles/4744429-realizace-objednavky)
- **F23** [Saved user queries](https://podpora.flexibee.eu/cs/articles/4744351-uzivatelske-dotazy)
- **F24** [Incremental updates](https://podpora.flexibee.eu/cs/articles/4725908-inkrementalni-aktualizace)
- **F25** [Current pricing and API allowances](https://www.abra.eu/flexi/cenik/)
- **F26** [Workflow and Activiti](https://podpora.flexibee.eu/cs/articles/4726188-workflow-a-procesy)
- **L01** [Live demo evidence catalog](https://demo.flexibee.eu/c/demo/evidence-list.json)
- **L02** [Live invoice properties](https://demo.flexibee.eu/c/demo/faktura-vydana/properties.json)
- **L03** [Live invoice relations](https://demo.flexibee.eu/c/demo/faktura-vydana/relations.json)
- **L04** [Live invoice reports](https://demo.flexibee.eu/c/demo/faktura-vydana/reports.json)
- **M01** [MCP server concepts](https://modelcontextprotocol.io/docs/learn/server-concepts)
- **M02** [Claude Desktop local MCP setup](https://modelcontextprotocol.io/docs/develop/connect-local-servers)
- **M03** [Claude Code MCP](https://code.claude.com/docs/en/mcp)
- **M04** [OpenCode MCP](https://opencode.ai/docs/mcp-servers/)
- **M05** [OpenCode skills](https://opencode.ai/docs/skills/)
- **M06** [Agent Skills specification](https://agentskills.io/specification)
- **M07** [Claude skills](https://support.claude.com/en/articles/12512180-using-skills-in-claude) and [Claude Code skills](https://code.claude.com/docs/en/skills)
- **M08** [Official TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk) and [v2 documentation](https://ts.sdk.modelcontextprotocol.io/v2/)
