// Package flexi – transport_java.go implements the native Java client transport.
//
// This transport spawns a Java subprocess that uses Java's HTTP stack (with
// the WinStrom/DESKTOP User-Agent) to communicate with FlexiBee.  It mimics
// how the native ABRA Flexi desktop client makes HTTP requests, including:
//   - Java's SSL/TLS implementation and truststore handling
//   - Proper HTTP keep-alive and connection pooling via Java's HttpURLConnection
//   - Session auth identical to the SessionTransport but executed in Java
//   - User-Agent: WinStrom/2026.6.2 (DESKTOP) matching the native app
//
// The Java bridge source is compiled on first use using javac.  A cached .class
// file avoids recompilation on subsequent starts.
//
// Requirements:
//   - Java 11+ (FLEXIBEE_JAVA_HOME or found on PATH / well-known locations)
//   - javac must be available (part of the JDK, not just JRE)
//
// If Java is unavailable, NewJavaTransport returns an error and AutoTransport
// silently skips this tier.
package flexi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
)

// JavaTransport is a Transport that delegates HTTP calls to a Java subprocess.
type JavaTransport struct {
	cfg      *config.Config
	javaExe  string // path to java binary
	javacExe string // path to javac binary
	classDir string // directory where compiled .class file lives

	mu   sync.Mutex
	proc *javaProcess // running subprocess (nil if not started)
}

// javaProcess wraps a running Java bridge subprocess.
type javaProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *json.Decoder
	mu     sync.Mutex
}

// javaBridgeCommand is the JSON command sent to the Java subprocess.
type javaBridgeCommand struct {
	Action  string `json:"action"`            // init|get|post|put|delete
	URL     string `json:"url,omitempty"`     // base URL (init only)
	User    string `json:"user,omitempty"`    // username (init only)
	Pass    string `json:"pass,omitempty"`    // password (init only)
	SkipTLS bool   `json:"skipTls,omitempty"` // skip TLS verify (init only)
	Path    string `json:"path,omitempty"`    // request path
	Body    string `json:"body,omitempty"`    // request body (JSON string)
}

// javaBridgeResponse is the JSON response from the Java subprocess.
type javaBridgeResponse struct {
	Status int    `json:"status"` // HTTP status code
	Body   string `json:"body"`   // response body
	Error  string `json:"error"`  // error message (non-empty on failure)
}

// NewJavaTransport builds a JavaTransport.  Returns an error if Java is
// unavailable on this system, allowing AutoTransport to skip this tier.
func NewJavaTransport(cfg *config.Config) (*JavaTransport, error) {
	javaExe, javacExe, err := findJava(cfg.JavaHome)
	if err != nil {
		return nil, fmt.Errorf("java transport: %w", err)
	}

	// Use a stable temp directory for the compiled bridge class.
	classDir := filepath.Join(os.TempDir(), "flexibee-mcp-java")

	return &JavaTransport{
		cfg:      cfg,
		javaExe:  javaExe,
		javacExe: javacExe,
		classDir: classDir,
	}, nil
}

// Name implements Transport.
func (t *JavaTransport) Name() string { return "java" }

// Get implements Transport.
func (t *JavaTransport) Get(ctx context.Context, path string) ([]byte, error) {
	return t.call(ctx, "get", path, nil)
}

// Post implements Transport.
func (t *JavaTransport) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.call(ctx, "post", path, body)
}

// Put implements Transport.
func (t *JavaTransport) Put(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.call(ctx, "put", path, body)
}

// Delete implements Transport.
func (t *JavaTransport) Delete(ctx context.Context, path string) ([]byte, error) {
	return t.call(ctx, "delete", path, nil)
}

// ── subprocess management ─────────────────────────────────────────────────────

// call ensures the Java subprocess is running, then sends one request.
func (t *JavaTransport) call(ctx context.Context, action, path string, body []byte) ([]byte, error) {
	proc, err := t.ensureProcess(ctx)
	if err != nil {
		return nil, err
	}

	cmd := javaBridgeCommand{
		Action: action,
		Path:   ensureV2Prefix(path),
	}
	if len(body) > 0 {
		cmd.Body = string(body)
	}

	resp, err := proc.send(ctx, cmd)
	if err != nil {
		// Subprocess died – reset so next call spawns a fresh one.
		t.resetProcess()
		return nil, NewTransportError("java transport subprocess: "+err.Error(), err)
	}

	if resp.Error != "" {
		return nil, NewTransportError("java bridge: "+resp.Error, nil)
	}
	if resp.Status == 402 {
		return nil, &FlexiError{Kind: ErrPaymentRequired,
			Message: fmt.Sprintf("java transport: HTTP 402: %s", resp.Body)}
	}
	if resp.Status == 401 {
		return nil, &FlexiError{Kind: ErrAuth,
			Message: fmt.Sprintf("java transport: HTTP 401: %s", resp.Body)}
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, NewHTTPError(resp.Status, []byte(resp.Body))
	}
	return []byte(resp.Body), nil
}

// ensureProcess starts the Java subprocess if it is not already running.
func (t *JavaTransport) ensureProcess(ctx context.Context) (*javaProcess, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.proc != nil {
		return t.proc, nil
	}

	if err := t.ensureBridge(); err != nil {
		return nil, err
	}

	proc, err := t.startProcess(ctx)
	if err != nil {
		return nil, err
	}
	t.proc = proc
	return proc, nil
}

// resetProcess clears the subprocess reference so the next call restarts it.
func (t *JavaTransport) resetProcess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.proc != nil {
		_ = t.proc.cmd.Process.Kill()
	}
	t.proc = nil
}

// startProcess spawns the Java bridge subprocess and sends the init command.
func (t *JavaTransport) startProcess(ctx context.Context) (*javaProcess, error) {
	args := []string{
		"-cp", t.classDir,
		"FlexiBeeJavaBridge",
	}

	cmd := exec.CommandContext(ctx, t.javaExe, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("java subprocess stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("java subprocess stdout pipe: %w", err)
	}
	// Discard stderr from the Java process to avoid cluttering MCP output.
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting java subprocess: %w", err)
	}

	proc := &javaProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: json.NewDecoder(stdoutPipe),
	}

	// Send init command.
	initCmd := javaBridgeCommand{
		Action:  "init",
		URL:     t.cfg.BaseURL,
		User:    t.cfg.Username,
		Pass:    t.cfg.Password,
		SkipTLS: t.cfg.SkipTLSVerify,
	}
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := proc.send(initCtx, initCmd)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("java bridge init: %w", err)
	}
	if resp.Error != "" {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("java bridge init error: %s", resp.Error)
	}
	return proc, nil
}

// send sends one command to the subprocess and reads one response.
func (p *javaProcess) send(ctx context.Context, cmd javaBridgeCommand) (*javaBridgeResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	b, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}
	b = append(b, '\n')

	if _, err := p.stdin.Write(b); err != nil {
		return nil, fmt.Errorf("write to java subprocess: %w", err)
	}

	var resp javaBridgeResponse
	// Set a deadline via a goroutine-based approach since json.Decoder doesn't
	// support context cancellation directly.
	type result struct {
		resp javaBridgeResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		var r javaBridgeResponse
		if err := p.stdout.Decode(&r); err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{resp: r}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		resp = res.resp
	}

	return &resp, nil
}

// ── Bridge compilation ────────────────────────────────────────────────────────

// ensureBridge writes the Java source to classDir and compiles it if the
// .class file does not already exist.
func (t *JavaTransport) ensureBridge() error {
	if err := os.MkdirAll(t.classDir, 0o700); err != nil {
		return fmt.Errorf("creating bridge class dir: %w", err)
	}

	classFile := filepath.Join(t.classDir, "FlexiBeeJavaBridge.class")
	if _, err := os.Stat(classFile); err == nil {
		return nil // already compiled
	}

	srcFile := filepath.Join(t.classDir, "FlexiBeeJavaBridge.java")
	if err := os.WriteFile(srcFile, []byte(flexiBeeJavaBridgeSource), 0o600); err != nil {
		return fmt.Errorf("writing bridge source: %w", err)
	}

	// Compile with javac from the JDK.
	out, err := exec.Command(t.javacExe, "-source", "11", "-target", "11",
		"-d", t.classDir, srcFile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("compiling java bridge: %w\n%s", err, out)
	}
	return nil
}

// ── Java binary discovery ─────────────────────────────────────────────────────

// findJava locates the java and javac binaries.
// javaHome overrides all other discovery when non-empty.
func findJava(javaHome string) (javaExe, javacExe string, err error) {
	candidates := javaCandidates(javaHome)

	for _, dir := range candidates {
		java := filepath.Join(dir, "bin", "java")
		javac := filepath.Join(dir, "bin", "javac")
		if executableExists(java) && executableExists(javac) {
			// Verify Java version ≥ 11 by running java -version.
			if ok := checkJavaVersion(java, 11); ok {
				return java, javac, nil
			}
		}
	}

	// Last resort: PATH lookup.
	if j, e := exec.LookPath("java"); e == nil {
		if jc, e2 := exec.LookPath("javac"); e2 == nil {
			return j, jc, nil
		}
	}

	return "", "", fmt.Errorf("java 11+ and javac not found; set FLEXIBEE_JAVA_HOME or install a JDK")
}

// javaCandidates returns a list of candidate Java home directories to probe.
func javaCandidates(javaHome string) []string {
	if javaHome != "" {
		return []string{javaHome}
	}
	// Well-known JVM installation paths (Linux / macOS).
	return []string{
		"/usr/lib/jvm/temurin-21-jdk-amd64",
		"/usr/lib/jvm/java-21-openjdk-amd64",
		"/usr/lib/jvm/java-21-openjdk",
		"/usr/lib/jvm/java-17-openjdk-amd64",
		"/usr/lib/jvm/java-17-openjdk",
		"/usr/lib/jvm/java-11-openjdk-amd64",
		"/usr/lib/jvm/java-11-openjdk",
		"/usr/local/lib/jvm/java-21",
		"/usr/local/lib/jvm/java-11",
		"/Library/Java/JavaVirtualMachines/temurin-21.jdk/Contents/Home",
		"/Library/Java/JavaVirtualMachines/openjdk-21.jdk/Contents/Home",
		"/Library/Java/JavaVirtualMachines/openjdk-17.jdk/Contents/Home",
		"/Library/Java/JavaVirtualMachines/openjdk-11.jdk/Contents/Home",
	}
}

// executableExists reports whether the given path exists and is executable.
func executableExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// checkJavaVersion returns true if the java binary at path reports a version
// number ≥ minMajor.
func checkJavaVersion(javaPath string, minMajor int) bool {
	out, err := exec.Command(javaPath, "-version").CombinedOutput()
	if err != nil {
		return false
	}
	// "version" line looks like: openjdk version "21.0.12.1" ...
	s := string(out)
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "version") {
			// Extract the major version number.
			idx := strings.Index(line, "\"")
			if idx < 0 {
				continue
			}
			rest := line[idx+1:]
			// Find the first dot or quote.
			end := strings.IndexAny(rest, ".\"")
			if end < 0 {
				continue
			}
			major := 0
			for _, c := range rest[:end] {
				if c >= '0' && c <= '9' {
					major = major*10 + int(c-'0')
				}
			}
			return major >= minMajor
		}
	}
	return false
}

// ── Embedded Java bridge source ───────────────────────────────────────────────

// flexiBeeJavaBridgeSource is the Java source for the bridge subprocess.
// It is compiled on first use via javac.
// This class uses only java.* standard library classes (no external JARs needed)
// and mimics the native ABRA Flexi desktop client's HTTP behaviour.
const flexiBeeJavaBridgeSource = `
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.*;
import javax.net.ssl.*;
import java.security.*;
import java.security.cert.*;

/**
 * FlexiBeeJavaBridge – native Java HTTP transport for flexibee-mcp.
 *
 * Mimics the ABRA Flexi desktop client by:
 *   - Using User-Agent: WinStrom/2026.6.2 (DESKTOP)
 *   - Establishing session via POST /v2/login-logout/login
 *   - Sending authSessionId + CSRF-Token cookies on every request
 *   - Passing CSRF token as ?token=<csrf> for write operations
 *
 * Protocol: one JSON command per stdin line, one JSON response per stdout line.
 * Commands: {"action":"init","url":"...","user":"...","pass":"...","skipTls":false}
 *           {"action":"get","path":"/v2/c/..."}
 *           {"action":"post","path":"/v2/c/...","body":"{...}"}
 *           {"action":"put","path":"/v2/c/...","body":"{...}"}
 *           {"action":"delete","path":"/v2/c/..."}
 * Responses: {"status":200,"body":"..."}  or  {"status":0,"error":"..."}
 */
public class FlexiBeeJavaBridge {
    static final String USER_AGENT = "WinStrom/2026.6.2 (DESKTOP)";

    static String baseURL;
    static String authSessionId = "";
    static String csrfToken = "";
    static boolean skipTLSVerify = false;

    public static void main(String[] args) throws Exception {
        BufferedReader reader = new BufferedReader(new InputStreamReader(System.in));
        PrintWriter writer = new PrintWriter(new OutputStreamWriter(System.out), true);
        // Suppress all output to stderr from Java internals
        System.setErr(new PrintStream(OutputStream.nullOutputStream()));

        String line;
        while ((line = reader.readLine()) != null) {
            line = line.trim();
            if (line.isEmpty()) continue;
            try {
                String response = handleCommand(line);
                writer.println(response);
            } catch (Exception e) {
                writer.println("{\"status\":0,\"error\":\"" + escapeJson(e.getMessage()) + "\"}");
            }
        }
    }

    static String handleCommand(String json) throws Exception {
        String action = extractString(json, "action");

        if ("init".equals(action)) {
            baseURL = stripTrailingSlash(extractString(json, "url"));
            String user = extractString(json, "user");
            String pass = extractString(json, "pass");
            skipTLSVerify = json.contains("\"skipTls\":true");
            doLogin(user, pass);
            return "{\"status\":200,\"body\":\"ok\"}";
        }

        if ("get".equals(action)) {
            return doRequest("GET", extractString(json, "path"), null);
        }
        if ("post".equals(action)) {
            return doRequest("POST", extractString(json, "path"), extractString(json, "body"));
        }
        if ("put".equals(action)) {
            return doRequest("PUT", extractString(json, "path"), extractString(json, "body"));
        }
        if ("delete".equals(action)) {
            return doRequest("DELETE", extractString(json, "path"), null);
        }

        return "{\"status\":0,\"error\":\"unknown action: " + escapeJson(action) + "\"}";
    }

    // ── Login ────────────────────────────────────────────────────────────────

    static void doLogin(String user, String pass) throws Exception {
        String loginURL = baseURL + "/v2/login-logout/login";
        String formBody = "username=" + urlEncode(user)
                + "&password=" + urlEncode(pass)
                + "&licenseChecked=false";

        HttpURLConnection conn = openConnection(loginURL);
        conn.setRequestMethod("POST");
        conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded");
        conn.setRequestProperty("Accept", "application/json");
        conn.setRequestProperty("User-Agent", USER_AGENT);
        conn.setDoOutput(true);

        try (OutputStream os = conn.getOutputStream()) {
            os.write(formBody.getBytes(StandardCharsets.UTF_8));
        }

        int status = conn.getResponseCode();
        String body = readStream(status >= 200 && status < 300
                ? conn.getInputStream() : conn.getErrorStream());

        if (status != 200) {
            throw new Exception("Login failed: HTTP " + status + " " + body);
        }

        // Parse authSessionId and csrfToken from JSON response.
        authSessionId = extractString(body, "authSessionId");
        csrfToken = extractString(body, "csrfToken");

        if (authSessionId == null || authSessionId.isEmpty()) {
            throw new Exception("Login failed: empty authSessionId in response");
        }
    }

    // ── HTTP request ─────────────────────────────────────────────────────────

    static String doRequest(String method, String path, String body) throws Exception {
        // Re-login if session was lost
        if (authSessionId.isEmpty()) {
            return "{\"status\":401,\"body\":\"\",\"error\":\"no active session\"}";
        }

        // Append CSRF token for write methods
        String fullPath = path;
        if (isWriteMethod(method) && !csrfToken.isEmpty()) {
            fullPath += (fullPath.contains("?") ? "&" : "?")
                    + "token=" + urlEncode(csrfToken);
        }

        String reqURL = baseURL + fullPath;
        HttpURLConnection conn = openConnection(reqURL);
        conn.setRequestMethod(method);
        conn.setRequestProperty("Accept", "application/json");
        conn.setRequestProperty("User-Agent", USER_AGENT);
        conn.setRequestProperty("Cookie", buildCookieHeader());
        if (body != null && !body.isEmpty()) {
            conn.setRequestProperty("Content-Type", "application/json");
            conn.setDoOutput(true);
        }

        if (body != null && !body.isEmpty()) {
            try (OutputStream os = conn.getOutputStream()) {
                os.write(body.getBytes(StandardCharsets.UTF_8));
            }
        }

        int status = conn.getResponseCode();
        InputStream is = status >= 200 && status < 300
                ? conn.getInputStream() : conn.getErrorStream();
        String responseBody = is != null ? readStream(is) : "";

        return "{\"status\":" + status + ",\"body\":" + jsonString(responseBody) + "}";
    }

    // ── Connection factory ────────────────────────────────────────────────────

    static HttpURLConnection openConnection(String url) throws Exception {
        URL u = new URL(url);
        HttpURLConnection conn = (HttpURLConnection) u.openConnection();
        conn.setConnectTimeout(30_000);
        conn.setReadTimeout(30_000);
        conn.setInstanceFollowRedirects(false);

        if (skipTLSVerify && conn instanceof HttpsURLConnection) {
            HttpsURLConnection https = (HttpsURLConnection) conn;
            https.setSSLSocketFactory(trustAllSSLSocketFactory());
            https.setHostnameVerifier((hostname, session) -> true);
        }
        return conn;
    }

    static SSLSocketFactory trustAllSSLSocketFactory() throws Exception {
        TrustManager[] trustAll = new TrustManager[]{
            new X509TrustManager() {
                public X509Certificate[] getAcceptedIssuers() { return new X509Certificate[0]; }
                public void checkClientTrusted(X509Certificate[] c, String a) {}
                public void checkServerTrusted(X509Certificate[] c, String a) {}
            }
        };
        SSLContext ctx = SSLContext.getInstance("TLS");
        ctx.init(null, trustAll, new SecureRandom());
        return ctx.getSocketFactory();
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    static String buildCookieHeader() {
        if (csrfToken.isEmpty()) {
            return "authSessionId=" + authSessionId;
        }
        return "authSessionId=" + authSessionId + "; CSRF-Token=" + csrfToken;
    }

    static boolean isWriteMethod(String method) {
        return "POST".equals(method) || "PUT".equals(method)
                || "DELETE".equals(method) || "PATCH".equals(method);
    }

    static String readStream(InputStream is) throws IOException {
        if (is == null) return "";
        ByteArrayOutputStream buf = new ByteArrayOutputStream();
        byte[] tmp = new byte[8192];
        int n;
        while ((n = is.read(tmp)) != -1) buf.write(tmp, 0, n);
        return buf.toString(StandardCharsets.UTF_8.name());
    }

    static String urlEncode(String s) {
        try { return URLEncoder.encode(s, "UTF-8"); }
        catch (Exception e) { return s; }
    }

    static String stripTrailingSlash(String s) {
        while (s != null && s.endsWith("/")) s = s.substring(0, s.length() - 1);
        return s;
    }

    /**
     * Minimal JSON string extraction – finds the first occurrence of
     * "key":"value" (or "key":null) and returns the value string or null.
     * This avoids a dependency on a JSON library.
     */
    static String extractString(String json, String key) {
        if (json == null) return "";
        String search = "\"" + key + "\":";
        int pos = json.indexOf(search);
        if (pos < 0) return "";
        int start = pos + search.length();
        // Skip whitespace
        while (start < json.length() && json.charAt(start) == ' ') start++;
        if (start >= json.length()) return "";
        char first = json.charAt(start);
        if (first == '"') {
            // String value
            StringBuilder sb = new StringBuilder();
            int i = start + 1;
            while (i < json.length() && json.charAt(i) != '"') {
                if (json.charAt(i) == '\\' && i + 1 < json.length()) {
                    char next = json.charAt(i + 1);
                    switch (next) {
                        case '"': sb.append('"'); break;
                        case '\\': sb.append('\\'); break;
                        case 'n': sb.append('\n'); break;
                        case 'r': sb.append('\r'); break;
                        case 't': sb.append('\t'); break;
                        default: sb.append(next);
                    }
                    i += 2;
                } else {
                    sb.append(json.charAt(i));
                    i++;
                }
            }
            return sb.toString();
        }
        if (first == 'n' && json.startsWith("null", start)) return "";
        // Non-string (number, bool) – not needed
        return "";
    }

    /** Encodes a string as a JSON string literal (including surrounding quotes). */
    static String jsonString(String s) {
        if (s == null) return "null";
        StringBuilder sb = new StringBuilder("\"");
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"': sb.append("\\\""); break;
                case '\\': sb.append("\\\\"); break;
                case '\n': sb.append("\\n"); break;
                case '\r': sb.append("\\r"); break;
                case '\t': sb.append("\\t"); break;
                default:
                    if (c < 0x20) {
                        sb.append(String.format("\\u%04x", (int) c));
                    } else {
                        sb.append(c);
                    }
            }
        }
        sb.append('"');
        return sb.toString();
    }

    /** Escapes a string for embedding in a JSON string value (no surrounding quotes). */
    static String escapeJson(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"")
                .replace("\n", "\\n").replace("\r", "\\r").replace("\t", "\\t");
    }
}
`
