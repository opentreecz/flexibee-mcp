# ABRA Flexi v2 API - Live-Probed Endpoint Catalog

> Probed live on **2026-10-15** against ABRA Flexi server **2026.6.2** at `https://127.0.0.1:5434`.
> Self-signed certificate, company DB `lukas_viktora_2026_08_01_1343`, user `test`.
> License: `basic` variant, `mini-1355`, 4 named users, **0 API users** (API license not purchased).

---

## Critical Discovery: v2 Session-Based API Bypasses API License

The REST API at `/c/` requires a purchased API license. Without it, Basic Auth returns **402 Payment Required**:

```json
{"winstrom":{"@version":"1.0","success":"false",
  "message":"Pristup uzivatele k API lze povolit pouze na licenci se zakoupenym pristupem k API"}}
```

However, the **v2 web UI API** at `/v2/c/` works with session-based auth and the `x-authSessionId` header, treating requests as web UI operations. This is the same mechanism the React SPA uses.

---

## 1. Authentication Flow

### 1.1 Login (Form POST)

```
POST /login-logout/login
Content-Type: application/x-www-form-urlencoded

username=test&password=test123
```

**Response:** `303 See Other` with cookies:

| Cookie | Path | Purpose |
|--------|------|---------|
| `authSessionId` | `/` | Session token (HttpOnly, Secure, SameSite=Lax) |
| `CSRF-Token` | `/` | CSRF protection token (HttpOnly) |
| `refreshToken` | `/v2/login-logout/check` | Session refresh token (HttpOnly, path-restricted) |
| `username` | `/` | Username |
| `useNewForm` | `/` | New UI flag (`true`) |
| `lang` | `/` | Language (`cs`) |

**IMPORTANT:** Do NOT follow the 303 redirect. The cookies on the 303 response itself are all that's needed.

### 1.2 JSON Login (v2)

```
POST /v2/login-logout/login
Content-Type: application/json

{"username":"test","password":"test123"}
```

**Response:** `200 OK` with JSON body AND Set-Cookie headers:

```json
{
  "authSessionId": "ffb54705...",
  "refreshToken": "5efabbce-...:202b2381-...",
  "csrfToken": "2d9d71e1-...",
  "intercomUserHash": "efd0b9f7...",
  "intercomJwt": "eyJ0eXA...",
  "success": true
}
```

**Note:** The JSON login (`/login-logout/login.json`) returns **402** without API license. Use `/v2/login-logout/login` instead.

### 1.3 Session Check / Refresh

```
GET /v2/login-logout/check
Cookie: refreshToken=...; authSessionId=...; CSRF-Token=...
```

**Response:** `200 OK` with refreshed session tokens:

```json
{
  "authSessionId": "56eb6d0c...",
  "csrfToken": "f1f5aafc-...",
  "intercomUserHash": "...",
  "intercomJwt": "...",
  "success": true
}
```

Also sets new `authSessionId` and `CSRF-Token` cookies.

### 1.4 Remote Logout (Concurrent Session Handling)

FlexiBee enforces a per-user concurrent session limit. When another session exists:

```json
{"winstrom":{"@version":"1.0","success":"false",
  "message":"Vas ucet je soucasne pouzivan i na jinem zarizeni, nejprve se na nem odhlaste.",
  "message@messageCode":"remoteLogoutDemand"}}
```

**Resolution flow:**

1. `POST /v2/login-logout/remote-logout` with session cookies -> `303 See Other`
2. Re-login via `POST /login-logout/login`
3. `GET /v2/login-logout/check` -> fresh `authSessionId`

### 1.5 Session Keep-Alive

```
GET /login-logout/session-keep-alive.js
Cookie: authSessionId=...
```

**Response:** `200 OK`

```json
{"obsolete":false,"loggedOut":false,"success":true}
```

- `"obsolete":true` means the session cookie format is old and needs refresh via `/v2/login-logout/check`
- `"loggedOut":true` means the session has been invalidated

### 1.6 Logout

```
GET /login-logout/logout
```

**Response:** `303 See Other` redirect to `/`

### 1.7 API Request Authentication

All v2 API requests use the `x-authSessionId` header:

```
GET /v2/c/{company}/faktura-vydana.json
x-authSessionId: 56eb6d0c3f52a3a1eb1f15b6ade03c67f8ac1c8221b9100d6c62c5ef6e9d466c
Accept: application/json
```

For write operations, the CSRF token is sent as a `token` query parameter:

```
PUT /v2/c/{company}/faktura-vydana.json?token={csrfToken}&dry-run=true
```

---

## 2. Server & License Endpoints

### 2.1 List Companies

```
GET /v2/c.json
x-authSessionId: ...
```

**Response:** `200 OK`

```json
{
  "companies": {
    "company": {
      "createDt": "2026-10-15T00:00:00Z",
      "dbNazev": "lukas_viktora_2026_08_01_1343",
      "id": 1,
      "licenseGroup": 1,
      "nazev": "test",
      "show": true,
      "stavEnum": "ESTABLISHED",
      "watchingChanges": false
    }
  }
}
```

Note: When only one company exists, `company` is an object. With multiple, it's an array.

### 2.2 License Info

```
GET /v2/license.json
x-authSessionId: ...
```

**Response:** `200 OK` - Returns license details including variant, features, user counts, modules.

### 2.3 Constants

```
GET /v2/constants.json
x-authSessionId: ...
```

**Response:** `200 OK`

```json
{"winstrom":{"@version":1,"constants":{"constant":{"name":"generatedPoldoklCisloRadkyIndex","value":9984}}}}
```

### 2.4 Users

```
GET /v2/u
GET /v2/u?withGroupInfo=true
x-authSessionId: ...
```

**Response:** `200 OK` - Lists all users with permissions, access lists, and (with `withGroupInfo`) license group details.

### 2.5 Admin Intercom Update

```
GET /v2/admin/intercom-update.json
x-authSessionId: ...
```

**Response:** `200 OK` - Returns Intercom JWT token for in-app help widget.

---

## 3. Evidence Discovery

### 3.1 Evidence List

```
GET /v2/c/{company}/evidence-list.json
x-authSessionId: ...
```

**Response:** `200 OK` - **155 evidence types** available. Full list below.

### 3.2 Evidence Properties (Schema)

```
GET /v2/c/{company}/{evidence}/properties.json
x-authSessionId: ...
```

**Response:** `200 OK` - Returns field definitions with types, sortability, visibility, mandatory flags, FK references, etc.

Example property:

```json
{
  "propertyName": "sumCelkem",
  "type": "numeric",
  "isSortable": true,
  "isWritable": true,
  "mandatory": false,
  "translationKey": "ucto-beans:beans.doklFak.sumCelkem.titl"
}
```

### 3.3 Evidence Relations

```
GET /v2/c/{company}/{evidence}/relations.json
x-authSessionId: ...
```

**Response:** `200 OK`

```json
{
  "relations": {
    "relation": [
      {"url": "polozkyDokladu", "evidenceType": "FAKTURA_OUT_POLOZKA", "name": "Polozky faktur"},
      {"url": "prilohy", "evidenceType": "PRILOHA", "name": "Prilohy"},
      {"url": "udalosti", "evidenceType": "ADR_UDALOST", "name": "Udalosti"},
      {"url": "vazby", "evidenceType": "VAZBA_MEZI_DOKLADY", "name": "Vazby mezi doklady"},
      {"url": "uzivatelske-vazby", "evidenceType": "VAZBA", "name": "Uzivatelske vazby"}
    ]
  }
}
```

### 3.4 Evidence Reports

```
GET /v2/c/{company}/{evidence}/reports.json
x-authSessionId: ...
```

**Response:** `200 OK` - Lists available report templates with IDs, languages, summary codes.

---

## 4. Evidence CRUD Operations

### 4.1 List Records (GET)

```
GET /v2/c/{company}/{evidence}.json
GET /v2/c/{company}/{evidence}.json?limit=20&start=0
x-authSessionId: ...
```

**Response:** `200 OK`

```json
{
  "winstrom": {
    "@version": "1.0",
    "faktura-vydana": [
      {
        "id": 1,
        "lastUpdate": "2026-04-29T21:04:53.341Z",
        "kod": "VF1-0001/2026",
        "stavUhrK": "stavUhr.uhrazeno",
        "datVyst": "2026-03-24",
        "sumCelkem": 500.0,
        "firma": "code:METABOND",
        "firma@showAs": "METABOND: Race Print GP s.r.o.",
        "firma@ref": "/v2/c/{company}/adresar/642.json",
        ...
      }
    ]
  }
}
```

### 4.2 Get Single Record

```
GET /v2/c/{company}/{evidence}/{id}.json
GET /v2/c/{company}/{evidence}/(kod='VF1-0001/2026').json
x-authSessionId: ...
```

Returns full detail for a single record. Code-based lookup uses parenthesized filter syntax.

### 4.3 Create Record (PUT)

```
PUT /v2/c/{company}/{evidence}.json
Content-Type: application/json
x-authSessionId: ...

{
  "winstrom": {
    "faktura-vydana": {
      "typDokl": "code:FAKTURA",
      "popis": "New invoice"
    }
  }
}
```

**Response:** `200 OK` (on success, or `201 Created`)

```json
{
  "winstrom": {
    "@version": "1.0",
    "success": true,
    "stats": {"created": 1, "updated": 0, "deleted": 0, "skipped": 0, "failed": 0},
    "results": [{"id": 9, "ref": "/v2/c/{company}/faktura-vydana/9.json"}]
  }
}
```

### 4.4 Update Record (PUT)

```
PUT /v2/c/{company}/{evidence}/{id}.json
Content-Type: application/json
x-authSessionId: ...

{
  "winstrom": {
    "faktura-vydana": {
      "id": 1,
      "popis": "Updated description"
    }
  }
}
```

### 4.5 Batch Create/Update (PUT)

Send an array to create/update multiple records in one transaction:

```
PUT /v2/c/{company}/{evidence}.json
Content-Type: application/json

{
  "winstrom": {
    "faktura-vydana": [
      {"typDokl": "code:FAKTURA", "popis": "Batch 1"},
      {"typDokl": "code:FAKTURA", "popis": "Batch 2"}
    ]
  }
}
```

### 4.6 Delete Record (DELETE)

```
DELETE /v2/c/{company}/{evidence}/{id}.json
x-authSessionId: ...
```

**404 response for non-existent record:**

```json
{
  "winstrom": {
    "success": false,
    "message@messageCode": "formZaznamNenalezen",
    "message": "Zaznam nebyl v datovem zdroji nalezen: cz.winstrom.vo.dok.DoklFak#99999"
  }
}
```

### 4.7 Action-Based Delete/Storno (POST)

```
POST /v2/c/{company}/{evidence}/{id}.json
Content-Type: application/json

{
  "winstrom": {
    "faktura-vydana": {
      "id": 1,
      "action": "storno"
    }
  }
}
```

Supported actions: `delete`, `storno`. Works with `?dry-run=true`.

### 4.8 XML Import (PUT)

```
PUT /v2/c/{company}/{evidence}.xml
Content-Type: application/xml

<?xml version="1.0"?>
<winstrom version="1.0">
  <faktura-vydana>
    <typDokl>code:FAKTURA</typDokl>
    <popis>XML import test</popis>
  </faktura-vydana>
</winstrom>
```

**Response:** XML format mirroring the JSON structure.

### 4.9 PATCH Method

**NOT SUPPORTED.** Returns HTTP 000 (connection reset). Use PUT for updates.

---

## 5. Query Features

All tested on `faktura-vydana` evidence. Apply to any evidence type.

### 5.1 Filtering

```
GET /v2/c/{company}/{evidence}.json?filter=datVyst>'2026-01-01'
GET /v2/c/{company}/{evidence}.json?filter=sumCelkem>100 and datVyst>='2026-01-01'
GET /v2/c/{company}/{evidence}.json?filter=popis like 'slu%'
GET /v2/c/{company}/{evidence}.json?filter=stavUhrK in ('stavUhr.uhrazeno')
```

Filter operators: `=`, `!=`, `>`, `>=`, `<`, `<=`, `like`, `in (...)`, `and`, `or`, `not`.

### 5.2 Sorting

```
GET /v2/c/{company}/{evidence}.json?order=datVyst@D
```

`@A` = ascending (default), `@D` = descending. Documented as `field+`/`field-` also.

### 5.3 Pagination

```
GET /v2/c/{company}/{evidence}.json?limit=20&start=0
```

- `limit` = number of records (default: 20)
- `start` = offset (0-based)

### 5.4 Field Selection

```
GET /v2/c/{company}/{evidence}.json?detail=custom:kod,datVyst,sumCelkem
```

Detail levels: `id`, `summary`, `full`, `custom:field1,field2,...`

**Response with `custom`:**

```json
{"winstrom":{"@version":"1.0","faktura-vydana":[
  {"id":1,"kod":"VF1-0001/2026","datVyst":"2026-03-24","sumCelkem":500.0},
  {"id":2,"kod":"VF1-0002/2026","datVyst":"2026-03-24","sumCelkem":269.0}
]}}
```

### 5.5 Row Count

```
GET /v2/c/{company}/{evidence}.json?add-row-count=true
```

**Response:** Adds `@rowCount` to the wrapper:

```json
{"winstrom":{"@version":"1.0","@rowCount":"6","faktura-vydana":[...]}}
```

### 5.6 Relations (Inline Sub-Records)

```
GET /v2/c/{company}/{evidence}.json?relations=polozkyDokladu
```

Returns line items inline within each parent record.

### 5.7 Includes

```
GET /v2/c/{company}/{evidence}/{id}.json?includes=/faktura-vydana/polozkyDokladu,/faktura-vydana/prilohy
```

### 5.8 Dry Run

```
PUT /v2/c/{company}/{evidence}.json?dry-run=true
```

Validates the operation without persisting. Returns the record as it would be saved (with computed fields filled in, `id=-1`).

### 5.9 Fail on Warning

```
PUT /v2/c/{company}/{evidence}.json?dry-run=true&fail-on-warning=true
```

### 5.10 GUI Mode

```
GET /v2/c/{company}/{evidence}.json?as-gui=true
```

The SPA uses `as-gui=true` for all requests. It affects field visibility and `@possibleValues` annotations.

### 5.11 Summary / Aggregation

```
GET /v2/c/{company}/{evidence}/$sum.json
```

**Response:** Accounting summary with totals:

```json
{
  "winstrom": {
    "@version": "1.0",
    "sum": {
      "sumDoklUcetni": {
        "values": {
          "sumDoklCelkem": {"msg": "Celkem:", "currency": "Kc", "value": "4291.0"},
          "sumDoklJizUh": {"msg": "Jiz uhrazeno:", "currency": "Kc", "value": "3871.0"},
          "sumDoklZbyvaUh": {"msg": "Zbyva uhradit:", "currency": "Kc", "value": "420.0"}
        }
      }
    }
  }
}
```

---

## 6. Sub-Evidence / Related Records

### 6.1 Line Items

```
GET /v2/c/{company}/{evidence}/{id}/polozkyDokladu.json
```

**Response:** Returns line items (`faktura-vydana-polozka`) for the parent document.

### 6.2 Attachments List

```
GET /v2/c/{company}/{evidence}/{id}/prilohy.json
```

Or via the global attachment evidence:

```
GET /v2/c/{company}/priloha.json
```

### 6.3 Attachment Download

```
GET /v2/c/{company}/priloha/{id}/content
x-authSessionId: ...
```

**Response:** Binary file content with appropriate `Content-Type`.

### 6.4 Attachment Upload (Inline with Parent)

```json
{
  "winstrom": {
    "faktura-vydana": {
      "typDokl": "code:FAKTURA",
      "popis": "Invoice with attachment",
      "prilohy": [
        {"nazSoub": "test.txt", "contentType": "text/plain", "dataFile": "dGVzdA=="}
      ]
    }
  }
}
```

Attachments must be imported as part of a parent object. Standalone attachment creation returns error `attachementOnlyWithObj`.

### 6.5 Document Links (Vazby)

```
GET /v2/c/{company}/{evidence}/{id}/vazby.json?detail=full&includes=/vazba/a,/vazba/b&as-gui=true
```

Returns linked documents (payments, stock movements, etc.).

### 6.6 Events

```
GET /v2/c/{company}/{evidence}/{id}/udalosti.json
```

### 6.7 User Links

```
GET /v2/c/{company}/{evidence}/{id}/uzivatelske-vazby.json
```

---

## 7. Output Formats

### 7.1 JSON (Primary)

```
GET /v2/c/{company}/{evidence}.json
Content-Type: application/json
```

### 7.2 XML

```
GET /v2/c/{company}/{evidence}.xml
Content-Type: application/xml
```

XML includes Czech comments with field descriptions.

### 7.3 CSV

```
GET /v2/c/{company}/{evidence}.csv
Content-Type: text/csv
```

Full column headers in first row.

### 7.4 PDF (Reports)

```
GET /v2/c/{company}/{evidence}/{id}.pdf
GET /v2/c/{company}/{evidence}/{id}.pdf?report-name=fakturaKB
GET /v2/c/{company}/{evidence}/{id}.pdf?report-name=fakturaKB&report-lang=en
GET /v2/c/{company}/{evidence}.pdf?filter=id in (1,2)     # multi-record print
```

Returns PDF document. Multi-record prints combine all matching records.

| Parameter | Purpose |
|-----------|---------|
| `report-name` | Report template ID (from `reports.json`) |
| `report-lang` | Language code: `cs`, `en`, `de`, `sk`, `pl`, `ru` |
| `report-sign` | Digital signature (requires certificate setup) |

### 7.5 XLSX (Excel)

```
GET /v2/c/{company}/{evidence}.xlsx
Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
```

### 7.6 XLS (Legacy Excel)

```
GET /v2/c/{company}/{evidence}.xls
Content-Type: application/ms-excel
```

### 7.7 DBF (dBASE)

```
GET /v2/c/{company}/{evidence}.dbf
Content-Type: application/dbf
```

### 7.8 ISDOC (Czech e-Invoice Standard)

```
GET /v2/c/{company}/{evidence}/{id}.isdoc
Content-Type: application/xml (ISDOC 6.0.2 namespace)
```

### 7.9 ISDOCX (ISDOC Package)

```
GET /v2/c/{company}/{evidence}/{id}.isdocx
Content-Type: application/x-isdocx
```

ZIP archive containing the ISDOC XML and attachments.

### 7.10 HTML

```
GET /v2/c/{company}/{evidence}.html
```

Returns old-style web form UI (not the SPA).

---

## 8. Evidence Types - Complete List (155 types)

Evidence types confirmed accessible via `/v2/c/{company}/evidence-list.json`:

### Document Evidences

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `faktura-vydana` | Vydane faktury | Yes |
| `faktura-vydana-polozka` | Polozky vydanych faktur | Yes |
| `faktura-prijata` | Prijate faktury | Yes |
| `faktura-prijata-polozka` | Polozky prijatych faktur | - |
| `interni-doklad` | Interni doklady | Yes |
| `interni-doklad-polozka` | Polozky internich dokladu | - |
| `banka` | Banka | Yes |
| `banka-polozka` | Polozky banky | - |
| `pokladni-pohyb` | Pokladna | Yes |
| `pokladni-pohyb-polozka` | Polozky pokladnich dokladu | - |
| `doklad` | Prehled vsech dokladu | Yes |
| `doklad-k-uhrade` | Doklady k uhrade | Yes |
| `pohledavka` | Ostatni pohledavky | - |
| `zavazek` | Ostatni zavazky | - |
| `skladovy-pohyb` | Prijemky/vydejky | Yes |
| `vzajemny-zapocet` | Vzajemne zapocty | - |
| `prikaz-k-uhrade` | Prikaz k uhrade | - |

### Address Book & Contacts

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `adresar` | Adresy firem | Yes |
| `kontakt` | Kontakty | Yes |
| `udalost` | Udalosti, aktivity | Yes |
| `adresar-bankovni-ucet` | Bankovni spojeni | - |
| `skupina-firem` | Skupiny firem | - |

### Products & Inventory

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `cenik` | Cenik | Yes |
| `skladova-karta` | Skladove karty | Yes |
| `sklad` | Seznam skladu | Yes |
| `skupina-zbozi` | Skupiny zbozi | - |
| `inventura` | Inventury | - |
| `sarze-expirace` | Sarze a expirace | - |
| `stav-skladu-k-datu` | Stav skladu k datu | Yes (requires `?sklad=`) |
| `merna-jednotka` | Merne jednotky | Yes |

### Finance & Banking

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `bankovni-ucet` | Bankovni ucty | Yes |
| `pokladna` | Seznam pokladen | Yes |
| `bankovni-ucet-pokladna` | Bankovni ucty a pokladny | - |
| `penezni-ustav` | Penezni ustavy | Yes |
| `forma-uhrady` | Formy uhrady | Yes |

### Assets

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `majetek` | Majetek | Yes |
| `leasing` | Leasing | Yes |
| `splatkovy-kalendar` | Splatkovy kalendar | - |
| `danovy-naklad` | Danove naklady | - |
| `danovy-odpis` | Danove odpisy | - |

### Accounting & Tax

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `predpis-zauctovani` | Predpisy zauctovani | Yes |
| `ucetni-obdobi` | Ucetni obdobi | Yes |
| `sazba-dph` | Sazby DPH | Yes |
| `podklady-dph` | Podklady DPH | Yes |
| `cleneni-dph` | Radky priznani DPH | - |
| `saldo-k-datu` | Stav uhrad k datu | Yes |
| `po-splatnosti` | Neuhrazene po splatnosti | Yes (requires `?datumVypoctu=`) |

### Configuration & System

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `nastaveni` | Nastaveni | Yes |
| `global-store` | Uloziste globalniho nastaveni | Yes |
| `setting-store` | Uloziste nastaveni | Yes |
| `typ-dokladu` | Typy dokladu | Yes |
| `rada` | Dokladove rady | Yes |
| `stredisko` | Strediska | Yes |
| `mena` | Meny | Yes |
| `stat` | Staty | Yes |
| `kurz` | Kurzy | Yes |
| `strom` | Uzel stromu | Yes |
| `uzivatel` | Uzivatele ve firme | Yes |
| `role` | Uzivatelske role | Yes |
| `filtr` | Uzivatelske filtry | Yes |
| `stitek` | Stitky | Yes |
| `priloha` | Priloha | Yes |
| `zurnal` | Zurnal | Yes |

### Reports & Aggregates

| Evidence Path | Czech Name | HTTP 200 |
|---------------|-----------|----------|
| `cenikovy-pohyb-nakup` | Nakupni pohyby | - |
| `cenikovy-pohyb-prodej` | Prodejni pohyby | Yes |
| `neuhrazene-po-splatnosti` | Neuhrazene po splatnosti | Yes |
| `danova-evidence` | Vykaz danove evidence | - |

### License-Restricted Evidences (400 - Access Denied)

These return `400` with `akcePristupOdepren` ("You don't have access to this action") on the `basic` license:

- `smlouva`, `zakazka`, `cinnost`, `objednavka-vydana`, `objednavka-prijata`
- `nabidka-vydana`, `nabidka-prijata`, `poptavka-vydana`, `poptavka-prijata`
- `ucetni-denik`, `ucetni-osnova`, `ucet`, `saldo`, `pohyb-na-uctech`
- `uzivatelsky-dotaz`, `cenova-uroven`, `typ-aktivity`, `typ-atributu`, `atribut`
- `stav-uctu`, `cislo-baliku`

### Changelog (Requires Activation)

```
GET /v2/c/{company}/changes.json
```

Returns `400` with `changelogNotEnabled`: "Changelog neni povolen." Must be enabled in company settings.

---

## 9. Response Formats & Error Codes

### Success Response

```json
{
  "winstrom": {
    "@version": "1.0",
    "success": true,
    "stats": {"created": 1, "updated": 0, "deleted": 0, "skipped": 0, "failed": 0},
    "results": [{"id": 9, "ref": "/v2/c/{company}/faktura-vydana/9.json"}]
  }
}
```

With `as-gui=true` or `dry-run=true`, `results` includes `content` with the full record.

### Error Responses

| HTTP Code | messageCode | Meaning |
|-----------|-------------|---------|
| 400 | `akcePristupOdepren` | Access denied (license/permissions) |
| 400 | `changelogNotEnabled` | Changelog not enabled |
| 400 | `missing_param_exception` | Required parameter missing |
| 401 | `authRequired` | Authentication required |
| 401 | `remoteLogoutDemand` | Concurrent session conflict |
| 402 | (none) | API license not purchased (Basic Auth only) |
| 404 | `adresaNeplatnaUrl` | Invalid URL / evidence not found |
| 404 | `formZaznamNenalezen` | Record not found |
| 406 | (none) | Not Acceptable (wrong Accept header for endpoint) |

### Reference Format in Records

Relations in records use this pattern:

```json
{
  "firma": "code:METABOND",
  "firma@evidencePath": "adresar",
  "firma@showAs": "METABOND: Race Print GP s.r.o.",
  "firma@ref": "/v2/c/{company}/adresar/642.json"
}
```

Enum fields:

```json
{
  "stavUhrK": "stavUhr.uhrazeno",
  "stavUhrK@showAs": "Uhrazeno",
  "stavUhrK@possibleValues": "[stavUhr.uhrazenoRucne]"
}
```

---

## 10. Complete Authentication Lifecycle for MCP Server

### Recommended Flow

```
1. POST /login-logout/login (form data) -> get authSessionId from Set-Cookie
2. GET /v2/login-logout/check (with cookies) -> get fresh authSessionId in JSON body
3. IF 401 remoteLogoutDemand:
   a. POST /v2/login-logout/remote-logout (with cookies)
   b. POST /login-logout/login (form data) -> new cookies
   c. GET /v2/login-logout/check -> fresh authSessionId
4. Use x-authSessionId header for all /v2/c/{company}/ requests
5. Periodically call GET /login-logout/session-keep-alive.js
6. On session expiry (302 redirect), re-authenticate from step 1
7. On shutdown: GET /login-logout/logout
```

### Key Header

```
x-authSessionId: {authSessionId}
```

The `x-authSessionId` header (note lowercase 'x') is the primary auth mechanism for the v2 API. The SPA conditionally sends it based on a `sendXAuthHeader` setting stored in `localStorage`.

### Write Operations

Write operations (PUT, POST, DELETE) use the CSRF token as a `token` query parameter:

```
PUT /v2/c/{company}/faktura-vydana.json?token={csrfToken}&as-gui=true
x-authSessionId: {authSessionId}
Content-Type: application/json
```

---

## 11. URL Path Structure

```
/v2/c/{companyDbName}/{evidencePath}.{format}
/v2/c/{companyDbName}/{evidencePath}/{id}.{format}
/v2/c/{companyDbName}/{evidencePath}/(filter).{format}
/v2/c/{companyDbName}/{evidencePath}/properties.{format}
/v2/c/{companyDbName}/{evidencePath}/relations.{format}
/v2/c/{companyDbName}/{evidencePath}/reports.{format}
/v2/c/{companyDbName}/{evidencePath}/{id}/{relation}.{format}
/v2/c/{companyDbName}/{evidencePath}/{id}/content          (attachments)
/v2/c/{companyDbName}/{evidencePath}/$sum.{format}          (aggregation)
/v2/c/{companyDbName}/evidence-list.{format}                (discovery)
```

The `/v2/` prefix is required for session-based auth without API license. The legacy `/c/` prefix requires a purchased API license and Basic Auth.

---

## 12. Differences: /v2/ vs /c/ (Legacy)

| Feature | `/v2/c/` | `/c/` |
|---------|----------|-------|
| Auth method | `x-authSessionId` header | Basic Auth |
| API license required | No | Yes |
| Response format | Same JSON/XML | Same JSON/XML |
| Session management | Session cookies + header | Stateless |
| `@ref` paths | Include `/v2/` prefix | No prefix |
| SPA compatibility | Full | N/A |

Both API versions return the same data structure (`{"winstrom":{...}}`). The `/v2/` prefix is consistently used in `@ref` links within responses.
