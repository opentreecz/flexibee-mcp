---
name: flexi-explore
description: Explore ABRA FlexiBee accounting data — discover companies, browse evidence schemas, and run bounded read queries with grounded, traceable answers.
version: 0.1.0
tools:
  - flexi_get_connection_info
  - flexi_list_companies
  - flexi_list_evidences
  - flexi_describe_evidence
  - flexi_query_records
  - flexi_get_record
  - flexi_get_related_records
  - flexi_get_guide
---

# flexi-explore

A portable skill for exploring ABRA FlexiBee accounting data. Provides grounded, traceable answers by following a disciplined tool-calling sequence.

## 1. Connection

Always start by calling `flexi_get_connection_info` to:

- Verify the server is reachable.
- Obtain the resolved company context (if a default is configured).

Never guess or assume a company identifier. Use only values returned by tools.

## 2. Company Selection

Use `flexi_list_companies` to enumerate all available companies.

- The `company` parameter in all subsequent calls takes the **`dbNazev`** value (the database identifier), not the human-readable display name.
- If the user does not specify a company and no default is configured, ask which company to use before proceeding.

## 3. Discovering Evidences

Use `flexi_list_evidences` to find the correct evidence for the user's question.

- Use the `search` parameter to narrow results (e.g. `search=faktura` to find invoice evidences).
- The **evidence path** returned (e.g. `faktura-vydana`) is the value to pass as the `evidence` parameter to all other tools.
- Never invent or guess evidence paths.

## 4. Understanding Fields

Before querying, call `flexi_describe_evidence` to learn what is available:

| `section` value | What it returns |
|---|---|
| `fields` | Available field names, types, and descriptions |
| `relations` | Related evidences and relation paths |
| `reports` | Printable report names |

Always check `fields` before writing a query so you use valid field names.

## 5. Querying Records

Use `flexi_query_records` for list queries:

- Always specify explicit `fields` — do not fetch all fields unless necessary.
- Pass a FlexiBee filter expression to `filter` (see section 10 for syntax).
- Respect the `complete` flag in the result:
  - `complete=true` — all matching records were returned.
  - `complete=false` — the result is a **page**, not the full dataset. Tell the user that results are paginated and more records may exist.

## 6. Reading One Record

Use `flexi_get_record` to fetch a single record by:

- Its internal numeric `id`, or
- A code identifier in the form `code:VALUE` (e.g. `code:FAV-2026-0042`).

## 7. Following Relations

Use `flexi_get_related_records` with the relation path obtained from `flexi_describe_evidence` with `section=relations`.

Common example — invoice line items:

```
evidence: faktura-vydana
relation: polozkyDokladu
```

## 8. Grounding Answers

Every answer must be traceable to actual tool results:

- Never invent evidence names, field names, or record counts.
- Always quote the `company` (`dbNazev`) from the response.
- When data is paginated (`complete=false`), state this explicitly and do not present partial counts as totals.
- Cite the filter used and the number of records returned.

## 9. Ambiguous References

If any of the following are unclear, ask for clarification before calling a query tool:

- Which company to query.
- Which accounting period is intended.
- Which document or record the user is referring to.

## 10. Filter Syntax Examples

FlexiBee uses a SQL-like filter expression language:

```
# Date comparison
datVyst > '2026-01-01'

# Exact match
stavUhrady = 'neuhrazena'

# Prefix match
kod like 'FAV%'

# Combined conditions
(datVyst > '2026-01-01') and (stavUhrady = 'neuhrazena')

# Negation
not (stavUhrady = 'uhrazena')

# Null check
firma is null
```

Always wrap individual conditions in parentheses when combining them with `and` / `or`.
