---
name: flexi-receivables
description: Identify overdue and unpaid outgoing invoices in ABRA FlexiBee, accounting for partial payments, currency differences, and query completeness.
version: 0.1.0
tools:
  - flexi_get_connection_info
  - flexi_list_companies
  - flexi_describe_evidence
  - flexi_query_records
  - flexi_get_record
  - flexi_sum_records
  - flexi_get_guide
---

# flexi-receivables

A portable skill for analyzing accounts receivable in ABRA FlexiBee. Identifies overdue and unpaid outgoing invoices, accounting for partial payments, currency differences, and query completeness.

## 1. Start

1. Call `flexi_get_connection_info` to verify the server is reachable.
2. Confirm the target company (`dbNazev`) before issuing any query — either from the resolved default or by asking the user.

Never proceed with a query against an unconfirmed company.

## 2. Unpaid Invoices

Query the `faktura-vydana` evidence with filter:

```
stavUhrady = 'neuhrazena'
```

Request these fields explicitly:

```
id, kod, datVyst, datSplat, firma, sumCelkem, sumCelkemMen, mena, stavUhrady
```

This returns invoices with **zero payment** applied.

## 3. Overdue Invoices

To restrict to invoices past their due date, extend the filter:

```
(stavUhrady = 'neuhrazena') and (datSplat < now())
```

`now()` resolves to the current date on the server. No client-side date substitution is needed.

## 4. Partial Payments

Invoices with partial payment have `stavUhrady = 'castecneUhrazena'`. Query them separately:

```
stavUhrady = 'castecneUhrazena'
```

For partially paid invoices, use the **`zbyvaUhradit`** field for the remaining balance — do not use `sumCelkem`, which reflects the original invoice total.

To get all open receivables (both fully unpaid and partially paid) in one query:

```
(stavUhrady = 'neuhrazena') or (stavUhrady = 'castecneUhrazena')
```

## 5. Period Filter Warning

`faktura-vydana` is **implicitly filtered to the current accounting period** by default via the `platiOd` / `platiDo` parameters. This means invoices from previous periods may be silently excluded.

- To retrieve all-time overdue invoices, pass the query hint `filtrovat-platnost=false`.
- Always inform the user whether period filtering is active and what period is in scope.
- If the user asks "show me all overdue invoices" without specifying a period, clarify whether they want the current period only or all periods, then apply the appropriate setting.

## 6. Currency

Invoices carry amounts in two currencies:

| Field | Meaning |
|---|---|
| `sumCelkem` | Total in **home currency** |
| `sumCelkemMen` | Total in the invoice's **foreign currency** |
| `mena` | Currency code (e.g. `EUR`, `USD`) |

When the result set contains invoices in multiple currencies:

- **Do not sum across currencies** into a single figure.
- Present totals **grouped by currency code**.
- Note the home-currency total separately if useful.

## 7. Partial Results

If the tool returns `complete=false`:

- The overdue total computed from the page is a **lower bound**, not the true total.
- Tell the user: how many records were returned, and that more may exist beyond the page.
- Use `flexi_sum_records` on `faktura-vydana` with the **same filter** to obtain server-computed aggregate totals (sum, count) without retrieving all records. This is the preferred way to get an authoritative total when the result is paginated.

## 8. Grounding

Every receivables answer must explicitly state:

1. **Company** — the `dbNazev` used.
2. **Filter** — the exact filter expression passed to the query.
3. **Record count** — how many records were returned.
4. **Completeness** — whether `complete=true` or `complete=false`.
5. **Caveat** (if `complete=false`) — that the figures shown are partial and the authoritative total should be obtained via `flexi_sum_records`.
6. **Period scope** — whether period filtering was active and what period was in scope.

Never present a partial sum as a definitive total.
