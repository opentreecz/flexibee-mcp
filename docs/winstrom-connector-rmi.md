# WinStromConnector RMI Interface — Complete Decompilation

Decompiled from FlexiBee 2026.4.12 JARs. This documents the complete Java RMI
interface the desktop client uses to communicate with the FlexiBee server.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Transport: Mini-RMI Protocol](#transport-mini-rmi-protocol)
3. [WinStromConnector Interface (all methods)](#winstromconnector-interface)
4. [WinStromConnectorImpl (server implementation)](#winstromconnectorimpl)
5. [Data Types](#data-types)
6. [Server-Side Handler](#server-side-handler)
7. [Key Findings](#key-findings)

---

## Architecture Overview

```
Desktop Client                          Server
┌──────────────┐    HTTPS POST     ┌──────────────────────────┐
│ WinStromClient│◄────────────────►│ MiniRMIRegistryServlet   │
│              │                   │  (extends ServletRegistry)│
│  ConnectorImpl│  /mini-rmi/      │                          │
│  (HTTP POST) │  WinStromConnector│  WinStromConnectorImpl   │
│              │  ?methodName      │  @PublishAs(WinStromConn.)│
│  RemoteStub  │  &user=...       │                          │
│  (cglib proxy)│  &authSessionId= │  CentralServerDatabase   │
└──────────────┘                   └──────────────────────────┘

Wire format: Java ObjectOutputStream serialization
Content-Type: application/winstrom-rmi
URL pattern: https://{host}:{port}/mini-rmi/WinStromConnector/?{methodName}&user={user}&authSessionId={shortId}
```

**How it works:**
1. Client calls `Naming.lookup(connector, host, port, WinStromConnector.class, timeout)`
2. This creates a cglib proxy implementing `WinStromConnector`
3. Every method call on the proxy → `RemoteStub.invoke()` → serializes a `MethodCall` object
4. HTTP POST to `/mini-rmi/WinStromConnector/?methodName&user=...&authSessionId=...`
5. Server's `ServletRegistry.doPost()` deserializes, finds the bound object, invokes via reflection
6. Response wrapped in `Response(returnValue, exception)`, serialized back

---

## Transport: Mini-RMI Protocol

### Naming (client-side lookup)

```java
package cz.winstrom.net.rmi;

public final class Naming {
    public static final String MINI_RMI_PREFIX = "mini-rmi";

    public static <T extends Remote> T lookup(
        Connector connector, String host, String port,
        Class<T> remoteRootInterface, long timeout
    ) throws RemoteException {
        String url = "https://" + host + ":" + port + "/mini-rmi/";
        String name = remoteRootInterface.getSimpleName();
        RemoteStubWrapper wrapper = new RemoteStubWrapper(name, remoteRootInterface.getCanonicalName());
        return (T) MarshallInputStream.enhanceStub(connector, url, wrapper, timeout);
    }
}
```

### Connector (HTTP transport interface)

```java
package cz.winstrom.net.rmi;

public interface Connector {
    byte[] getPostConnection(String url, byte[] data) throws IOException;
    Future<Response> getPostConnectionAsync(String url, byte[] data, AsyncHandler<Response> handler) throws IOException;
    void closeIdleConnections();
}
```

### ConnectorImpl (client-side HTTP implementation)

```java
package cz.winstrom.client;

public class ConnectorImpl implements Connector {
    public static final String PROTOCOL = "https://";
    public static final String API_PATH = "/c/";
    public static final int CONNECTION_TIMEOUT = 15000;

    @Override
    public byte[] getPostConnection(String url, byte[] content) throws IOException {
        PostMethod post = new PostMethod(url);
        post.setRequestHeader("Content-Type", "application/winstrom-rmi");
        post.setRequestEntity(new ByteArrayRequestEntity(content));
        CLIENT.executeMethod(post);
        return post.getResponseBody();
    }

    // Sets X-authSessionId header on all requests
    private static void setAuthHeader(HttpMethodBase method, Authenticator authenticator) {
        if (authenticator != null && authenticator.getAuthSessionId() != null) {
            method.setRequestHeader("X-authSessionId",
                authenticator.getAuthSessionId().getSessionId());
        }
    }

    // Uses Apache Commons HttpClient 3.x with custom SSL (MySSLProtocolSocketFactory)
    // Registers "https" protocol with trust-all SSL
}
```

### RemoteStub (cglib proxy invocation handler)

```java
package cz.winstrom.net.rmi;

public class RemoteStub implements Remote, Serializable, InvocationHandler {
    protected final String objectURL;   // e.g. "https://host:port/mini-rmi/WinStromConnector/"
    protected final String remoteUrl;   // e.g. "https://host:port/mini-rmi/"
    protected final Connector connector;
    protected final long timeout;

    @Override
    public Object invoke(Object proxy, Method method, Object[] args) throws Exception {
        // Skips "finalize" calls
        // Builds MethodCall with method name, args, param class names
        // Calls makeCall() which:
        //   1. Serializes MethodCall via ObjectOutputStream
        //   2. POSTs to objectURL + methodCall.buildQueryString()
        //   3. Deserializes Response from server
        //   4. Returns response.returnValue or throws response.exception
        //
        // After "keepAlive" calls, also calls connector.closeIdleConnections()
    }
}
```

### MethodCall (wire format)

```java
package cz.winstrom.net.rmi.internal;

public final class MethodCall implements Serializable {
    public static final String MINIRMI_CONTENT_TYPE = "application/winstrom-rmi";
    public final String methodName;
    public final Object[] parameters;
    public final String[] paramClassNames;

    // Query string format: ?methodName&user=XXX&authSessionId=YYY
    public String buildQueryString() {
        StringBuilder query = new StringBuilder("?").append(this.methodName);
        // Extracts user and authSessionId from Authenticator parameters
        return query.toString();
    }
}
```

### Response (wire format)

```java
package cz.winstrom.net.rmi.internal;

public final class Response implements Serializable {
    public final Object returnValue;
    public final RemoteException exception;
}
```

### MarshallInputStream / MarshalOutputStream

The input stream resolves `RemoteStubWrapper` objects back into cglib proxies
(for nested Remote references). The output stream replaces `Remote` objects with
`RemoteStubWrapper` (using `@PublishAs` annotation if present).

---

## WinStromConnector Interface

**Package:** `cz.winstrom.net.connector`  
**Extends:** `java.rmi.Remote`  
**Source JAR:** `winstrom-connector-2026.4.12.jar`

### Constants

```java
public static final String NAME = "WinStromConnector";
public static final String MAGIC = "JDWSJDWS";
public static final String BROADCAST_MAGIC = "FlexiBee.eu";
public static final String BROADCAST_MAGIC_SEARCH = "FlexiBee.eu serverSearch";
public static final String AUTH_SESSION_ID = "authSessionId";
public static final String X_AUTH_SESSION_ID = "X-authSessionId";
public static final String REFRESH_TOKEN = "refreshToken";
public static final String CSRF_TOKEN = "csrfToken";
public static final String ONE_TIME_PASSWORD = "otp";
public static final String INTERCOM_HASH = "intercomUserHash";
public static final String INTERCOM_JWT = "intercomJwt";
public static final String COOKIE_LAST_USERNAME = "username";
public static final String COOKIE_USE_NEW_FORM = "useNewForm";
public static final String COOKIE_SMALL_MENU = "useSmallMenu";
public static final String COOKIE_CSRF_TOKEN = "CSRF-Token";
public static final String AUTH_INSTANCE_ADMIN_KEY = "InstanceAdminKey";
public static final String X_AUTH_INSTANCE_ADMIN_KEY = "X-InstanceAdminKey";
public static final String IN_DESKTOP_APP_PARAM = "inDesktopApp";
public static final int AUTH_SESSION_KEEPER_INTERVAL = 300000;  // 5 minutes
public static final String KEEP_ALIVE_METHOD_NAME = "keepAlive";
```

### Complete Method Signatures (57 methods)

#### Database/Company Management

```java
// List all companies (databases) the user has access to
List<DatabaseInfo> findDatabaseInfos(boolean withAccess, Boolean admin,
    Boolean connected, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Find a specific company by its dbNazev (database name)
DatabaseInfo findDatabaseInfo(String dbNazev, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Count companies in a license group
Long findDatabaseInfoCount(OpaqueId licenseGroupId, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Create or update company metadata in central server
void createOrUpdateDatabaseInfo(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Delete ALL data about a company (metadata + database + roles)
void deleteAllAboutCompany(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Check if a new database can be created (no name collision, no existing role)
boolean canCreateDatabase(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, WSServerStateException;

// Check if a PostgreSQL database physically exists for this company
DatabaseExists existsDatabase(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Find a unique database name (appends suffix if needed)
String findUniqueDbName(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Create a new company database
boolean createDatabase(CreateCompanyInfo create, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Delete a company database (requires deleteCompany permission)
boolean deleteDatabase(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Check if database schema can be updated
boolean canUpdateDatabase(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Update (migrate) a company database schema
void updateDatabase(DatabaseInfo databaseInfo, Authenticator auth)
    throws WSServerRTException, WSNotAuthorizedException, RemoteException,
           WSServerStateException, FixLobsOwnerException;

// Get JDBC connection info for a company
DatabaseInfo getConnection(String serverId, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Mangle (transform) JDBC connection URL for cloud/multi-instance
ConnectionInfo mangleConnectionInfo(ConnectionInfo original, Authenticator auth)
    throws WSServerStateException;
```

#### User Management

```java
// List all users
List<UserInfo> findUserInfos(Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// List users from companies the caller has access to
List<UserInfo> findUserInfosFromMyCompanies(Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Count users in a license group
Long findUserInfoCount(OpaqueId licenseGroupId, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Count connected applications by type
Long findConnectedApplicationCount(ConnectionType type, OpaqueId groupId,
    Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Count company admins
Long findCompanyAdminCount(Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Find a single user by login name
UserInfo findUserInfo(String login, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException;

// Find a user by email
Optional<UserInfo> findUserByEmail(String email, Authenticator auth)
    throws WSServerStateException;

// Create or update a user
void createOrUpdateUserInfo(UserInfo userInfo, Authenticator auth)
    throws RemoteException, WSServerStateException,
           NamedUsersLimitExceededException, WSUserEmailDuplicatedException,
           WSNotAuthorizedException;

// Create the initial admin user (only if no admins exist)
void createOrUpdateAdminUserInfo(UserInfo userInfo, String password)
    throws RemoteException;

// Delete a user
void deleteUserInfo(UserInfo userInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Create multiple users and grant them access to a company
List<UserInfo> createUsersWithAccess(List<UserInfo> users,
    DatabaseInfo databaseInfo, Authenticator auth)
    throws WSApplicationException, RemoteException, WSServerStateException;

// Create a system user with a specific password
void createSystemUserWithPassword(UserInfo userInfo, DatabaseInfo databaseInfo,
    String password, Authenticator auth)
    throws WSApplicationException, RemoteException, WSServerStateException;

// Remove a user from a specific company
void deleteUserFromCompany(String username, DatabaseInfo databaseInfo,
    Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Get the default license group ID
OpaqueId findDefaultSkupina();
```

#### Authentication & Password

```java
// Authenticate a user — returns an Authenticator token
Authenticator authentificateUser(KonektorAuthenticator auth)
    throws WSNotAuthorizedException, RemoteException,
           WSServerStateException, WSBlockedException;

// Check if a user's password can be changed
boolean canChangeUsersPassword(String username, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Check if 2FA is enabled for a user
boolean isTwoPhaseAuthEnabled(String username)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Verify old password before changing (always returns true — stub!)
boolean verifyPrevPasswordForChange(String username, String oldPassword,
    Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Check password quality (0 = too weak)
int verifyPasswordQuality(String username, String password, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Change a user's password
void changeUserPassword(String username, String newPassword, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Change the OTP secret for the current user
void changeUserSecret(Authenticator auth, String secret)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Change OTP secret for another user (admin operation)
void changeUserSecretForUser(Authenticator auth, String username)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Logout — disposes the auth session
void logout(Authenticator auth)
    throws RemoteException, WSNotAuthorizedException, WSServerStateException;
```

#### Session Management

```java
// Acquire a company session (enter a company for work)
@AcquireCompanySessionMethod
CompanySession acquireSession(String companyId, Authenticator auth, Calendar clientDate)
    throws RemoteException, WSServerStateException, WSNotAuthorizedException;

// Re-acquire session (mark other sessions obsolete for named-user licensing)
void reacquireSession(Authenticator auth)
    throws RemoteException, WSServerStateException, WSNotAuthorizedException;

// Keep-alive ping (deprecated overload without client version)
@Deprecated
KeepAliveResponse keepAlive(CompanySessionId sessionId, Authenticator auth)
    throws RemoteException, WSServerStateException, WSNotAuthorizedException;

// Keep-alive ping with version check
KeepAliveResponse keepAlive(CompanySessionId sessionId, Authenticator auth,
    String clientVersion)
    throws RemoteException, WSServerStateException, WSNotAuthorizedException;

// List other active sessions for the same user
List<AuthSession> getOtherAuthSessions(Authenticator auth)
    throws RemoteException, WSNotAuthorizedException, WSServerStateException;

// Release a company session (leave a company)
@ReleaseCompanySessionMethod
void releaseSession(CompanySessionId sessionId, Authenticator auth)
    throws RemoteException, WSServerStateException, WSNotAuthorizedException;
```

#### Access Control

```java
// Create or update an access record (user <-> company mapping)
void createOrUpdateAccessInfo(AccessInfo accessInfo, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Find access info for a specific user+company pair
AccessInfo findAccessInfo(UserInfo user, DatabaseInfo database, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// List all access records (returns Map<companyDbName, List<AccessInfo>>)
Map<String, List<AccessInfo>> listAccessInfo(Authenticator auth)
    throws WSNotAuthorizedException, WSServerStateException;

// Delete access (revoke user from company)
void deleteAccessInfo(UserInfo user, DatabaseInfo database, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Grant admin PostgreSQL roles to a user for a company
void grantAdminRoles(UserInfo user, DatabaseInfo database, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;

// Check if a connection to a company is allowed by license
boolean canConnectApplication(String companyId, Authenticator auth,
    boolean checkAlreadyAuth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException,
           WSCompanyStateException, WSAlreadyAuthenticatedException;

// Get named users count for licensing
int getNamedUsersCount(Authenticator auth, NamedUserType userType,
    OpaqueId licenseGroup, String excludedUser)
    throws RemoteException, WSNotAuthorizedException, WSServerStateException;

// Get named admins count for licensing
int getNamedAdminsCount(Authenticator auth, NamedUserType userType,
    OpaqueId licenseGroup)
    throws RemoteException, WSNotAuthorizedException, WSServerStateException;
```

#### License Management

```java
// Get license data (binary blob) for a license group
LicenseData findLicense(OpaqueId licenseGroupId, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException;

// Upload/update license data
void createOrUpdateLicense(LicenseData license, OpaqueId licenseGroupId,
    Authenticator auth)
    throws WSNotAuthorizedException, RemoteException;

// Delete license data
void deleteLicense(OpaqueId licenseGroupId, Authenticator auth)
    throws WSNotAuthorizedException, RemoteException;

// Check license errors before creating a company
String getFirstLicenseErrorCreatingCompany(CompanyInfo companyInfo,
    Authenticator auth)
    throws WSNotAuthorizedException, RemoteException, WSServerStateException;
```

#### Server Information & Settings

```java
// Get server info (version, PgSQL version, cloud status, etc.)
ServerInfo getServerInfo() throws RemoteException;

// Get SMTP configuration (deprecated)
@Deprecated
SmtpConf getServerXMLSmtpConf(Authenticator auth)
    throws WSServerStateException, WSNotAuthorizedException, RemoteException;

// Get server settings (OTP, SSO, etc.)
SettingInfo findServerSetting(ServerVariables settingName, Authenticator auth)
    throws WSNotAuthorizedException, WSServerStateException;

// Update server settings
void createOrUpdateSettingInfo(SettingInfo setting, Authenticator auth)
    throws WSNotAuthorizedException, WSServerStateException;

// Get auth configuration for a specific user (password/OTP/SSO)
AuthInformation getAuthInformation(String username)
    throws RemoteException, WSServerStateException;

// Get global auth configuration
AuthInformation getAuthGlobalInformation()
    throws RemoteException, WSServerStateException;
```

#### Utility

```java
// Send email via the server's SMTP
void sendMail(String mail, Authenticator auth, SmtpConf smtpConf)
    throws RemoteException, WSServerStateException,
           WSNotAuthorizedException, MailException;

// Trigger automatic invoice sending (async via MailEventLoop)
void automatickyOdeslatFaktury(String companyId, String userId,
    List<Integer> invoiceIds, Authenticator auth)
    throws RemoteException, WSServerStateException, WSNotAuthorizedException;
```

---

## WinStromConnectorImpl

**Package:** `cz.winstrom.net.server.rmi`  
**Annotation:** `@PublishAs(as = WinStromConnector.class)`  
**Source JAR:** `winstrom-server-2026.4.12.jar`

### Singleton Initialization

```java
// Created as a singleton with TxProxy wrapping for transactional methods
static {
    CentralServerDatabase csDatabase = FlexiContainer.getMainComponent(CentralServerDatabase.class);
    if (csDatabase instanceof HasTxManager) {
        TxManager txManager = ((HasTxManager) csDatabase).getTxManager();
        instance = TxProxy.create(WinStromConnectorImpl.class, txManager, WSCI_EXCEPTION_HANDLER);
    } else {
        instance = new WinStromConnectorImpl();
    }
    // Wire dependencies
    instance.csDatabase = csDatabase;
    instance.databaseManager = FlexiContainer.getMainComponent(DatabaseManager.class);
    instance.authentication = FlexiContainer.getMainComponent(Authentication.class);
    instance.authSessionRegistry = FlexiContainer.getMainComponentLazy(AuthSessionRegistry.class);
    // ...

    // Wrap in proxies for cross-cutting concerns:
    instance = CurrentCompanySettingProxy.create(instance);  // Sets CurrentInstanceCompanyKeys
    instance = CurrentUserSettingProxy.create(instance);     // Sets HttpCurrentRequest.currentUser
}
```

### Key Implementation Details

**Authentication pattern:** Nearly every method follows:
```java
this.checkAuthAndServerState(authenticator);  // verify auth + check warranty
UserInfo userInfo = this.authentication.authenticate(authenticator);  // get UserInfo
```

**`checkAuthAndServerState`** checks:
1. Authenticator is not null
2. Server warranty (license) is not expired

**`acquireSession`** — creates a company session:
- Validates user has access to the company
- Checks license (read-only if license is read-only or user type is READ_ONLY)
- Marks other sessions obsolete for named-user licensing
- Returns `CompanySession(CompanySessionId(random32hexChars, companyId), readOnly)`

**`keepAlive`** — session heartbeat:
- Checks server version compatibility with client version
- Returns status: `OK`, `OBSOLETE` (remote logout), `QUIT_LOGOUT`, `QUIT_MAINTANANCE`

**`authentificateUser`** — delegates to `UserLoginManager.authenticate(auth, ClientType.DESKTOP)`

**`getConnection`** — returns JDBC connection info with `$$centralServer$$:{port}` placeholder

**`createDatabase`** — delegates to `CompanyDatabase.createDatabase()`

**`deleteUserFromCompany`** — complex: revokes PostgreSQL roles, removes access, optionally drops user if last access

**`canConnectApplication`** — license-aware connection check:
- Verifies company exists and is "established"
- Checks user access (or `manageAll` permission)
- Validates connection type (RW/RO, not ALL)
- Calls `CsLicenseHelper.canConnectApplication(connType, DESKTOP, userType, auth, checkAlreadyAuth)`

**`getServerInfo`** — returns:
```java
new ServerInfo(hasUsers, winstromVersion, dbServerVersion, pgsqlVersion,
    centralSchemaVersion, companySchemaVersion, warrantyExpired, isOldSchema,
    isCloud, hasRestoreSkipList);
```

---

## Data Types

### CompanySession

```java
public class CompanySession implements Serializable {
    private final CompanySessionId companySession;
    private final boolean isReadOnly;
}
```

### CompanySessionId

```java
public class CompanySessionId implements Serializable {
    public static final CompanySessionId EMPTY = new CompanySessionId("(empty)", null);
    private final String sessionId;   // 32-char hex random string
    private final String companyId;   // e.g. "demo_cz"
}
```

### DatabaseExists (enum)

```java
public enum DatabaseExists {
    YES,
    NO_CAN_DELETE,      // database doesn't exist but role can be cleaned up
    NO_CANNOT_DELETE;   // cloud: can't delete what doesn't exist
}
```

### KeepAliveResponse

```java
public class KeepAliveResponse implements Serializable {
    private final Status status;
    private final boolean allowQuit;
    private final boolean allowOk;
    private final String message;

    public enum Status {
        OK,                 // session is alive
        QUIT_LICENSE,       // license issue, must quit
        QUIT_MAINTANANCE,   // version mismatch, must quit
        QUIT_LOGOUT,        // session was terminated remotely
        MESSAGE,            // server has a message
        OBSOLETE;           // another session took over (named users)
    }
}
```

### AuthInformation

```java
public class AuthInformation implements Serializable {
    public boolean usePassword;  // password auth enabled
    public boolean useOTP;       // OTP (2FA) enabled
    public boolean useSSO;       // SSO enabled
}
```

### CreateCompanyInfo

```java
public class CreateCompanyInfo implements Serializable {
    private String dbNazev;         // database name (slug)
    private String nazev;           // display name
    private String stat;            // country code
    private String typOrganizace;   // organization type
    private String kodMena;         // currency code
    private String ucto;            // accounting type
    private boolean fillDemo;       // fill with demo data
    private boolean isPodvojUcto;   // double-entry bookkeeping
}
```

### RestoreParams

```java
public class RestoreParams {
    public boolean forTesting = false;
    public boolean disableEet = false;           // disable EET (electronic sales records)
    public boolean disableWebHooks = false;
    public boolean disableSmlouvyAutoMail = false; // disable auto-mail for contracts
    public boolean skipZurnal = false;            // skip audit journal
    public boolean skipChangelog = false;
}
```

### RefreshToken

```java
public class RefreshToken {
    public static final int EXPIRY_DAYS = 7;
    private final String plainToken;
    private final String hashedToken;  // bcrypt via AuthUtils
    private final String username;
    private final Date expiryDate;
    private final String uuid;

    // Full token format: "uuid:plainToken"
    public String getFullToken() { return uuid + ":" + plainToken; }
}
```

### ClientType (enum)

```java
public enum ClientType {
    MOBILE, TABLET, DESKTOP, WEB, API, FLEXI;

    public boolean isLegacyWebUI() { return this == WEB || this == TABLET || this == MOBILE; }
    public boolean isWebUI() { return this == FLEXI || this.isLegacyWebUI(); }
}
```

### ServerVariables (enum)

```java
public enum ServerVariables {
    OTP_REQUIRE(true, Boolean.class, "auth-otp"),
    AUTHN_TYPE(true, String.class),              // default: "INTERNAL"
    SSO_REQUIRE(true, Boolean.class),
    SSO_LOGIN_URL(true, String.class, "auth-sso-openid", "auth-sso-saml"),
    SSO_LOGOUT_URL(true, String.class),
    SSO_CHNG_PASSWD_URL(true, String.class, "auth-sso-openid", "auth-sso-saml"),
    SSO_CERT_FINGERPRINT(true, String.class, "auth-sso-saml"),
    SSO_SERVICE_PROVIDER_ID(true, String.class, "auth-sso-saml"),
    WEB_DISABLE_HTML_LOGIN(true, Boolean.class),
    PASSWORD_QUALITY(true, Long.class),
    SUPPORT_MZDY(false, Boolean.class, "supportMzdy"),      // payroll support
    SUPPORT_JDBC(true, Boolean.class),
    LISTEN_PUBLIC(false, Boolean.class, "listenPublic"),
    ACCESS_CONTROL_ALLOW_ORIGIN(true, String.class, "rest-api-rw"),
    PORT(false, Integer.class, "port"),
    ITH_USERS_MANAGEMENT(true, Boolean.class),
    WEB_THEME(true, String.class),
    CLOUD_OVERRIDE_DIR(true, String.class),
    SHOW_REGISTRATION_EMAIL_PROMPT(true, Boolean.class);    // default: true
}
```

### UserInfo

```java
@XmlRootElement
public class UserInfo implements Comparable<UserInfo>, Serializable {
    private int id;
    private String uuid;
    private String username;
    private String dbUsername;       // PostgreSQL role name
    private String givenName;
    private String familyName;
    private String email;
    private String mobile;
    private String ssoIdentifier;
    private String defaultRole;
    private Boolean overrideRole;
    private BlockedState blockedState;  // { blocked, message }
    private Permissions permissions;    // see below
    private Boolean systemUser;
    private NamedUserType userType;     // AUTOMATIC, READ_ONLY, etc.
    private Date createDt;
    private Date lastDt;
    private OpaqueId licenseGroup;
    private Boolean twoPhaseAuthEnabled;
    private Boolean resetTokenFilled;
    private Date lastLoginDate;
    private Date lastApiDate;
    private Boolean apiAccessEnabled;
    private LicenseGroupInfo licenseGroupInfo;
    private UserCompanyAccessList accessList;

    public static class Permissions implements Serializable {
        Boolean manageAll;         // super-admin: access to everything
        Boolean createCompany;
        Boolean deleteCompany;
        Boolean createUser;
        Boolean changePassword;
        Boolean grantPermission;
        Boolean licenseMgmt;      // can manage licenses
    }

    public static class BlockedState implements Serializable {
        Boolean blocked;
        String message;
    }
}
```

### LicenseData

```java
public class LicenseData implements Serializable {
    public static final String LICENSE = "license";
    private int id;
    private byte[] license;   // raw license binary blob
}
```

### ServerInfo (vo)

Note: There are TWO `ServerInfo` classes:
- `cz.winstrom.vo.ServerInfo` — returned by `getServerInfo()` (contains version, schema info, cloud flag)
- `cz.winstrom.client.ServerInfo` — used for server discovery (address, port, hostname)

```java
// cz.winstrom.client.ServerInfo
public class ServerInfo implements Serializable {
    private String version;
    private String hostname;
    private InetAddress address;
    private int port;
}
```

---

## Server-Side Handler

### MiniRMIRegistryServlet

```java
package cz.winstrom.net.server.rmi;

public class MiniRMIRegistryServlet extends ServletRegistry {

    @Override
    public void init(ServletConfig config) throws ServletException {
        super.init(config);
        // Binds WinStromConnector to the registry
        this.rebind("WinStromConnector", WinStromConnectorImpl.getInstance());
        // Also binds BrotherDiscovery if Cloud.isCloud()
    }

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse res) {
        // Rejects BrotherDiscovery calls from untrusted sources
        // Delegates to ServletRegistry.doPost()
    }

    private boolean isTrustedClusterPeer(HttpServletRequest req) {
        // Checks: BrotherDiscoveryService.isBrotherAddress() || isLoopback
    }
}
```

### ServletRegistry (the core RMI dispatcher)

```java
package cz.winstrom.net.rmi.registry;

public class ServletRegistry extends HttpServlet implements Registry {
    protected final Map<String, Remote> registry;

    // Security: blocks deserialization of ServerAuthenticator
    private static final ObjectInputFilter MINI_RMI_DESERIALIZATION_FILTER = info -> {
        if (ServerAuthenticator.class.isAssignableFrom(info.serialClass())) {
            return REJECTED;  // prevents privilege escalation
        }
        return UNDECIDED;
    };

    protected void doPost(HttpServletRequest req, HttpServletResponse res) {
        // 1. Parse facade name from URL path (/mini-rmi/{facadeName}/...)
        // 2. Deserialize MethodCall from request body
        //    - Apply deserialization filter (block ServerAuthenticator)
        //    - Block ServerAuthenticator in method parameters too
        // 3. Lookup the bound Remote object
        // 4. Resolve parameter classes by name
        // 5. Find and invoke the method via reflection
        // 6. Wrap result in Response(returnValue, exception)
        // 7. Serialize Response via MarshalOutputStream
        //    Content-Type: application/winstrom-rmi
    }
}
```

---

## Key Findings

### Security

1. **ServerAuthenticator blocked** — The server explicitly rejects deserialization of
   `ServerAuthenticator` both in the ObjectInputStream filter and in parameter checking.
   This prevents clients from impersonating server-to-server authentication.

2. **BrotherDiscovery restricted** — Cluster discovery calls are only accepted from
   loopback or known cluster peers.

3. **No CSRF on RMI** — The mini-RMI endpoint uses Java serialization, not browser forms,
   so CSRF protection relies on the binary protocol being hard to forge from a browser.

4. **Trust-all SSL** — The client uses `MySSLProtocolSocketFactory` which likely accepts
   all certificates (common in desktop apps connecting to self-hosted servers).

### Protocol Details

- **Wire format:** Java `ObjectOutputStream` serialization, not JSON/XML
- **Transport:** HTTPS POST to `/mini-rmi/WinStromConnector/?methodName`
- **Content-Type:** `application/winstrom-rmi`
- **Auth header:** `X-authSessionId` (session ID from `authentificateUser`)
- **Session keepalive:** Every 5 minutes (`AUTH_SESSION_KEEPER_INTERVAL = 300000`)

### License Enforcement

- License warranty is checked on nearly every call (`checkServerState`)
- Named-user licensing: `acquireSession` marks other sessions obsolete
- Connection types: `RW`, `RO` (not `ALL`)
- Cloud restrictions: cannot change license in cloud, cannot delete non-existent DBs

### Interesting Implementation Details

- `verifyPrevPasswordForChange` always returns `true` — no actual verification
- `releaseSession` only authenticates, doesn't actually release anything server-side
- `deleteDatabase` always returns `false` regardless of success
- `getConnection` replaces the host with `$$centralServer$$:{port}` placeholder
- The impl is wrapped in TWO cglib proxies: `CurrentCompanySettingProxy` (sets
  company context from `@SetCurrentCompany` annotated method args) and
  `CurrentUserSettingProxy` (sets current user from any `Authenticator` arg)
- Exception handler converts `JdbcException` → `WSServerRTException`, ignores others
