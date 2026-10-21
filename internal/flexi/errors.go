// Package flexi – error taxonomy and constructors for the FlexiBee REST client.
package flexi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ErrorKind classifies the origin or category of a FlexiBee client error.
// Use a type switch or errors.As to inspect it from a returned error value.
type ErrorKind int

const (
	ErrConfig          ErrorKind = iota // bad or missing configuration
	ErrTransport                        // network error, timeout, or TLS failure
	ErrAuth                             // HTTP 401 – authentication required
	ErrPaymentRequired                  // HTTP 402 – API licence required (triggers transport fallback)
	ErrPermission                       // HTTP 403 – access denied
	ErrNotFound                         // HTTP 404 – resource does not exist
	ErrValidation                       // HTTP 400 – request rejected by the server
	ErrUnsupported                      // HTTP 406 – unsupported format or report type
	ErrConflict                         // HTTP 409 – concurrent modification conflict
	ErrMaintenance                      // HTTP 503 – server under maintenance
	ErrApplication                      // HTTP 200 body carries @globalError / @rowError
	ErrHTMLResponse                     // HTML page received instead of JSON (login redirect / proxy wall)
)

// String returns a short, stable, human-readable label for the ErrorKind.
// These labels are suitable for log fields and error message prefixes.
func (k ErrorKind) String() string {
	switch k {
	case ErrConfig:
		return "config"
	case ErrTransport:
		return "transport"
	case ErrAuth:
		return "auth"
	case ErrPaymentRequired:
		return "payment_required"
	case ErrPermission:
		return "permission"
	case ErrNotFound:
		return "not_found"
	case ErrValidation:
		return "validation"
	case ErrUnsupported:
		return "unsupported"
	case ErrConflict:
		return "conflict"
	case ErrMaintenance:
		return "maintenance"
	case ErrApplication:
		return "application"
	case ErrHTMLResponse:
		return "html_response"
	default:
		return fmt.Sprintf("error_kind(%d)", int(k))
	}
}

// FlexiError is the single error type returned by all flexi package operations.
//
// Use errors.As to obtain the concrete *FlexiError and inspect Kind, Code, and
// Details.  The Cause chain is compatible with errors.Is / errors.Unwrap.
type FlexiError struct {
	Kind    ErrorKind
	Message string
	// Cause is the underlying error, if any (e.g. a net.Error on transport
	// failures).  Exposed via Unwrap for errors.Is / errors.As traversal.
	Cause error
	// Code contains the FlexiBee application error code when the server
	// provides one (e.g. a winstrom @globalErrorCode value).
	Code string
	// Details holds per-row error strings extracted from
	// winstrom.results[].@rowError on write operation responses.
	Details []string
}

// Error implements the error interface.  The format is:
//
//	flexibee <kind>: <message>
//	flexibee <kind> [<code>]: <message>
//	flexibee <kind>: <message>: <cause>
func (e *FlexiError) Error() string {
	switch {
	case e.Cause != nil && e.Code != "":
		return fmt.Sprintf("flexibee %s [%s]: %s: %v", e.Kind, e.Code, e.Message, e.Cause)
	case e.Cause != nil:
		return fmt.Sprintf("flexibee %s: %s: %v", e.Kind, e.Message, e.Cause)
	case e.Code != "":
		return fmt.Sprintf("flexibee %s [%s]: %s", e.Kind, e.Code, e.Message)
	default:
		return fmt.Sprintf("flexibee %s: %s", e.Kind, e.Message)
	}
}

// Unwrap implements the errors.Unwrap interface, enabling errors.Is /
// errors.As traversal through the Cause chain.
func (e *FlexiError) Unwrap() error {
	return e.Cause
}

// ── Constructors ──────────────────────────────────────────────────────────────

// NewConfigError returns a FlexiError of kind ErrConfig for misconfiguration
// issues detected before any network operation is attempted.
func NewConfigError(msg string) *FlexiError {
	return &FlexiError{Kind: ErrConfig, Message: msg}
}

// NewTransportError wraps a network-level or TLS error.  If msg is empty the
// cause's own message is used as the FlexiError message.
func NewTransportError(msg string, cause error) *FlexiError {
	if msg == "" && cause != nil {
		msg = cause.Error()
	}
	return &FlexiError{Kind: ErrTransport, Message: msg, Cause: cause}
}

// NewAuthError returns a FlexiError of kind ErrAuth.  If msg is empty a
// generic "authentication required" message is used.
func NewAuthError(msg string) *FlexiError {
	if msg == "" {
		msg = "authentication required"
	}
	return &FlexiError{Kind: ErrAuth, Message: msg}
}

// NewPermissionError returns a FlexiError of kind ErrPermission.  If msg is
// empty a generic "access denied" message is used.
func NewPermissionError(msg string) *FlexiError {
	if msg == "" {
		msg = "access denied"
	}
	return &FlexiError{Kind: ErrPermission, Message: msg}
}

// NewNotFoundError returns a FlexiError of kind ErrNotFound.  If msg is empty
// a generic "resource not found" message is used.
func NewNotFoundError(msg string) *FlexiError {
	if msg == "" {
		msg = "resource not found"
	}
	return &FlexiError{Kind: ErrNotFound, Message: msg}
}

// NewAppError constructs a FlexiError of kind ErrApplication from an
// application-level message (typically winstrom.@globalError) and an optional
// slice of per-row error strings (winstrom.results[].@rowError).
func NewAppError(msg string, details []string) *FlexiError {
	return &FlexiError{Kind: ErrApplication, Message: msg, Details: details}
}

// NewHTTPError constructs a *FlexiError from an HTTP status code and the raw
// response body.
//
// Detection order:
//  1. If body contains "<html" (case-insensitive) → ErrHTMLResponse.
//  2. Attempt to extract winstrom.@globalError from the JSON body for the
//     error message.
//  3. Fall back to the raw body text, or net/http.StatusText when empty.
//  4. Map the status code to the appropriate ErrorKind.
func NewHTTPError(statusCode int, body []byte) *FlexiError {
	bodyStr := strings.TrimSpace(string(body))

	// ── 1. Detect HTML ────────────────────────────────────────────────────────
	if strings.Contains(strings.ToLower(bodyStr), "<html") {
		return &FlexiError{
			Kind:    ErrHTMLResponse,
			Message: "HTML response received instead of JSON; check the base URL and authentication settings",
		}
	}

	// ── 2. Extract message from FlexiBee JSON error body ──────────────────────
	msg := extractJSONMessage(body)

	// ── 3. Fall back to raw body text or HTTP status text ─────────────────────
	if msg == "" {
		if bodyStr != "" {
			msg = bodyStr
		} else {
			msg = http.StatusText(statusCode)
		}
	}

	// ── 4. Map HTTP status code to ErrorKind ──────────────────────────────────
	switch statusCode {
	case http.StatusUnauthorized: // 401
		return &FlexiError{Kind: ErrAuth, Message: msg}
	case http.StatusPaymentRequired: // 402 – API licence required
		return &FlexiError{Kind: ErrPaymentRequired, Message: msg}
	case http.StatusForbidden: // 403
		return &FlexiError{Kind: ErrPermission, Message: msg}
	case http.StatusNotFound: // 404
		return &FlexiError{Kind: ErrNotFound, Message: msg}
	case http.StatusBadRequest: // 400
		return &FlexiError{Kind: ErrValidation, Message: msg}
	case http.StatusNotAcceptable: // 406
		return &FlexiError{Kind: ErrUnsupported, Message: msg}
	case http.StatusConflict: // 409
		return &FlexiError{Kind: ErrConflict, Message: msg}
	case http.StatusServiceUnavailable: // 503
		return &FlexiError{Kind: ErrMaintenance, Message: msg}
	default:
		return &FlexiError{
			Kind:    ErrTransport,
			Message: fmt.Sprintf("HTTP %d: %s", statusCode, msg),
		}
	}
}

// ── Application-level error parsing ──────────────────────────────────────────

// winstromErrShape is a minimal struct used only for extracting error fields
// from a winstrom JSON body received with HTTP 200.
type winstromErrShape struct {
	Winstrom struct {
		GlobalError string            `json:"@globalError"`
		Results     []json.RawMessage `json:"results"`
	} `json:"winstrom"`
}

// rowErrShape extracts @rowError from a single result entry.
type rowErrShape struct {
	RowError string `json:"@rowError"`
}

// ParseAppErrors inspects a winstrom response body (delivered with HTTP 200)
// for application-level errors encoded in @globalError or per-record @rowError
// fields.  It returns nil when neither kind of error is present.
//
// FlexiBee can indicate failure inside an otherwise successful HTTP response:
//
//	{"winstrom": {"@globalError": "Neplatný dotaz."}}
//
//	{"winstrom": {"results": [{"@rowError": "Pole je povinné.", "id": "0"}]}}
//
// When both a @globalError and @rowError entries are present, @globalError is
// used as the primary FlexiError.Message and the row-level strings are
// collected into FlexiError.Details.
func ParseAppErrors(body []byte) *FlexiError {
	var env winstromErrShape
	if err := json.Unmarshal(body, &env); err != nil {
		// Body is not parseable as winstrom JSON; no application error.
		return nil
	}

	globalErr := strings.TrimSpace(env.Winstrom.GlobalError)

	var rowErrors []string
	for _, raw := range env.Winstrom.Results {
		var row rowErrShape
		if err := json.Unmarshal(raw, &row); err != nil {
			continue
		}
		if s := strings.TrimSpace(row.RowError); s != "" {
			rowErrors = append(rowErrors, s)
		}
	}

	if globalErr == "" && len(rowErrors) == 0 {
		return nil
	}

	// Primary message: @globalError takes precedence; fall back to first row
	// error when no global error is present.
	msg := globalErr
	if msg == "" {
		msg = rowErrors[0]
	}

	return &FlexiError{
		Kind:    ErrApplication,
		Message: msg,
		Details: rowErrors,
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// extractJSONMessage attempts to pull a human-readable error string from a
// FlexiBee JSON error body by checking winstrom.@globalError.  Returns an
// empty string on any parse failure or when the field is absent.
func extractJSONMessage(body []byte) string {
	var env struct {
		Winstrom struct {
			GlobalError string `json:"@globalError"`
		} `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return strings.TrimSpace(env.Winstrom.GlobalError)
}
