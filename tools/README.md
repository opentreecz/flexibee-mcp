# tools/

Standalone Python utilities for working with FlexiBee outside the Go MCP server.

## flexibee_client.py

A complete Python client for FlexiBee supporting all three auth modes.
Useful for scripting, quick smoke tests, and as a protocol reference.

**Requirements**: `pip install requests`

### Auth modes

| Mode | How it works | Requires |
|------|-------------|----------|
| `session` | v2 HTML form login (`/login-logout/login`) | Nothing extra |
| `basic` | HTTP Basic Auth with `?auth=http` | `rest-api-rw` licence |
| `auto` | Tries session first, falls back to basic | Nothing extra |

### Python API

```python
from flexibee_client import FlexiBeeClient

# Session mode — no API licence required (works on basic2016)
with FlexiBeeClient("https://server:5434", "mcpbot", "password",
                    company="my-company", auth_mode="session") as client:

    # Read
    invoices = client.query("faktura-vydana",
                             filter="sumCelkem > 1000",
                             fields=["id", "kod", "sumCelkem"],
                             limit=10)

    # Write (returns server result)
    result = client.create("adresar", {
        "kod": "NEW001",
        "nazev": "New Contact",
        "email": "new@example.cz",
    })

    # Report download (returns bytes)
    pdf = client.get_report("faktura-vydana", invoice_id, format="pdf")
```

### CLI

```bash
python3 tools/flexibee_client.py \
  --url https://server:5434 \
  --user mcpbot --password secret \
  --company my-company \
  --auth-mode session \
  query --evidence faktura-vydana --fields id,kod,sumCelkem --limit 5
```

Available actions: `info`, `evidences`, `query`, `get`, `create`, `update`,
`delete`, `status`, `license`, `users`, `sum`, `export`, `report`

---

## test_protocols.py

Live integration test suite that exercises all three auth modes and all
major FlexiBee features against a real server.

**Warning**: this test creates and deletes real records. Use a disposable
test company only.

```bash
# Edit URL / USER / PASS / COMPANY at the top of the file, then:
python3 tools/test_protocols.py
```

The suite covers: auth, discovery, read, filters, pagination, full CRUD
lifecycle, dry-run, aggregation, exports (CSV, XML, JSON, ISDOC), PDF reports,
sub-evidences, and server info.
