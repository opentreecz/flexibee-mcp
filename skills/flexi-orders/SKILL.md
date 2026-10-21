---
name: flexi-orders
description: Manage purchase and sales orders in ABRA FlexiBee, including order creation, fulfilment tracking and conversion to invoices.
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

# flexi-orders

A portable skill for managing orders in ABRA FlexiBee.

## Key evidence types

| Evidence key | Description |
|---|---|
| `objednavka-vydana` | Sales orders (issued to customers) |
| `objednavka-prijata` | Purchase orders (received from suppliers) |
| `nabidka-vydana` | Sales quotes / proposals |
| `poptavka-vydana` | Sales enquiries |

## 1. Query open sales orders

```
flexi_query_records(company, evidence="objednavka-vydana",
  filter="stavK='stavOtevren'",
  fields=["id","kod","firma","datVyst","datDodani","sumCelkem","stavK"],
  sort="datDodani@A")
```

`stavK` values: `stavOtevren` (open), `stavVyrizen` (fulfilled), `stavStorno` (cancelled).

## 2. Create a sales order

```
flexi_upsert_record(company, evidence="objednavka-vydana",
  record={
    "typDokl": {"kod": "OBJEDVYD"},
    "firma": {"id": "<contact-id>"},
    "datVyst": "2024-12-01",
    "datDodani": "2024-12-15",
    "polozkyObjednavky": [
      {
        "nazev": "Product A",
        "mnozMj": 5,
        "cenaMj": 2000,
        "sazDph": {"kod": "typSazbyDphZakladni"}
      }
    ]
  })
```

## 3. Convert order to invoice

```
flexi_execute_action(company, evidence="objednavka-vydana",
  identifier="<order-id>", action="fakturovat")
```

This creates a linked outbound invoice. Check the returned record for the new invoice ID.

## 4. Check fulfilment status

```
flexi_get_related_records(company, evidence="objednavka-vydana",
  identifier="<order-id>", relation="faktury")
```

Returns linked invoices. Compare `sumCelkem` on the order vs. invoice to assess fulfilment.

## 5. Purchase orders

Create a purchase order:
```
flexi_upsert_record(company, evidence="objednavka-prijata",
  record={
    "typDokl": {"kod": "OBJEDPRI"},
    "firma": {"id": "<supplier-id>"},
    "datVyst": "2024-12-01",
    "polozkyObjednavky": [...]
  })
```

Approve received goods:
```
flexi_execute_action(company, evidence="objednavka-prijata",
  identifier="<order-id>", action="potvrdit")
```

## 6. Overdue orders

```
filter="stavK='stavOtevren' AND datDodani<today"
```

## 7. Grounding

Always state:
1. **Company** and **order type** (sales or purchase).
2. **Order status** at time of query.
3. **Related documents** (invoices, delivery notes) when relevant.
