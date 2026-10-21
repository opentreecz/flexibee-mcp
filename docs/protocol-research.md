# FlexiBee Protocol Deep Dive — Three Communication Channels

Research date: **2026-10-15**. Scope: reverse engineering of FlexiBee desktop client JARs (v2026.4.12), decompilation of server JARs (v2026.6.2), live probing of local Docker server. All findings are from the on-premises version; cloud-hosted instances may differ.

## Executive Summary

FlexiBee (ABRA Flexi) exposes **three distinct communication channels**, all multiplexed on a single HTTPS port (default 5434):

| Channel | Protocol | Auth Method | API License Required | Best For |
|---------|----------|-------------|---------------------|----------|
| **REST API (v1)** | HTTPS + JSON/XML | Basic Auth + `?auth=http` | **Yes** (checks `api_access_enabled` + license feature) | External integrations with API license |
| **Web Session (v2)** | HTTPS + JSON/XML | Form login → `authSessionId` cookie | **No** | MCP without API license; remote browser access |
| **Desktop Client** | mini-RMI over HTTPS + JDBC proxy over TLS | Form login → session → RMI + JDBC tunnel | **No** (checks `pristup-aplikaci` license feature) | Desktop UI; business logic not exposed via REST |

**Key finding**: The **v2 API path** (`/v2/c/{company}/...`) is what the React web UI uses. It provides the same data endpoints as the REST API but authenticates via web session cookies, bypassing the `api_access_enabled` user flag entirely. This is the recommended channel for MCP when no API license is available.

---

## Channel 1: REST API (v1 path)

### Authentication
- **HTTP Basic Auth** with `?auth=http` query parameter to prevent HTML login redirect
- Over verified TLS (self-signed certs common on localhost)
- With 2FA enabled, every request needs a current OTP

### Access Control
1. Server checks license feature `rest-api-ro` / `rest-api-rw`
2. Server checks `csuzivatel.api_access_enabled` flag per user
3. If either check fails → `402 Payment Required` with message about API license

### URL Pattern
```
GET  /c/{company}/{evidence}.json?auth=http
GET  /c/{company}/{evidence}/{id}.json?auth=http
PUT  /c/{company}/{evidence}.json?auth=http           (create/update)
DELETE /c/{company}/{evidence}/{id}.json?auth=http    (delete)
```

### Response Format
- All values as strings: `"id":"1"`, `"show":"true"`, `"sumCelkem":"500.0"`
- No `@evidencePath` annotations on relation fields
- Refs use `/c/` prefix

### When to Use
- External systems with purchased API license
- Hosted FlexiBee cloud instances where API is enabled
- Automated integrations that need stateless auth (no session management)

---

## Channel 2: Web Session (v2 path) — RECOMMENDED FOR MCP

### Authentication Flow

```
1. POST /login-logout/login
   Body: username=test&password=test123  (form-urlencoded)
   Response: 303 See Other
   Set-Cookie: authSessionId=<64-char-hex>; Path=/; HttpOnly; Secure
   Set-Cookie: CSRF-Token=<uuid>; Path=/; HttpOnly; Secure
   Set-Cookie: refreshToken=<uuid>:<uuid>; Path=/login-logout/check

2. All subsequent requests:
   Cookie: authSessionId=<token>
   — OR —
   x-authSessionId: <token>  (header, case-insensitive)

3. Write operations: CSRF token via one of:
   X-CSRF-Token: <token>  (header)
   — OR —
   ?token=<token>  (query parameter)
   — NOTE: CSRF may not be enforced on all servers

4. Keepalive:
   GET /login-logout/session-keep-alive.js
   → {"obsolete":false,"loggedOut":false,"success":true}

5. Logout:
   GET /login-logout/logout  (with authSessionId cookie/header)
```

### URL Pattern
```
GET  /v2/c/{company}/{evidence}.json
GET  /v2/c/{company}/{evidence}/{id}.json
PUT  /v2/c/{company}/{evidence}.json          (create → 201)
PUT  /v2/c/{company}/{evidence}/{id}.json     (update → 201)
DELETE /v2/c/{company}/{evidence}/{id}.json   (delete → 200)
```

### Response Format (differs from v1)
- Native JSON types: `"id":1`, `"show":true`, `"sumCelkem":500.0`
- Relation fields annotated: `"firma@evidencePath":"adresar"`, `"firma@ref":"/v2/c/.../adresar/642.json"`, `"firma@showAs":"METABOND: Race Print GP s.r.o."`
- Refs use `/v2/c/` prefix
- Create returns **201 Created** (v1 returns 200)

### Session Management Constraints
- **Per-user concurrent session limit** enforced by named-user licensing
- New sessions for the same user may mark older sessions as **obsolete**
- Obsolete sessions get redirected to `/login-logout/remote-logout`
- Always `logout()` when done to free the session slot
- If the FlexiBee desktop client is running as the same user, it consumes a session slot

### Discovered Endpoints (live-verified on 2026.6.2)

#### Discovery & Metadata
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v2/c.json` | GET | List companies |
| `/v2/c/{co}/evidence-list.json` | GET | List 155 evidence types |
| `/v2/c/{co}/all-evidence-list.json` | GET | List ALL evidences (incl. hidden) |
| `/v2/c/{co}/metadata.json` | GET | Full metadata (~1MB, all fields/relations for all evidences) |
| `/v2/c/{co}/{ev}/properties.json` | GET | 69 field definitions for adresar |
| `/v2/c/{co}/{ev}/relations.json` | GET | Relation definitions |
| `/v2/c/{co}/{ev}/reports.json` | GET | Available report templates |
| `/v2/c/{co}/{ev}/actions.json` | GET | Available actions (copy, edit, delete, storno) |
| `/v2/c/{co}/{ev}/named-filters.json` | GET | Saved named filters |
| `/v2/c/{co}/permissions.json` | GET | Evidence permissions for current user |

#### CRUD Operations
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v2/c/{co}/{ev}.json` | GET | List records (with query params below) |
| `/v2/c/{co}/{ev}/{id}.json` | GET | Get single record |
| `/v2/c/{co}/{ev}.json` | PUT/POST | Create new record(s) |
| `/v2/c/{co}/{ev}/{id}.json` | PUT/POST | Update existing record |
| `/v2/c/{co}/{ev}/{id}.json` | DELETE | Delete record |
| `/v2/c/{co}/{ev}/query.json` | POST | Query with filter/params in body |

#### Query Parameters
| Parameter | Example | Purpose |
|-----------|---------|---------|
| `limit` | `?limit=20` | Page size (0 = no limit) |
| `start` | `?start=40` | Offset for pagination |
| `detail` | `?detail=custom:id,kod,nazev` | Field selection |
| `detail` | `?detail=full` | All fields |
| `filter` | `?filter=kod='VF1'` | Filter expression |
| `sort` | `?sort=datVyst` | Sort field(s) |
| `add-row-count` | `?add-row-count=true` | Include `@rowCount` in response |
| `relations` | `?relations=polozkyDokladu` | Include relation records |
| `dry-run` | `?dry-run=true` | Validate without saving |
| `fail-on-warning` | `?fail-on-warning=true` | Treat warnings as errors |
| `report-name` | `?report-name=fakturaVydana` | Select report template for PDF |

#### Sub-Evidence (Relations/Line Items)
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v2/c/{co}/{ev}/{id}/{relation}.json` | GET | Get related records (e.g., invoice line items) |
| `/v2/c/{co}/{ev}/{id}/prilohy.json` | GET | List attachments |
| `/v2/c/{co}/{ev}/{id}/prilohy/{attId}/content` | GET | Download attachment content |

#### Document Sums
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v2/c/{co}/{ev}/$sum.json` | GET | Aggregate totals for document-type evidences |

#### Export Formats
| Extension | Content-Type | Notes |
|-----------|-------------|-------|
| `.json` | application/json | Default |
| `.xml` | application/xml | Full XML with Czech comments |
| `.csv` | text/csv | All fields, quoted headers |
| `.pdf` | application/pdf | Report output (invoice, etc.) |
| `.isdoc` | application/xml | Czech ISDOC e-invoice format |
| `.xlsx` | application/xlsx | Excel export |
| `.xls` | application/xls | Legacy Excel |
| `.dbf` | application/dbf | dBASE format |
| `.isdocx` | application/isdocx | ISDOC with attachments |

#### Server/License/User Management (v2)
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/status.json` | GET | Server status (v1 only, no v2 equivalent) |
| `/v2/license.json` | GET | Full license info with modules and features |
| `/v2/u.json` | GET | List users with permissions |
| `/v2/u/{id}.json` | GET/PUT/DELETE | User CRUD |
| `/v2/c.json` | PUT/POST | Create company |
| `/v2/c/{co}/backup` | GET | Download company backup |
| `/login-logout/connection-file` | GET | Download `.flexibee-connection` file |

#### Activiti Workflows
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/c/{co}/activiti/process-definitions` | GET | List workflow definitions |
| `/c/{co}/{ev}/{id}/workflows.json` | GET | Workflows for record (requires license) |
| `/c/{co}/{ev}/{id}/workflows/{pid}/start` | POST | Start workflow on record |

---

## Channel 3: Native Desktop Client

### Protocol Stack
The desktop client uses **two sub-protocols** over the same HTTPS port:

#### 3a. mini-RMI over HTTPS
- Custom serialization protocol (NOT standard Java RMI)
- `POST https://host:port/mini-rmi/WinStromConnector/?methodName&user=...`
- Content-Type: `application/winstrom-rmi`
- Body: Java `ObjectOutputStream` serialized `MethodCall` object
- Response: Java `ObjectOutputStream` serialized `Response` object
- Client creates **cglib proxy** of `WinStromConnector` interface

#### 3b. JDBC Proxy over TLS
- Port 5434 multiplexes HTTP and PostgreSQL wire protocol
- After TLS handshake, client sends: `JDWSJDWS` magic + version + host + port + username + authSessionId + companySessionId + databaseName
- Then standard PostgreSQL startup message with user/database args
- Server authenticates via session registry, creates internal PostgreSQL connection
- Server proxies all subsequent PostgreSQL traffic bidirectionally
- Custom SSL factory: `cz.winstrom.client.MySSLClientSocketFactory` (trusts all certs)

### WinStromConnector Interface (57 methods)

**Database/Company Management (14 methods)**:
- `createDatabase(name, template, orgType, auth)` → create company
- `deleteDatabase(name, auth)` → delete company (stub — always returns false)
- `updateDatabase(name, info, auth)` → update company metadata
- `databaseExists(name, auth)` → check if company exists
- `getJdbcConnection(name, auth)` → get JDBC URL for company
- `getJdbcConnections(auth)` → list all JDBC URLs
- `backupDatabase(name, auth)` / `restoreDatabase(name, params, auth)`
- `getLastBackupDate(name, auth)`, `getLastChangeDate(name, auth)`
- `getDatabaseVersion(name, auth)` → schema version
- `upgradeDatabase(name, auth)` → upgrade schema
- `getTemplateList(auth)` → list company templates

**User Management (12 methods)**:
- `createUser(userInfo, auth)` / `deleteUser(userId, auth)`
- `updateUser(userInfo, auth)`
- `listUsers(auth)` → list all users
- `findUser(username, auth)` → get user by name
- `createUserWithAccess(userInfo, companies, auth)`
- `deleteUserFromCompany(userId, company, auth)`
- `getUserCompanyAccessList(userId, auth)` / `setUserCompanyAccessList(userId, list, auth)`
- `listUsersAndAccess(auth)` → users with company access
- `setUserCompanyAccess(userId, company, rw, auth)`
- `checkUserDBAccess(userId, company, auth)`

**Authentication & Password (9 methods)**:
- `login(user, pass, clientType, auth)` → returns AuthInformation
- `loginOTP(user, pass, otp, clientType, time, auth)` → login with 2FA
- `logout(auth)` → end session
- `verifyPrevPasswordForChange(pass, auth)` → stub (always returns true)
- `changePassword(user, newPass, auth)` / `changePasswordAndOtp(user, newPass, otpSecret, auth)`
- `changePasswordAdmin(user, newPass, auth)`
- `checkPasswordQuality(pass, auth)` → returns quality score
- `getAuthType(auth)` → returns auth configuration

**Session Management (6 methods)**:
- `acquireCompanySession(company, connectionType, auth)` → lock company for access
- `releaseCompanySession(auth)` → release company lock
- `keepAlive(auth)` → heartbeat (returns KeepAliveResponse)
- `setServerVariables(vars, auth)` → set session variables
- `getServerInfo()` → server version, build info
- `findLicense(groupId, auth)` → get license data

**Access Control (7 methods)**:
- `getConnectedUsers(auth)` / `getConnectedUsersRW(auth)` / `getConnectedUsersRO(auth)`
- `canConnectLicense(company, connectionType, auth)` → check license allows connection
- `grantUserCompanyAccess(userId, company, rw, auth)`
- `getServerVariables(auth)` → get server-side session variables
- `getKeepAliveInterval()` → heartbeat interval in seconds

**License Management (4 methods)**:
- `getLicenseBlob(groupId, auth)` / `setLicenseBlob(blob, groupId, auth)`
- `createLicenseForNewCompany(license, auth)` / `getLicenseForNewCompany(groupId, auth)`

**Utility (2 methods)**:
- `sendEmail(email, auth)` → send email via server SMTP
- `sendInvoices(company, date, auth)` → auto-send invoices for date

### Desktop Client Login Sequence
1. `WinStromClient.init(host, port)` → set server address
2. `Naming.lookup(connector, host, port, WinStromConnector.class)` → create RMI proxy
3. `connector.login(user, pass, ClientType.SWING, auth)` → authenticate, get `AuthInformation` with `authSessionId`
4. `ClientAuthenticator.setAuthSessionId(sessionId)` → store session
5. `connector.acquireCompanySession(company, RW, auth)` → get `CompanySession`
6. `MySSLClientSocketFactory.setCompanyId(companyDbName)` → prepare for JDBC
7. JDBC connection via `jdbc:postgresql://host:port/company?ssl=true&sslfactory=MySSLClientSocketFactory`
8. From this point: all data access is direct SQL via the JDBC proxy

---

## Comparison Matrix

| Capability | REST API (v1) | Web Session (v2) | Desktop Client |
|------------|:---:|:---:|:---:|
| Read records | Yes | **Yes** | Yes (SQL) |
| Create records | Yes | **Yes** | Yes (SQL) |
| Update records | Yes | **Yes** | Yes (SQL) |
| Delete records | Yes | **Yes** | Yes (SQL) |
| Filter/sort/page | Yes | **Yes** | Yes (SQL WHERE) |
| PDF reports | Yes | **Yes** | Yes (local rendering) |
| Attachments | Yes | **Yes** | Yes (SQL + HTTP) |
| Change tracking | Yes | **Yes** | Yes (SQL) |
| Webhooks | Yes | Yes | No |
| Workflows/Activiti | Yes | **Yes** | Yes (RMI) |
| User management | Yes | **Yes** | Yes (RMI) |
| Company management | Yes | **Yes** | Yes (RMI) |
| License management | Limited | **Limited** | Yes (RMI) |
| Backup/restore | Yes | **Yes** | Yes (RMI) |
| Email sending | No | No | Yes (RMI) |
| Auto-send invoices | No | No | Yes (RMI) |
| Direct SQL access | No | No | Yes (JDBC) |
| Stateless auth | Yes | No (session) | No (session) |
| No API license needed | **No** | **Yes** | **Yes** |

---

## Recommendations for MCP Implementation

### Primary Channel: Web Session v2
Use `/v2/c/{company}/{evidence}.json` with form-login session auth. This provides:
- Full CRUD on all 155 evidences
- All query features (filter, sort, page, field selection)
- All export formats (JSON, XML, CSV, PDF, ISDOC, XLSX)
- Metadata discovery (properties, relations, reports, actions)
- No API license required

### Fallback Channel: REST API v1
When the user has API license and `api_access_enabled=true`, offer Basic Auth mode:
- Simpler (stateless, no session management)
- No concurrent session limit concerns
- Better for long-running automated processes

### Configuration
```
FLEXIBEE_URL=https://localhost:5434
FLEXIBEE_COMPANY=lukas_viktora_2026_08_01_1343
FLEXIBEE_USERNAME=test
FLEXIBEE_PASSWORD=test123
FLEXIBEE_AUTH_MODE=auto    # auto|session|basic
# auto: try session first, fall back to basic if api_access_enabled
```

### Session Lifecycle for MCP
1. Login once at MCP server startup
2. Use keepalive every 5 minutes
3. Handle `remote-logout` (302 redirect) by re-authenticating
4. Logout on MCP server shutdown
5. Never create multiple concurrent sessions for the same user

---

## Password Hashing (Reference)

Format in `csuzivatel.heslo`: `sha256:SALT:HASH`
```
HASH = sha256("SALT:plaintext_password".encode("utf-8")).hexdigest()
```
Other supported algorithms: `sha512`, `sha1`, `md5`, `pbkdf2` (PBKDF2WithHmacSHA1, 1024 iterations, 128-bit key).

Legacy fallback (no colon in stored hash): `md5(plaintext_password)`.
