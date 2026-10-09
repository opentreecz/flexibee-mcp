# Development roadmap

Status: Go/local-stdio/account direction approved, 2026-10-08; implementation pending. See [latest decisions](decisions.md).

Confirmed requirements: Go, x86-64, Windows 11/Debian 13/openSUSE/Fedora, macOS later. Local stdio in M1, one FlexiBee account per process. Hosted HTTP and multi-profile support are backlog features.

## M0 — Research and design

- [x] Inspect the empty repository and existing vault conventions.
- [x] Survey official API/client/skill documentation.
- [x] Probe public demo metadata and a bounded read query.
- [x] Record architecture, capability map, uncertainties, and roadmap.
- [x] Approve Go, local stdio, x86-64, and one account per instance.

## M1 — Usable read-only MCP and first skills

Prepared as [Phase 1](phase-1.md) with work packages, tool contracts, risks and a definition of done. Implementation has not started; the Go toolchain is not yet installed on the development machine.

Deliver:

- Go project, pinned official MCP SDK, native release builds and meaningful CI checks.
- Validated configuration, Basic authentication, verified TLS/custom CA support, bounded HTTP client.
- Company/evidence discovery, schema descriptions, record/relation queries, supported document sums.
- Distinct metadata/record parsers, string flag handling, precise monetary/date representations.
- Local stdio server with the compact tool surface in [architecture](architecture.md).
- Windows 11/Linux x86-64 release artifacts, verified on Debian 13 and selected openSUSE/Fedora releases.
- `flexi-explore` and `flexi-receivables` skill packages, MCP guidance/prompt/resource access.
- Installation examples for all three requested clients.

Acceptance:

- A user can select a company, discover invoice fields, find overdue invoices, and distinguish page results from complete totals.
- Normal record reads work across discovered supported evidences without generating a tool per evidence.
- Unknown fields/relations, invalid configuration, HTML login responses, and unavailable licensed capabilities produce actionable errors.
- Contract tests and stdio protocol tests pass; upstream auth/company behavior and runtime/build checks pass.
- Native Windows tests plus a Windows 11/client smoke test; Debian 13/openSUSE/Fedora tests on x86-64. Record actual OS/client versions.
- Local clients launch the MCP and query the remote FlexiBee server using their configured account.
- An actual self-hosted smoke test is recorded, followed by client-version-specific smoke results. Any unavailable environment is labeled unverified rather than passed.

## M2 — Invoice/contact writes and document handling

- Server-side dry-run preview and explicit create/update semantics.
- Outgoing invoice and contact creation/update; incoming invoices after dedicated coverage.
- Preserve line identifiers; external-ID reconciliation for uncertain outcomes.
- Export discovered PDF/XLSX reports; attachment list/download and bounded artifact access.
- Attachment upload as a separate operation once its request/response contract is tested.
- `flexi-invoice` skill, fixtures, sandbox integration scenarios.

Acceptance:

- Preview does not persist a record; saved output is read back and checked.
- Repeated create identifiers do not silently overwrite existing invoices.
- Validation warnings are visible, and write timeouts are not silently retried.
- PDF retrieval and artifact consumption work in the requested clients.

## M3 — Business operations and financial reporting

- Payment candidate analysis, explicit matching/unmatching, partial-payment and currency handling.
- Received/issued order flows and partial order realization.
- Stock availability and tested stock-movement operations.
- Ledger, account balance, trial balance, P&L/balance-sheet adapters with endpoint-specific parameters.
- Named saved-query execution and domain skills.

Acceptance: fixtures and disposable-company scenarios prove accounting side effects, record links, filters, periods, and returned totals. Scope each domain to the modules actually licensed on the test installation.

## M4 — Cloud certification, platform expansion, and synchronization

These are independent tracks; cloud certification can be brought forward once a tenant is available.

- **Cloud certification:** run the common contract suite against a cloud tenant; document license/request limits and differences.
- **Platform expansion:** macOS native builds, signing/distribution plan and client smoke tests; additional CPU architectures after verification.
- **Remote MCP:** Streamable HTTP, caller authentication, Linux container, Helm/systemd examples, client tests, then multi-replica/shared artifact handling.
- **Multiple profiles:** multiple account profiles per process with explicit selection and isolated credential/cache contexts.
- **Synchronization:** Changes API cursor handling, deletion processing, deduplication, restart persistence; later a durable webhook receiver.
- **Authentication expansion:** tested session/refresh/OTP flows as required by deployments.
- **Packaging:** optional Desktop extension and Claude Code plugin, reproducible skill ZIPs.

## Longer-term opportunities

- Recurring contracts and billing; bank-statement imports and payment-order export.
- Advanced VAT/report exports, fixed assets/leasing, payroll/personnel modules.
- Custom attributes, user relations, and partner-provisioned workflows.
- Dedicated administration features such as company lifecycle and backup/restore, if requested.
- Multi-instance profiles, historical-version compatibility, richer Czech/English workflows.

## Decisions and inputs

### Needed to begin M1

Go, x86-64, local stdio and one account per process are approved. The read-only foundation and two skills remain the initial delivery plan, detailed in [Phase 1](phase-1.md). Install a Go 1.25+ toolchain before the first work package. Specify openSUSE edition/version and Fedora release before claiming platform verification; this does not block portable core development.

### Needed during development / before integration verification

- Target self-hosted Flexi version, deployment shape (direct or reverse proxy), and API entitlement.
- A disposable test company and dedicated API account, configured locally rather than pasted into chat.
- Which business workflows matter first after read-only access: invoicing, banking, inventory, or accounting reports?
- Actual client versions/operating systems, especially Claude Desktop.
- Repository hosting, package namespace, and license before publication.

No arbitrary release dates are assigned before the target environment and first implementation are established.
