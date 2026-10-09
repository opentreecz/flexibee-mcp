# Language comparison and recommendation

Date: **2026-10-08**. **Follow-up decision: Go approved.** User subsequently clarified local stdio first, one account per process, x86-64, Windows 11/Debian 13/openSUSE/Fedora and macOS later. Hosted HTTP and multi-profile support are backlog features. [Latest decisions](decisions.md) supersede the earlier requirements and delivery proposals recorded below; the language tradeoff research remains applicable.

This revises the initial TypeScript/stdio-first proposal after asking the user about priorities. It incorporates the subsequent OS requirements.

## Requirements supplied by the user

- Priorities: client compatibility, maintainability, performance and footprint.
- Interests: Rust for performance, Go for Kubernetes, Python for readability and feature development.
- Both local stdio and remotely hosted MCP in the first release.
- Windows 11 and Debian Linux first; macOS later.
- Claude Desktop, Claude Code, OpenCode; portable skills; self-hosted Flexi first.

CPU architectures and Debian versions have not yet been specified. Proposed initial targets are Windows amd64 and Debian amd64; Linux arm64 is a useful additional release target once tested. Windows arm64 is not implied by Windows 11 support.

## MCP does not mandate a language

MCP defines JSON-RPC messages, schemas, transports, capabilities, and protocol behavior. The client does not need the server's implementation language installed when the server is delivered as a native executable. All five candidates below have official SDKs. Portable skills are Markdown packages and do not require a Python or TypeScript server.

## Recommendation: Go

Use **Go with the official `github.com/modelcontextprotocol/go-sdk`**, one executable, and two transport entry points sharing the same business logic.

Why it fits this project:

1. Native Windows/Linux releases without a separately installed language runtime.
2. Straightforward HTTP services, cancellation, connection pooling, testing, and concurrency.
3. Generally simpler contributor/build model than an async Rust application, while retaining compiled-code performance.
4. Natural container packaging and a small dependency surface using standard-library HTTP, JSON, TLS, logging, and testing.
5. An official SDK with documented stdio, Streamable HTTP, tools, resources, prompts, and authorization helpers.

Kubernetes is language-neutral: Rust, Python, TypeScript, and C# can all run well in it. Go's benefit is packaging and familiar service tooling, not special Kubernetes compatibility. We do not need an operator or the Kubernetes Go client to deploy this MCP.

Most Flexi operations should be network/upstream-bound. That is an architectural expectation, not a measurement. Rust can reduce local CPU/memory overhead, but it cannot remove Flexi query latency or request quotas. Correct pagination, projections, pooling, and bounded concurrency are likely to matter more for typical interactive use.

## Comparison

Assessments below are engineering judgments, not benchmark results or measured SDK-quality scores.

| Language | Strengths for this MCP | Costs / limitations | Recommendation |
| --- | --- | --- | --- |
| **Go** | Native executable; readable service code; strong HTTP/concurrency tooling; straightforward cross-builds; official SDK | More explicit error and data-mapping code than Python; GC; dynamic schemas still need runtime validation | **Best overall balance for the stated priorities** |
| **Rust** | Native executable; strong type/ownership guarantees; no GC; good control over resource usage; official `rmcp` SDK | Ownership/async/macro complexity; longer build feedback and more involved target toolchains; dynamic Flexi metadata still requires validation | Strong alternative if Rust expertise/footprint takes priority over iteration speed |
| **Python** | Concise readable workflows; easy prototyping; built-in Decimal; official SDK and convenient schema helpers | Interpreter/environment packaging; typically higher runtime overhead; must avoid blocking the async server; frozen binaries add packaging work | Best if business-feature iteration becomes the dominant priority |
| **TypeScript** | Official SDK, schema ecosystem, convenient npm distribution, good async HTTP ergonomics | Node/runtime distribution; build/dependency maintenance; money must not pass through ordinary JS floating-point calculations | Good option, but less aligned with native-binary/footprint preference than Go |
| **C# / .NET** | Official SDK; strong types; Decimal; mature ASP.NET hosting/authentication; Windows/Linux support | Adds an ecosystem the user did not prefer; self-contained distribution is larger than framework-dependent deployment; AOT requires dependency-specific validation | Viable, especially for an existing .NET team; no compelling advantage here |

Do not mix a Rust core, Python workflows, and a Go gateway in the first release. That introduces multiple build chains and cross-language boundaries before a measured need exists. Keep workflow instructions in skills and business operations in one server language.

## SDK evidence checked

Official repositories and GitHub latest-release endpoints were consulted on the date above:

| SDK | Observed release / documentation | Relevant evidence |
| --- | --- | --- |
| Go | Latest release `v1.8.0`, published 2026-09-14; tagged `go.mod` specifies Go 1.25.0 | README documents protocol revisions through 2026-07-28 and older versions; protocol docs cover both transports and stateless HTTP |
| Rust | Latest release `rmcp-v3.5.1`, published 2026-10-05 | README documents Tokio, tools/resources/prompts, transports, OAuth, current and legacy protocol support |
| Python | Latest release `v2.3.0`, published 2026-10-02 | README documents `MCPServer`, stdio/Streamable HTTP/SSE and Python 3.10+ |
| TypeScript | Main README describes v2 as stable | Split server/client packages, stdio/HTTP, schema integrations and auth helpers |
| C# | Official README and package descriptions checked; release version not independently queried | Core/hosting and ASP.NET Core HTTP packages |

Main-branch documentation can move ahead of published packages. Pin a published SDK and supported Go toolchain during implementation and verify its actual APIs. Release observations are not a dependency lockfile.

Go-specific caveats from its own documentation:

- Client-side OAuth has experimental qualifications in the compatibility table. This project primarily needs server-side authentication; do not conflate the two.
- SDK auth helpers are not a complete identity provider.
- The rough-edges document notes tool-annotation boolean serialization and capability defaults; test the actual wire output against all three clients.
- The protocol documentation says current-revision HTTP requires stateless mode, while older compatibility sections retain caveats. Test the pinned package's legacy and current behavior explicitly.

## Platform and distribution proposal

| Target | First-release proposal | Verification |
| --- | --- | --- |
| Windows 11 amd64 | ZIP with `flexibee-mcp.exe`, docs, checksums; native stdio and HTTP modes | Windows-native tests, actual Windows 11/client smoke test, paths with spaces/Unicode, process shutdown, system/private CA handling |
| Debian Linux amd64 | Tar archive with executable; Linux OCI image; systemd example for HTTP | Tests inside selected Debian versions, direct executable and container tests, permissions, signals and CA handling |
| Debian / Linux arm64 | Additional proposed binary/container target | Native/emulated smoke tests clearly identified; availability subject to testing |
| macOS arm64 / amd64 | Later native releases | Separate runners and real client smoke tests; signing/notarization strategy before general distribution |

Propose Debian 12 and 13 as the initial verification matrix, pending the user's actual Debian version. A generic Linux build does not establish Debian-version support. A Windows Server CI runner does not prove every Windows 11 Desktop-client behavior.

Prefer pure-Go dependencies and build with `CGO_ENABLED=0` where supported. This simplifies Linux libc and cross-compilation concerns, but does not remove OS APIs, trusted-certificate needs, or architecture-specific testing. A minimal container must still include trusted CA roots. Kubernetes deployment uses the Linux image; native Windows usage does not require Docker or WSL.

Release output: binaries, OCI image, deployment examples/Helm chart, skill directories and individual ZIPs, checksums and recorded test matrix. Package names and signing/distribution arrangements remain undecided.

## Performance verification plan

No language benchmarks have been run. Do not advertise latency, binary size, or RAM numbers yet.

For the initial Go implementation measure:

- Cold launch to ready MCP tool discovery, idle RSS, artifact size.
- Query latency and peak RSS at bounded concurrency (1, 10, 50 clients) against a reproducible mock API.
- A small schema response, a 200-record response, and later a bounded binary report download.
- Adapter overhead separately from injected upstream latency.
- HTTP authentication overhead, cancellation, timeout behavior, and graceful termination.

Repeat an end-to-end sample against the test Flexi installation. Set container requests/limits from observed behavior. A cross-language benchmark is justified only if Go fails the agreed resource or latency objectives.

## Sources

- [Go SDK](https://github.com/modelcontextprotocol/go-sdk), [v1.8.0 release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0), [tagged go.mod](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/go.mod)
- [Go protocol/transport/auth documentation](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md), [server capabilities](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/server.md), [known rough edges](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/rough_edges.md)
- [Rust SDK](https://github.com/modelcontextprotocol/rust-sdk), [observed release](https://github.com/modelcontextprotocol/rust-sdk/releases/tag/rmcp-v3.5.1)
- [Python SDK](https://github.com/modelcontextprotocol/python-sdk), [observed release](https://github.com/modelcontextprotocol/python-sdk/releases/tag/v2.3.0)
- [TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- [C# SDK](https://github.com/modelcontextprotocol/csharp-sdk)
- [Go target OS/architecture and build documentation](https://go.dev/doc/install/source#environment)
- [MCP HTTP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)
- [Claude remote connectors: network and authentication requirements](https://support.claude.com/en/articles/11175166-getting-started-with-custom-connectors-using-remote-mcp)
- [Kubernetes pod lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/)

See [architecture](architecture.md) for the revised deployment design and [research](research.md) for the Flexi-specific evidence.
