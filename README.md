# flexibee-mcp

An MCP server project for **ABRA Flexi (formerly FlexiBee)**, initially targeting self-hosted instances and use in **Claude Desktop, Claude Code, and OpenCode**.

## Project status

**Design agreed — 2026-10-08.** Go, local stdio, and one account per instance are approved. Implementation has not started. No package or release is published.

**Confirmed:** Go and x86-64, targeting **Windows 11, Debian 13, openSUSE, and Fedora**, with **macOS later**. A **local stdio MCP process connects to a remote FlexiBee server using one FlexiBee account**. Multi-profile support and hosted MCP HTTP are backlog features. See the [latest decisions](docs/decisions.md).

The project will provide:

- Metadata-driven discovery and querying of Flexi evidences (business record collections).
- Business workflows for invoices, contacts, products, orders, banking, inventory, and accounting, delivered incrementally.
- Portable Agent Skills, plus MCP prompts and resources for workflow guidance.
- A configurable Flexi API connection that can accommodate self-hosted and cloud deployments.

## Research and plan

| Document | Contents |
| --- | --- |
| [Latest decisions](docs/decisions.md) | Approved language/platform and clarified local-MCP deployment |
| [API research](docs/research.md) | Official sources, API semantics, capability map, live demo observations, unresolved questions |
| [Language comparison](docs/language-comparison.md) | Go/Rust/Python/TypeScript/C#, official SDK evidence, OS distribution and recommendation |
| [Architecture](docs/architecture.md) | Proposed stack, tools, adapter contracts, skills, deployment and client compatibility |
| [Roadmap](docs/roadmap.md) | Milestones, acceptance criteria, scope and decisions needed |

Cloud API compatibility is a design goal, **not yet a tested support claim**. Likewise, client compatibility will be verified against named client versions before release.

Repository: [opentreecz/flexibee-mcp](https://github.com/opentreecz/flexibee-mcp).

License: GNU Affero General Public License v3.0; see [LICENSE](LICENSE). Package namespace is not yet selected.
