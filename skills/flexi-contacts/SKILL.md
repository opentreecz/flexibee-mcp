---
name: flexi-contacts
description: Search, create and update contacts (adresar) in ABRA FlexiBee, including address lookup, relationship types, and deduplication by ICO/DIC.
version: 0.1.0
tools:
  - flexi_get_connection_info
  - flexi_list_companies
  - flexi_describe_evidence
  - flexi_query_records
  - flexi_get_record
  - flexi_get_related_records
  - flexi_upsert_record
  - flexi_get_guide
---

# flexi-contacts

A portable skill for managing contacts (address book) in ABRA FlexiBee.

## 1. Start

1. Call `flexi_get_connection_info` — confirm server is reachable.
2. Confirm the company before any mutation.

## 2. Search contacts

By name (fuzzy):
```
flexi_query_records(company, evidence="adresar",
  filter="nazev LIKE '%<name>%'",
  fields=["id","kod","nazev","ic","dic","email","tel"])
```

By registration number (ICO) — exact match:
```
flexi_query_records(company, evidence="adresar",
  filter="ic='<ico>'")
```

By VAT number (DIC):
```
flexi_query_records(company, evidence="adresar",
  filter="dic='<dic>'")
```

By relationship type:
```
filter="typVztahu.kod='typVztahuOdberatel'"   # customers
filter="typVztahu.kod='typVztahuDodavatel'"   # suppliers
```

## 3. Create a contact

Minimum required fields:

| Field | Description |
|-------|-------------|
| `nazev` | Display name |
| `ic` | Company registration number (if applicable) |

```
flexi_upsert_record(company, evidence="adresar",
  record={
    "nazev": "ACME s.r.o.",
    "ic": "12345678",
    "dic": "CZ12345678",
    "ulice": "Nová 1",
    "mesto": "Praha",
    "psc": "110 00",
    "stat": {"kod": "CZ"},
    "email": "info@acme.cz",
    "tel": "+420 123 456 789",
    "typVztahu": [{"kod": "typVztahuOdberatel"}]
  })
```

## 4. Update a contact

Include the contact `id` to update an existing record:
```
flexi_upsert_record(company, evidence="adresar",
  record={"id": "42", "email": "new@acme.cz", "tel": "+420 999 888 777"})
```

## 5. Deduplication

Before creating a contact, always search by ICO first:
1. `flexi_query_records` with `filter="ic='<ico>'"`.
2. If a match is returned, update the existing record instead of creating.
3. If no match, create new.

## 6. Relationship types

| Code | Meaning |
|------|---------|
| `typVztahuOdberatel` | Customer |
| `typVztahuDodavatel` | Supplier |
| `typVztahuPartner` | Partner |

A contact can have multiple relationship types (array field).

## 7. Grounding

Every contact answer must state:
1. **Company** — the `dbNazev` used.
2. **Action** — created, updated, or found existing.
3. **Contact id/code** — the server-returned identifier.
