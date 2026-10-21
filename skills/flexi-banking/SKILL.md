---
name: flexi-banking
description: Import bank statements, match payments to invoices, manage bank accounts and cash movements in ABRA FlexiBee.
version: 0.1.0
tools:
  - flexi_get_connection_info
  - flexi_list_companies
  - flexi_describe_evidence
  - flexi_query_records
  - flexi_get_record
  - flexi_get_related_records
  - flexi_list_actions
  - flexi_upsert_record
  - flexi_execute_action
  - flexi_sum_records
  - flexi_get_guide
---

# flexi-banking

A portable skill for managing banking and cash in ABRA FlexiBee.

## Key evidence types

| Evidence key | Description |
|---|---|
| `banka` | Bank movements (statements) |
| `bankovni-ucet` | Bank accounts |
| `pokladni-pohyb` | Cash movements |
| `pokladna` | Cash registers |
| `prikaz-k-uhrade` | Payment orders |

## 1. Query bank movements

```
flexi_query_records(company, evidence="banka",
  filter="datVyst>='2024-01-01' AND datVyst<='2024-12-31'",
  fields=["id","datVyst","popis","castka","mena","varSym","protiucet"])
```

Key fields: `castka` (amount), `varSym` (variable symbol), `protiucet` (counterpart account).

## 2. Find unmatched bank transactions

```
filter="sparovano=false AND typPohybuK='typPohybuKreditni'"
```

`typPohybuK` values: `typPohybuKreditni` (incoming), `typPohybuDebetni` (outgoing).

## 3. Match a payment to an invoice (manual)

```
flexi_upsert_record(company, evidence="banka",
  record={
    "id": "<bank-movement-id>",
    "sparovano": true,
    "sparovaniDokladu": [{"id": "<invoice-id>"}]
  })
```

## 4. Auto-match payments

```
flexi_execute_action(company, evidence="banka",
  identifier="<bank-movement-id>", action="sparovat")
```

Or bulk auto-match for all unmatched:
```
flexi_execute_action(company, evidence="bankovni-ucet",
  identifier="<account-id>", action="automatickyPairovatPlatby")
```

## 5. Create a payment order

```
flexi_upsert_record(company, evidence="prikaz-k-uhrade",
  record={
    "typDokl": {"kod": "PRIKAZUHRADA"},
    "protiucet": "123456789/0800",
    "castka": 10000,
    "varSym": "2024001",
    "popis": "Payment for invoice F2024001"
  })
```

## 6. Cash movements

Create a cash receipt:
```
flexi_upsert_record(company, evidence="pokladni-pohyb",
  record={
    "typDokl": {"kod": "PRIJEM"},
    "castka": 500,
    "popis": "Cash sale"
  })
```

## 7. Grounding

Always state:
1. **Company** and **bank account** or **cash register** used.
2. **Date range** for any movement queries.
3. Whether matches were manual or automatic.
