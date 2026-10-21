package flexi

import (
	"errors"
	"strings"
	"testing"
)

// ── NewHTTPError – status-code mapping ───────────────────────────────────────

func TestNewHTTPErrorAuth(t *testing.T) {
	err := NewHTTPError(401, []byte("unauthorized"))
	if err.Kind != ErrAuth {
		t.Errorf("kind = %v, want ErrAuth", err.Kind)
	}
}

func TestNewHTTPErrorPaymentRequired(t *testing.T) {
	err := NewHTTPError(402, []byte("payment required"))
	if err.Kind != ErrPaymentRequired {
		t.Errorf("kind = %v, want ErrPaymentRequired", err.Kind)
	}
}

func TestNewHTTPErrorPermission(t *testing.T) {
	err := NewHTTPError(403, []byte("forbidden"))
	if err.Kind != ErrPermission {
		t.Errorf("kind = %v, want ErrPermission", err.Kind)
	}
}

func TestNewHTTPErrorNotFound(t *testing.T) {
	err := NewHTTPError(404, []byte("not found"))
	if err.Kind != ErrNotFound {
		t.Errorf("kind = %v, want ErrNotFound", err.Kind)
	}
}

func TestNewHTTPErrorValidation(t *testing.T) {
	body := []byte(`{"winstrom":{"@globalError":"invalid field"}}`)
	err := NewHTTPError(400, body)
	if err.Kind != ErrValidation {
		t.Errorf("kind = %v, want ErrValidation", err.Kind)
	}
	if !strings.Contains(err.Message, "invalid field") {
		t.Errorf("message = %q; want to contain \"invalid field\"", err.Message)
	}
}

func TestNewHTTPErrorHTMLResponse(t *testing.T) {
	// HTML body must trigger ErrHTMLResponse regardless of status code.
	body := []byte("<html><body>Login</body></html>")
	err := NewHTTPError(200, body)
	if err.Kind != ErrHTMLResponse {
		t.Errorf("kind = %v, want ErrHTMLResponse", err.Kind)
	}
}

func TestNewHTTPErrorHTMLDetectionCaseInsensitive(t *testing.T) {
	// Detection is case-insensitive; upper-case <HTML> must also be detected.
	body := []byte("<HTML><BODY>Session expired</BODY></HTML>")
	err := NewHTTPError(200, body)
	if err.Kind != ErrHTMLResponse {
		t.Errorf("kind = %v, want ErrHTMLResponse for upper-case HTML body", err.Kind)
	}
}

func TestNewHTTPErrorHTMLBeatsStatus(t *testing.T) {
	// HTML detection happens before status-code mapping.
	// A 401 response with HTML body must be ErrHTMLResponse, not ErrAuth.
	body := []byte("<html>login page</html>")
	err := NewHTTPError(401, body)
	if err.Kind != ErrHTMLResponse {
		t.Errorf("kind = %v, want ErrHTMLResponse (HTML check must precede status mapping)", err.Kind)
	}
}

func TestNewHTTPErrorFallbackToRawBody(t *testing.T) {
	// When no JSON body is present, the raw body text becomes the message.
	err := NewHTTPError(400, []byte("bad request text"))
	if !strings.Contains(err.Message, "bad request text") {
		t.Errorf("message = %q; want raw body text as message", err.Message)
	}
}

// ── ParseAppErrors ────────────────────────────────────────────────────────────

func TestParseAppErrorsGlobalError(t *testing.T) {
	body := []byte(`{"winstrom":{"@globalError":"test error"}}`)
	err := ParseAppErrors(body)
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if err.Kind != ErrApplication {
		t.Errorf("kind = %v, want ErrApplication", err.Kind)
	}
	if !strings.Contains(err.Message, "test error") {
		t.Errorf("message = %q; want to contain \"test error\"", err.Message)
	}
}

func TestParseAppErrorsNoError(t *testing.T) {
	// A 200 body with only record data and no @globalError must return nil.
	body := []byte(`{"winstrom":{"some-evidence":[{"id":"1"}]}}`)
	err := ParseAppErrors(body)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestParseAppErrorsEmptyGlobalError(t *testing.T) {
	// An empty @globalError field (whitespace only) must also return nil.
	body := []byte(`{"winstrom":{"@globalError":"   "}}`)
	err := ParseAppErrors(body)
	if err != nil {
		t.Errorf("expected nil for whitespace-only globalError, got %v", err)
	}
}

func TestParseAppErrorsRowErrors(t *testing.T) {
	// @rowError entries without a @globalError must still produce a non-nil error.
	body := []byte(`{"winstrom":{"results":[{"@rowError":"row problem","id":"1"}]}}`)
	err := ParseAppErrors(body)
	if err == nil {
		t.Fatal("expected non-nil error from rowError, got nil")
	}
	if err.Kind != ErrApplication {
		t.Errorf("kind = %v, want ErrApplication", err.Kind)
	}
	if len(err.Details) == 0 {
		t.Error("expected Details to contain row error strings")
	}
}

func TestParseAppErrorsInvalidJSON(t *testing.T) {
	// Non-parseable body must return nil (treated as no application error).
	err := ParseAppErrors([]byte("not json"))
	if err != nil {
		t.Errorf("expected nil for unparseable body, got %v", err)
	}
}

// ── FlexiError.Error() ───────────────────────────────────────────────────────

func TestFlexiErrorMessageInOutput(t *testing.T) {
	e := &FlexiError{Kind: ErrAuth, Message: "authentication required"}
	s := e.Error()
	if !strings.Contains(s, "authentication required") {
		t.Errorf("Error() = %q; want to contain \"authentication required\"", s)
	}
	// Kind label "auth" must also appear.
	if !strings.Contains(s, "auth") {
		t.Errorf("Error() = %q; want to contain kind label \"auth\"", s)
	}
}

func TestFlexiErrorWithCode(t *testing.T) {
	e := &FlexiError{Kind: ErrValidation, Message: "bad value", Code: "E001"}
	s := e.Error()
	if !strings.Contains(s, "E001") {
		t.Errorf("Error() = %q; want to contain code \"E001\"", s)
	}
	if !strings.Contains(s, "bad value") {
		t.Errorf("Error() = %q; want to contain \"bad value\"", s)
	}
}

func TestFlexiErrorWithCause(t *testing.T) {
	cause := errors.New("network timeout")
	e := &FlexiError{Kind: ErrTransport, Message: "request failed", Cause: cause}
	s := e.Error()
	if !strings.Contains(s, "network timeout") {
		t.Errorf("Error() = %q; want to contain cause message \"network timeout\"", s)
	}
}

// ── errors.As compatibility ──────────────────────────────────────────────────

func TestErrorsAs(t *testing.T) {
	var target *FlexiError
	var err error = NewHTTPError(401, []byte("unauthorized"))
	if !errors.As(err, &target) {
		t.Fatal("errors.As failed to match *FlexiError")
	}
	if target.Kind != ErrAuth {
		t.Errorf("kind after As = %v, want ErrAuth", target.Kind)
	}
}

func TestErrorsAsWrapped(t *testing.T) {
	// errors.As must traverse the Cause chain.
	inner := &FlexiError{Kind: ErrAuth, Message: "inner"}
	wrapped := &FlexiError{Kind: ErrTransport, Message: "outer", Cause: inner}

	var target *FlexiError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed on wrapped FlexiError")
	}
	// The outermost match is found first.
	if target.Kind != ErrTransport {
		t.Errorf("first As match kind = %v, want ErrTransport", target.Kind)
	}
}

// ── ErrorKind.String() ───────────────────────────────────────────────────────

func TestErrorKindString(t *testing.T) {
	tests := []struct {
		kind ErrorKind
		want string
	}{
		{ErrConfig, "config"},
		{ErrTransport, "transport"},
		{ErrAuth, "auth"},
		{ErrPermission, "permission"},
		{ErrNotFound, "not_found"},
		{ErrValidation, "validation"},
		{ErrUnsupported, "unsupported"},
		{ErrConflict, "conflict"},
		{ErrMaintenance, "maintenance"},
		{ErrApplication, "application"},
		{ErrHTMLResponse, "html_response"},
	}
	for _, tc := range tests {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", int(tc.kind), got, tc.want)
		}
	}
}
