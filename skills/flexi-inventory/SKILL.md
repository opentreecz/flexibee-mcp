---
name: flexi-inventory
description: Manage stock cards, inventory levels, stock movements and warehouses in ABRA FlexiBee.
version: 0.1.0
tools:
  - flexi_get_connection_info
  - flexi_list_companies
  - flexi_describe_evidence
  - flexi_query_records
  - flexi_get_record
  - flexi_get_related_records
  - flexi_sum_records
  - flexi_upsert_record
  - flexi_execute_action
  - flexi_get_guide
---

# flexi-inventory

A portable skill for managing inventory in ABRA FlexiBee.

## Key evidence types

| Evidence key | Description |
|---|---|
| `skladova-karta` | Stock cards (product definitions) |
| `skladovy-pohyb` | Stock movements (receipts, issues) |
| `sklad` | Warehouses |
| `cenik` | Price list / product catalog |

## 1. List stock cards

```
flexi_query_records(company, evidence="skladova-karta",
  fields=["id","kod","nazev","skladMnozstvi","jednotka","prodejCena"],
  sort="nazev@A")
```

Key fields:
- `skladMnozstvi` — current stock quantity
- `minSkladMnozstvi` — minimum stock threshold
- `prodejCena` — selling price (excl. VAT)
- `nakupCena` — purchase price

## 2. Find low-stock items

```
filter="skladMnozstvi<=minSkladMnozstvi"
```

Or below a fixed threshold:
```
filter="skladMnozstvi<10"
```

## 3. Stock total value

```
flexi_sum_records(company, evidence="skladova-karta")
```

Returns: `sumNakupCena` (total purchase value), `sumProdejCena` (total selling value).

## 4. Create a stock receipt (příjem)

```
flexi_upsert_record(company, evidence="skladovy-pohyb",
  record={
    "typDokl": {"kod": "PRIJEM"},
    "sklad": {"kod": "CENTRAL"},
    "polozkyPohybu": [
      {
        "skladovaKarta": {"kod": "ITEM001"},
        "mnozstvi": 100,
        "cenaMj": 50
      }
    ]
  })
```

## 5. Create a stock issue (výdej)

```
flexi_upsert_record(company, evidence="skladovy-pohyb",
  record={
    "typDokl": {"kod": "VYDEJ"},
    "sklad": {"kod": "CENTRAL"},
    "polozkyPohybu": [
      {
        "skladovaKarta": {"kod": "ITEM001"},
        "mnozstvi": 10
      }
    ]
  })
```

## 6. Stock movement history for an item

```
flexi_query_records(company, evidence="skladovy-pohyb",
  filter="polozkyPohybu.skladovaKarta.kod='ITEM001'",
  sort="datVyst@D")
```

## 7. Grounding

Always state:
1. **Company** and **warehouse** (sklad) used.
2. **Stock card code** (kod) for inventory items.
3. Whether the query represents current stock or a historical snapshot.
