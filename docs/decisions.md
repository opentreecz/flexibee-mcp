# Latest project decisions

Updated: 2026-10-08. These decisions take precedence over the earlier architecture and roadmap proposals where they differ.

## Confirmed by the user

- Language: **Go**.
- Architecture: **x86-64 / amd64**.
- Initial operating systems: **Windows 11, Debian 13, openSUSE, and Fedora**; macOS later. openSUSE edition/version and Fedora release remain to be specified for verification.
- OpenCode and Claude Desktop run a **local MCP instance** that connects to a **remote ABRA FlexiBee server**.
- FlexiBee has multiple user accounts and credentials. Upstream authentication must respect the selected FlexiBee account and its permissions.
- Claude Code remains a requested client from the original requirements.

```text
OpenCode / Claude Desktop / Claude Code
                  |
             local stdio
                  |
       local Go flexibee-mcp process
                  |
      HTTPS + FlexiBee user credentials
                  |
       remote ABRA FlexiBee server
                  |
        account permissions / companies
```

A remote FlexiBee server does not require remote MCP HTTP hosting or MCP-side OAuth. User confirmed **local stdio first**; hosted MCP Streamable HTTP belongs on the backlog.

## Approved account model

Use **one configured FlexiBee account per local MCP process**. Different people configure their own credentials on their machines. A person who needs several accounts can register separate named MCP instances, each with its own credentials and company defaults. **Multiple profiles in one process are a requested future feature.**

This keeps credential and metadata contexts isolated and avoids model-controlled global account switching. Future multi-profile support should use explicit profile selection with per-profile HTTP clients and caches.

Keep credentials in local secret configuration or secret files, not tool arguments or committed client examples. FlexiBee is the authority for account/company permissions. A company's selector is not an account selector.

## Verification details still needed

- openSUSE Leap (version) or Tumbleweed, and Fedora release.
- Actual client versions and a test FlexiBee instance/version/company/API account.

The Go/local-stdio/account/platform direction is approved. Initial business scope remains the read-only foundation and two skills described in the roadmap. Platform names are support targets, not completed runtime certification.
