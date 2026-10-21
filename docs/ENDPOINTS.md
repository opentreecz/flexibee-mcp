# FlexiBee (ABRA Flexi) Server - Complete HTTP Endpoint Map

> Generated from decompilation of `winstrom-server-2026.4.12.jar` and `winstrom-server-impl-2026.4.12.jar`.
> Root JAX-RS resource: `cz.winstrom.net.server.services.RootResource` at `@Path("/")`

## Architecture Overview

The server uses:
- **Jersey (JAX-RS 1.x)** via `com.sun.jersey` for the REST API (servlet mapped to `/*`)
- **Restlet** for Activiti workflow engine REST API (servlet mapped to `/activiti/*`)
- **Custom servlets** for static content (`/flexibee-static/*`), downloads (`/download/*`), and mini-RMI (`/mini-rmi/*`)
- **Filters**: `FlexiBeeHandlerFilter` (all requests), `AuthHandlerFilter` (authentication)
- **Versioned API paths**: base (`/`), `/v2/`, `/v3/` with increasing features

---

## 1. Servlet-Level URL Mappings (from web.xml)

| Servlet | URL Pattern | Class | Purpose |
|---------|-------------|-------|---------|
| JerseyServlet | `/*` | `com.sun.jersey.server.impl.container.servlet.ServletAdaptor` | Main REST API |
| staticContent | `/flexibee-static/*` | `cz.winstrom.net.server.StaticContentServlet` | Static CSS/JS/images |
| downloadContent | `/download/*` | `cz.winstrom.net.server.DownloadContentServlet` | File downloads (attachments) |
| ActivitiServlet | `/activiti/*` | `org.restlet.ext.servlet.ServerServlet` | Activiti workflow REST API |
| MiniRMIRegistryServlet | `/mini-rmi/*` | `cz.winstrom.net.server.rmi.MiniRMIRegistryServlet` | Java RMI-over-HTTP |

---

## 2. Root-Level Endpoints (RootResource @Path("/"))

### 2.1 Server Info & Health

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/` | GET, HEAD | `RootResource` | Root - redirects to `/start` or first-user setup |
| `/index` | GET | `RootResource` | Same as root |
| `/start` | GET | `RootResource` | Auto-start - redirects to company or WUI |
| `/ping` | GET | `RootResource` | Health check (returns `{"success":true,"responseFrom":"appServer"}`) |
| `/status` | GET | `RootResource` → `Status` | Server status (version, sessions, memory, license, etc.) |
| `/status-check` | GET | `RootResource` → `Status` | Alias for `/status` |
| `/status/measure` | GET | `RootResource` | Fast status check (just `{"success":true}`) |
| `/echo` | PUT, POST | `RootResource` | Echo test (XML/JSON in → XML/JSON out) |
| `/sleep` | GET | `RootResource` | Sleep test (`?duration=` seconds) |
| `/reply` | GET | `RootResource` | Response size test (`?length=` characters) |
| `/constants` | GET | `RootResource` | Server constants |
| `/v2/constants` | GET | `RootResource` | Server constants (v2 API) |
| `/v3/constants` | GET | `RootResource` | Server constants (v3 API) |

### 2.2 Session & Connection Management

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/session-keep-alive` | GET | `RootResource` | Keep session alive |
| `/status/session` | GET | `RootResource` → `AuthSessionInfos` | List active sessions |
| `/status/session/{shortSessionId}` | GET | `RootResource` → `AuthSessionInfo` | Get session details |
| `/status/session/{shortSessionId}/logout` | POST | `RootResource` | Logout a session by ID |
| `/status/connection` | GET | `RootResource` → `Connections` | List active connections |
| `/status/user` | GET | `RootResource` → `UserProjectInfos` | List logged-in user projects |
| `/status/user/{username}/logout` | POST | `RootResource` | Logout user by username |
| `/v2/status/user/{username}/logout` | POST | `RootResource` | Logout user (v2) |
| `/v3/status/user/{username}/logout` | POST | `RootResource` | Logout user (v3) |

### 2.3 Cluster & Monitoring

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/status/cluster` | GET | `RootResource` → `BrotherStates` | Cluster node states |
| `/status/thread-pool` | GET | `RootResource` → `ServerPoolInfos` | Thread pool info |
| `/status/thread-pool/request-logger-control` | POST | `RootResource` | Toggle request logger |
| `/status/conn-pool` | GET | `RootResource` → `ConnPoolInfos` | Connection pool info |
| `/status/conn-pool/{companyId}/debug` | POST | `RootResource` | Enable debug on connection pool |
| `/status/conn-pool/{companyId}/close` | POST | `RootResource` | Close connection pool |
| `/status/replication` | GET | `RootResource` → `ReplicationInfos` | CouchDB replication status |
| `/status/web-hooks` | GET | `RootResource` → `WebHooksInfo` | Webhook processor status |
| `/status/web-hooks/control` | POST | `RootResource` | Restart/elect webhook watcher |
| `/status/api-usage` | GET | `RootResource` → `ApiStatsOneResource` | API usage stats (this instance) |
| `/status/api-usage-all` | GET | `RootResource` → `ApiStatsAllResource` | API usage stats (all instances) |
| `/status/db-usage` | GET | `RootResource` → `DbStatsOneResource` | DB usage stats (this instance) |
| `/status/db-usage-all` | GET | `RootResource` → `DbStatsAllResource` | DB usage stats (all instances) |

### 2.4 License

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/license` | GET | `RootResource` → `GroupLicenseResource` | Current user's license info |
| `/v2/license` | GET | `RootResource` → `GroupLicenseResource` | License (v2) |
| `/v3/license` | GET | `RootResource` → `GroupLicenseResource` | License (v3) |
| `/default-license` | GET | `RootResource` → `GroupLicenseResource` | Default group license |
| `/v2/default-license` | GET | `RootResource` → `GroupLicenseResource` | Default license (v2) |
| `/v3/default-license` | GET | `RootResource` → `GroupLicenseResource` | Default license (v3) |

### 2.5 Web UI (SPA)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/flexi` | GET | `RootResource` | Flexi web app root (serves SPA index.html from CDN) |
| `/flexi/{whatever}` | GET | `RootResource` | Flexi SPA catch-all (client-side routing) |
| `/app` | GET | `RootResource` | New web app root (alternative WUI path) |
| `/app/{whatever}` | GET | `RootResource` | New web app SPA catch-all |
| `/flexi-cdn/{fileName}` | GET | `RootResource` | CDN proxy for Flexi app static assets |

### 2.6 Documentation & Help

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/devdoc` | GET | `RootResource` | Developer documentation index |
| `/devdoc/{page}` | GET | `RootResource` | Developer documentation page |
| `/help` | GET | `RootResource` | Help index (redirects to table of contents) |
| `/help/{page}` | GET | `RootResource` | Help page (HTML content) |
| `/help/images/{img}` | GET | `RootResource` | Help page images (PNG) |
| `/error/{page}` | GET | `RootResource` | Error pages (Freemarker templates) |

---

## 3. Authentication & Authorization

### 3.1 Login/Logout (LoginLogoutResource)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/login-logout` | GET | `LoginLogoutResource` | Login/logout resource root |
| `/login-logout/login` | GET | `LoginLogoutResource` | Redirect to login form |
| `/login-logout/login` | POST | `LoginLogoutResource` | Perform login (form or JSON) |
| `/login-logout/login-form` | GET | `LoginLogoutResource` | Login form page |
| `/login-logout/logout` | GET | `LoginLogoutResource` | Logout current user |
| `/login-logout/check` | GET | `LoginLogoutResource` | Check auth session / refresh token |
| `/login-logout/session-keep-alive` | GET | `LoginLogoutResource` | Keep session alive |
| `/login-logout/first-user-form` | GET | `LoginLogoutResource` | Initial admin setup form |
| `/login-logout/first-user` | GET, PUT, POST | `LoginLogoutResource` | Create first admin user |
| `/login-logout/howto` | GET | `LoginLogoutResource` | How-to page |
| `/login-logout/lang` | GET | `LoginLogoutResource` | Change language (`?lang=`) |
| `/login-logout/connection-file` | GET | `LoginLogoutResource` | Download `.flexibee-connection` file |
| `/login-logout/sso` | GET | `LoginLogoutResource` | Initiate SSO authentication |
| `/login-logout/sso-saml` | POST | `LoginLogoutResource` | SAML SSO callback endpoint |
| `/login-logout/sso-openid` | GET | `LoginLogoutResource` | OpenID SSO callback endpoint |
| `/login-logout/remote-logout` | GET, POST | `LoginLogoutResource` | Remote logout (named users) |
| `/login-logout/prepare-remote-logout` | POST | `LoginLogoutResource` | Prepare remote logout |
| `/login-logout/user/{username}/logout` | POST | `LoginLogoutResource` | Logout specific user |
| `/login-logout/license-check` | GET | `LoginLogoutResource` | License check page |
| `/v2/login-logout/**` | * | `LoginLogoutResource` | All above with v2 API semantics |
| `/v3/login-logout/**` | * | `LoginLogoutResource` | All above with v3 API semantics |

### 3.2 Password Reset (PasswordResetResource)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/password-reset/send-email` | PUT, POST | `PasswordResetResource` | Send password reset token via email |
| `/password-reset/set-password` | PUT, POST | `PasswordResetResource` | Set new password using reset token |
| `/v2/password-reset/send-email` | PUT, POST | `PasswordResetResource` | Send reset (v2) |
| `/v2/password-reset/set-password` | PUT, POST | `PasswordResetResource` | Set password (v2) |
| `/v3/password-reset/send-email` | PUT, POST | `PasswordResetResource` | Send reset (v3) |
| `/v3/password-reset/set-password` | PUT, POST | `PasswordResetResource` | Set password (v3) |

---

## 4. User Management

### 4.1 Users (UsersResource at `/u`)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/u` | GET | `UsersResource` | List users |
| `/u` | PUT, POST | `UsersResource` | Create new user |
| `/u/disable-2fa` | PUT | `UsersResource` | Disable 2FA for a user |
| `/u/{userId}` | GET | `UserResource` | Get user details |
| `/u/{userId}` | PUT, POST | `UserResource` | Update user |
| `/u/{userId}` | DELETE | `UserResource` | Delete user |
| `/u/{userId}/contacts` | GET | `ContactsResource` | User's CardDAV contacts |
| `/u/{userId}/enable-2fa` | PUT | `UserResource` | Enable 2FA |
| `/u/{userId}/disable-2fa` | PUT | `UserResource` | Disable 2FA |
| `/u/{userId}/status-2fa` | GET | `UserResource` | Get 2FA status |
| `/u/{userId}/qrcode-2fa` | GET | `UserResource` | Get 2FA QR code image |
| `/v2/u/**` | * | `UsersResource` | Users (v2, with detailed info) |
| `/v3/u/**` | * | `UsersResource` | Users (v3) |
| `/u-deleted` | GET | `UsersResource` | List deleted users |
| `/u-system` | GET | `UsersResource` | List system users |

### 4.2 User Access (UsersAccessResource at `/a`)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/a` | GET | `UsersAccessResource` | List user-company access mappings |
| `/a/{username}` | GET | `UserAccessResource` | Get access for specific user |
| `/a/{username}` | PUT, POST | `UserAccessResource` | Update access for user |

### 4.3 Groups (GroupsResource at `/g`)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/g` | GET | `GroupsResource` | List license groups |
| `/g/{groupId}` | GET | `GroupResource` | Get group details |
| `/g/{groupId}` | PUT, POST | `GroupResource` | Update group |
| `/g/{groupId}/license` | GET | `GroupLicenseResource` | Get group license |

---

## 5. Company Data (the core API)

### 5.1 Companies List (CompaniesResource at `/c`)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/c` | GET | `CompaniesResource` | List companies |
| `/c-all` | GET | `CompaniesResource` | List ALL companies (including hidden) |
| `/c/{company}` | GET | `CompanyResource` | Get company details |
| `/c/{company}` | PUT, POST | `CompanyResource` | Edit company |
| `/c/{company}` | DELETE | `CompanyResource` | Delete company |
| `/c/{company}/restore` | PUT, POST | `CompaniesResource` | Restore company from `.winstrom-backup` |
| `/c/{company}/database-upgrade` | POST | `CompaniesResource` | Upgrade company database schema |
| `/v2/c/**` | * | `CompaniesV2Resource` | Companies (v2) |
| `/v3/c/**` | * | `CompaniesV3Resource` | Companies (v3) |

### 5.2 Company-Level Endpoints (under `/c/{company}/`)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/c/{company}/session-keep-alive` | GET | `AbstractCompanyResource` | Keep session alive in company context |
| `/c/{company}/statistics` | GET | `CompanyStatistics` | Company DB statistics |
| `/c/{company}/backup` | GET | `AbstractCompanyResource` | Download company backup |
| `/c/{company}/zaloha` | GET | `AbstractCompanyResource` | Backup UI page |
| `/c/{company}/app` | GET | `AbstractCompanyResource` | FlexiBee app in company context |
| `/c/{company}/metadata` | GET | `AbstractCompanyResource` | Full company metadata (all evidences, props, relations) |
| `/c/{company}/evidence-list` | GET | `AbstractCompanyResource` | List available evidences |
| `/c/{company}/all-evidence-list` | GET | `AbstractCompanyResource` | List ALL evidences (incl. hidden) |
| `/c/{company}/virtual-evidence-list` | GET | `AbstractCompanyResource` | List virtual evidences |
| `/c/{company}/all-virtual-evidence-list` | GET | `AbstractCompanyResource` | List ALL virtual evidences |
| `/c/{company}/permissions` | GET | `AbstractCompanyResource` | Get permissions for evidence |
| `/c/{company}/all-permissions` | GET | `AbstractCompanyResource` | Get permissions for ALL evidences |
| `/c/{company}/changes` | GET | `CompanyChanges` | Changes/changelog resource |
| `/c/{company}/hooks` | GET | `CompanyHooks` | Webhook hooks resource |
| `/c/{company}/logo/thumbnail` | GET | `AbstractCompanyResource` | Company logo thumbnail |
| `/c/{company}/parametr-types` | GET | `AbstractCompanyResource` | Parameter types page |
| `/c/{company}/sencha` | * | `CompanySencha` | Sencha/ExtJS UI data resource |
| `/c/{company}/config:k-collect` | GET | `AbstractCompanyResource` | K-Collect configuration XML |
| `/c/{company}/move-db` | PUT, POST | `AbstractCompanyResource` | Move company to different DB pool |
| `/c/{company}/move-db-instance` | PUT, POST | `AbstractCompanyResource` | Move company to different instance |
| `/c/{company}/configurable-reports` | GET | `AbstractCompanyResource` | List configurable reports |
| `/c/{company}/configurable-reports/{reportId}` | GET | `AbstractCompanyResource` | Get specific report |
| `/c/{company}/configurable-reports/{reportId}/metadata` | GET | `AbstractCompanyResource` | Report metadata |
| `/c/{company}/configurable-reports/{reportId}/configuration` | GET, PUT, POST | `AbstractCompanyResource` | Report configuration |
| `/c/{company}/configurable-reports/{reportId}/localization` | GET, PUT, POST | `AbstractCompanyResource` | Report localization |

### 5.3 Evidence CRUD (under `/c/{company}/{evidence}/`)

Each evidence type (e.g., `faktura-vydana`, `adresar`, `banka`, etc.) exposes these sub-paths via `EvidenceResource`:

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/c/{company}/{evidence}` | GET | `EvidenceResource` | List records (with filtering, paging) |
| `/c/{company}/{evidence}` | PUT, POST | `EvidenceResource` | Create/update records (XML/JSON import) |
| `/c/{company}/{evidence}/{id}` | GET | `EvidenceResource` | Get single record by ID or code |
| `/c/{company}/{evidence}/{id}` | PUT, POST | `EvidenceResource` | Update record |
| `/c/{company}/{evidence}/{id}` | DELETE | `EvidenceResource` | Delete record |
| `/c/{company}/{evidence}/properties` | GET | `EvidenceResource` | List evidence properties/fields |
| `/c/{company}/{evidence}/all-properties` | GET | `EvidenceResource` | List ALL properties (incl. hidden) |
| `/c/{company}/{evidence}/dictionary` | GET | `EvidenceResource` | Get dictionary for evidence |
| `/c/{company}/{evidence}/invisible` | GET | `EvidenceResource` | List invisible properties |
| `/c/{company}/{evidence}/relations` | GET | `EvidenceResource` | List evidence relations |
| `/c/{company}/{evidence}/reports` | GET | `EvidenceResource` | List available reports |
| `/c/{company}/{evidence}/workflows` | GET | `EvidenceResource` | List available workflows |
| `/c/{company}/{evidence}/named-filters` | GET | `EvidenceResource` | List named filters |
| `/c/{company}/{evidence}/actions` | GET | `EvidenceResource` | List available actions |
| `/c/{company}/{evidence}/toolbars` | GET | `EvidenceResource` | List toolbar buttons |
| `/c/{company}/{evidence}/opensearch` | GET | `EvidenceResource` | OpenSearch description XML |
| `/c/{company}/{evidence}/schema-export` | GET | `EvidenceResource` | Export schema |
| `/c/{company}/{evidence}/schema-import` | GET | `EvidenceResource` | Import schema |
| `/c/{company}/{evidence}/sencha` | * | `EvidenceSencha` | Sencha/ExtJS data resource |
| `/c/{company}/{evidence}/query` | PUT, POST | `EvidenceResource` | Custom query (XML/JSON body) |
| `/c/{company}/{evidence}/query/{actionName}` | PUT, POST | `EvidenceResource` | Query + action |
| `/c/{company}/{evidence}/get` | PUT, POST | `EvidenceResource` | Get records by filter in body |
| `/c/{company}/{evidence}/{id}/{subEvidence}` | GET, PUT, POST | `EvidenceResource` | Sub-evidence (line items, attachments) |
| `/c/{company}/{evidence}/{id}/relations` | GET | `EvidenceResource` | Record relations |
| `/c/{company}/{evidence}/{id}/workflows` | GET | `EvidenceResource` → `WorkflowDescriptors` | Record workflows |
| `/c/{company}/{evidence}/{id}/workflows/{processId}/start` | PUT, POST | `EvidenceResource` | Start workflow on record |
| `/c/{company}/{evidence}/{id}/workflow-signal/{signalId}` | PUT, POST | `EvidenceResource` | Send workflow signal |
| `/c/{company}/{evidence}/{id}/workflow-message/{messageId}` | PUT, POST | `EvidenceResource` | Send workflow message |
| `/c/{company}/{evidence}/{id}/a/{action}` | GET, PUT, POST | `EvidenceResource` | Execute action on record |

### 5.4 Specialized Evidence Types

These evidence types have custom `EvidenceResource` subclasses with additional functionality:

| Evidence Path | Custom Class | Special Features |
|---------------|-------------|------------------|
| `priloha` | `AttachmentEvidenceResource` | File upload/download for attachments |
| `nastaveni` | `NastaveniEvidenceResource` | Company settings |
| `certifikat` | `CertifikatEvidenceResource` | SSL certificates |
| `certifikat-eet` | `CertifikatEetEvidenceResource` | EET certificates |
| `certifikat-finbricks` | `CertifikatFinbricksEvidenceResource` | Finbricks certificates |
| `skladova-karta` | `SkladKartaEvidenceResource` | Inventory cards |
| `faktura-vydana` | Custom | Issued invoices (has PDF/report generation) |
| `faktura-prijata` | Custom | Received invoices |
| `banka` | Custom | Bank transactions |
| `pokladna` | Custom | Cash register |
| `mzdy` | Custom | Payroll |
| `adresar` | Custom | Address book / contacts |
| And ~50 more... | Various `*EvidenceResource` | Each evidence type in `EvidenceType` enum |

---

## 6. Administration

### 6.1 Admin Resource (at `/admin`)

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/admin` | GET | `AdminResource` | Admin page |
| `/admin/batch` | PUT, POST | `AdminResource` | XML batch operations (create/update/delete companies, users, access) |
| `/admin/auth-settings` | GET | `AdminResource` | Auth settings page |
| `/admin/server-settings` | GET | `AdminResource` | Server settings page |
| `/admin/disk-usage` | GET | `AdminResource` | Disk usage page |
| `/admin/zalozeni-firmy` | GET, PUT, POST | `AdminResource.CreateAction` | Create new company |
| `/admin/organization-type-list` | GET | `AdminResource` | List organization types |
| `/admin/logging` | POST | `AdminResource` | Reconfigure logging |
| `/admin/intercom-update` | GET | `AdminResource` | Get Intercom JWT (v2 only) |
| `/v2/admin/**` | * | `AdminResource` | Admin (v2) |
| `/v3/admin/**` | * | `AdminResource` | Admin (v3) |

---

## 7. Server Settings & Certificates

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/setting` | GET | `SettingsResource` | Server-level settings list |
| `/setting/{key}` | GET, PUT, POST | `SettingResource` | Get/set specific setting |
| `/certificate` | GET, PUT, POST | `CertificateResource` | SSL certificate management |

---

## 8. WebDAV / CardDAV

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/.well-known` | GET | `WellKnownResource` | Well-known resource |
| `/.well-known/carddav` | GET, OPTIONS, PROPFIND | `WellKnownResource` | CardDAV discovery (redirects to user contacts) |
| `/u/{user}/contacts` | GET, PROPFIND | `ContactsResource` | CardDAV contacts for user |
| `/u/{user}/contacts/{company}` | GET, PROPFIND | `ContactsResource` | CardDAV contacts for company |

---

## 9. Cloud-Only Endpoints

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/i` | GET | `InstancesResource` | List cloud instances |
| `/i/{instanceId}` | GET, PUT | `InstanceResource` | Get/update instance |
| `/i/{instanceId}/redirect/{companyId}` | GET | `InstanceRedirectResource` | Redirect to company on instance |
| `/db-pools` | GET | `DbServerPoolsResource` | List DB server pools |
| `/db-pools/{poolId}` | GET, PUT | `DbServerPoolResource` | DB server pool details |
| `/app-pools` | GET | `AppServerPoolsResource` | List app server pools |
| `/app-pools/{poolId}` | GET, PUT | `AppServerPoolResource` | App server pool details |
| `/company-pools` | GET | `CompaniesPoolsResource` | List company-pool assignments |
| `/company-pools/{companyId}` | GET, PUT | `CompanyPoolsResource` | Company pool assignment |

---

## 10. Activiti Workflow REST API

Mapped via `FlexiBeeHandlerFilter` pattern: `/c/{company}/activiti/**` → internally rewritten to `/activiti/{company}/**`

Standard Activiti REST API endpoints including:

| Path Pattern | Purpose |
|-------------|---------|
| `/c/{company}/activiti/process-definitions` | List process definitions |
| `/c/{company}/activiti/process-instances` | List/create process instances |
| `/c/{company}/activiti/tasks` | List/manage tasks |
| `/c/{company}/activiti/executions` | Manage executions |
| `/c/{company}/activiti/history` | Process history |
| `/c/{company}/activiti/management` | Engine management |
| `/c/{company}/activiti/identity` | Identity management |
| `/c/{company}/activiti/repository` | Repository operations |

---

## 11. RMI-over-HTTP

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/mini-rmi/*` | POST | `MiniRMIRegistryServlet` | Java serialized RMI calls |
| `/mini-rmi/WinStromConnector` | POST | Servlet | Core connector (desktop client) |
| `/mini-rmi/BrotherDiscovery` | POST | Servlet | Cluster peer discovery (restricted) |

---

## 12. Static Resources

| Path | Methods | Class | Purpose |
|------|---------|-------|---------|
| `/flexibee-static/**` | GET | `StaticContentServlet` | CSS, JS, images (with cache headers) |
| `/download/**` | GET | `DownloadContentServlet` | File downloads |

---

## 13. Output Format Negotiation

All REST API endpoints support content negotiation via:
- **URL suffix**: `.json`, `.xml`, `.csv`, `.dbf`, `.xls`, `.xlsx`, `.pdf`, `.isdocx`, `.isdoc`, `.html`
- **Accept header**: `application/json`, `application/xml`, `text/html`, `text/csv`, etc.
- **Query param**: `?format=json|xml|csv|...`

Handled by `MyUriConnegFilter`.

---

## 14. Evidence Types (Complete List)

These are the evidence paths available under `/c/{company}/{evidence}`:

Common evidence types include (there are ~100+):
`faktura-vydana`, `faktura-prijata`, `banka`, `pokladna`, `interni-doklad`, `skladovy-pohyb`,
`adresar`, `cenik`, `objednavka-vydana`, `objednavka-prijata`, `nabidka-vydana`, `nabidka-prijata`,
`poptavka-vydana`, `poptavka-prijata`, `dodavatelska-smlouva`, `odberatelska-smlouva`,
`mzdy`, `personalistika`, `majetek`, `odpisy`, `strom`, `stredisko`, `cinnost`, `zakazka`,
`nastaveni`, `priloha`, `uzivatel`, `role`, `smena`, `prace`, `ucet`, `predpis-zauctovani`,
`ucetni-denik`, `saldo`, `pokladni-pohyb`, `bankovni-ucet`, `typ-organizace`,
`forma-uhrad`, `stav-skladu-k-datu`, `skladova-karta`, `cenova-uroven`,
`merna-jednotka`, `stat`, `mena`, `kurz`, `kurz-pro-cenotvorbu`, `sazba-dph`, `typ-dokladu`,
`rada-banka`, `rada-faktura-vydana`, `rada-faktura-prijata`, `rada-interni-doklad`,
`rada-pokladna`, `rada-pohledavka`, `rada-zavazek`, `penezni-ustav`,
`certifikat`, `certifikat-eet`, `kontakt`, `kontaktni-osoba`,
`dashboard-panel`, `dashboard-sharing`, `worksheet`, `workshift`,
`eet-komunikace`, `intrastat`, `saldo-k-datu`, and many more...

The complete list is enumerable at runtime via `/c/{company}/evidence-list.json`.

---

## 15. Query Parameters (Common across endpoints)

| Parameter | Purpose |
|-----------|---------|
| `detail` | Detail level: `id`, `summary`, `full`, `custom:field1,field2` |
| `limit` | Page size |
| `start` | Page offset |
| `order` | Sort: `field+`, `field-` |
| `filter` | Filter expression (FlexiBee query language) |
| `relations` | Include relations |
| `includes` | Include sub-evidences |
| `add-row-count` | Include total count |
| `dry-run` | Validate without saving |
| `fail-on-warning` | Fail if warnings |
| `code-as-id` | Use code instead of numeric ID |
| `code-in-id` | Return code in ID field |
| `no-ext-ids` | Exclude external IDs |
| `no-ids` | Exclude IDs |
| `encoding` | Response encoding |
| `authSessionId` | Session ID (query param auth) |
