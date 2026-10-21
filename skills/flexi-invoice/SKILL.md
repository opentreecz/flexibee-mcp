---
name: flexi-invoice
description: Create, update, preview and manage outbound invoices (faktura-vydana) in ABRA FlexiBee, including line items, dry-run validation, report download and workflow actions.
version: 0.2.0
tools:
  - flexi_get_connection_info
  - flexi_list_companies
  - flexi_describe_evidence
  - flexi_query_records
  - flexi_get_record
  - flexi_get_related_records
  - flexi_sum_records
  - flexi_list_actions
  - flexi_upsert_record
  - flexi_delete_record
  - flexi_execute_action
  - flexi_get_report
  - flexi_list_attachments
  - flexi_get_attachment
  - flexi_get_guide
---

# flexi-invoice

A portable skill for creating, updating, and managing outbound invoices in ABRA FlexiBee.

## 1. Start

1. Call `flexi_get_connection_info` — confirm server is reachable.
2. Confirm the target company (`dbNazev`) before any mutation.
3. Call `flexi_get_guide` with topic="write-operations" for field conventions.

Never proceed with a write against an unconfirmed company.

## 2. Discover the invoice schema

```
flexi_describe_evidence(company, evidence="faktura-vydana", section="fields")
```

Key required fields for a new invoice:

| Field | Required | Description |
|-------|----------|-------------|
| `typDokl` | Yes | Document type code (e.g. `code:FAKTURA`) |
| `firma` | Yes | Contact (by id or `code:` selector) |
| `datVyst` | Yes | Issue date (YYYY-MM-DD) |
| `datSplat` | Yes | Due date (YYYY-MM-DD) |
| `polozkyFaktury` | Yes | Array of line items |

Line item fields:

| Field | Required | Description |
|-------|----------|-------------|
| `nazev` | Yes | Item description |
| `mnozMj` | Yes | Quantity |
| `cenaMj` | Yes | Unit price (excl. VAT) |
| `sazDph` | Yes | VAT rate code (e.g. `typSazbyDph:typSazbyDphZakladni`) |

## 3. Look up a contact

```
flexi_query_records(company, evidence="adresar",
  filter="nazev LIKE '%<name>%'", fields=["id","kod","nazev","ic"])
```

Use the returned `id` as `{"firma": {"id": "<id>"}}` in the invoice.

## 4. Dry-run first (always)

```
flexi_upsert_record(company, evidence="faktura-vydana",
  record={...}, dry_run=true)
```

The server validates required fields, calculates VAT and totals, and returns
the computed invoice without persisting it. Inspect the result before writing.

## 5. Create the invoice

```
flexi_upsert_record(company, evidence="faktura-vydana",
  record={
    "typDokl": {"kod": "FAKTURA"},
    "firma": {"id": "<contact-id>"},
    "datVyst": "2024-12-01",
    "datSplat": "2024-12-31",
    "polozkyFaktury": [
      {
        "nazev": "Consulting services",
        "mnozMj": 10,
        "cenaMj": 1000,
        "sazDph": {"kod": "typSazbyDphZakladni"}
      }
    ]
  })
```

## 6. Update an existing invoice

Include `"id": "<invoice-id>"` (or `"kod": "<code>"`) in the record map.
Only supply fields you want to change.

## 7. Execute workflow actions

```
flexi_list_actions(company, evidence="faktura-vydana")
flexi_execute_action(company, evidence="faktura-vydana",
  identifier="<id>", action="odeslat")
```

Common actions: `lock` (lock document), `unlock`, `storno` (cancel),
`odeslat` (mark as sent), `zaplatit` (mark as paid).

## 8. Download the invoice as PDF

```
flexi_get_report(company, evidence="faktura-vydana",
  report_path="faktura", format="pdf")
```

The `data_base64` field contains the standard-base64-encoded PDF bytes.

## 9. Attachments

List attachments: `flexi_list_attachments(company, evidence="faktura-vydana", identifier="<id>")`
Download one: `flexi_get_attachment(company, attachment_id="<id>")`

## 10. Grounding

Every mutation answer must state:
1. **Company** — the `dbNazev` used.
2. **Dry run** — whether it was a dry run or a real write.
3. **Result** — the server-returned record id/code, or the error detail.
