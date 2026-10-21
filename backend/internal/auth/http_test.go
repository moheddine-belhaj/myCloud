package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestMux(t *testing.T) (*http.ServeMux, *Service) {
	t.Helper()
	s, _ := newTestService(t)
	mux := http.NewServeMux()
	NewHandler(s).Routes(mux)
	return mux, s
}

func post(mux http.Handler, path, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func registerBody(code, email, pw string) string {
	b, _ := json.Marshal(registerRequest{InviteCode: code, Email: email, Password: pw})
	return string(b)
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&e); err != nil {
		t.Fatalf("decode error body: %v (body %q)", err, rec.Body.String())
	}
	return e.Error
}

func TestRegisterHTTP(t *testing.T) {
	mux, s := newTestMux(t)
	code := newInvite(t, s)

	rec := post(mux, "/api/auth/register", "application/json; charset=utf-8", registerBody(code, "Mo@Example.com", goodPassword))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	var got registerResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Email != "mo@example.com" || got.ID == "" {
		t.Errorf("body = %+v, want id and email mo@example.com", got)
	}
	if strings.Contains(rec.Body.String(), "argon2") {
		t.Error("response leaks the password hash")
	}

	// Same invite again: invalid. Fresh invite, same email: taken.
	rec = post(mux, "/api/auth/register", "application/json", registerBody(code, "x@example.com", goodPassword))
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_invite" {
		t.Errorf("reused invite: %d %s, want 400 invalid_invite", rec.Code, rec.Body)
	}
	rec = post(mux, "/api/auth/register", "application/json", registerBody(newInvite(t, s), "mo@example.com", goodPassword))
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "email_taken" {
		t.Errorf("taken email: %d %s, want 409 email_taken", rec.Code, rec.Body)
	}
}

func TestRegisterHTTPBadRequests(t *testing.T) {
	mux, s := newTestMux(t)
	code := newInvite(t, s)

	tests := []struct {
		name, contentType, body string
		status                  int
		code                    string
	}{
		{"no content type", "", registerBody(code, "mo@example.com", goodPassword), 415, "unsupported_media_type"},
		{"form post", "application/x-www-form-urlencoded", "email=mo@example.com", 415, "unsupported_media_type"},
		{"broken json", "application/json", `{"email":`, 400, "invalid_json"},
		{"unknown field", "application/json", `{"email":"mo@example.com","role":"admin"}`, 400, "invalid_json"},
		{"two objects", "application/json", `{} {}`, 400, "invalid_json"},
		{"too large", "application/json", `{"email":"` + strings.Repeat("a", maxBodyBytes) + `"}`, 413, "body_too_large"},
		{"bad email", "application/json", registerBody(code, "not-an-email", goodPassword), 400, "invalid_email"},
		{"short password", "application/json", registerBody(code, "mo@example.com", "short"), 400, "invalid_password"},
		{"missing invite", "application/json", registerBody("", "mo@example.com", goodPassword), 400, "invalid_invite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(mux, "/api/auth/register", tt.contentType, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.status, rec.Body)
			}
			if got := errorCode(t, rec); got != tt.code {
				t.Errorf("error = %q, want %q", got, tt.code)
			}
		})
	}

	// None of the failures used up the invite.
	if rec := post(mux, "/api/auth/register", "application/json", registerBody(code, "mo@example.com", goodPassword)); rec.Code != http.StatusCreated {
		t.Errorf("register after bad requests = %d, want 201", rec.Code)
	}
}

func TestRegisterHTTPMethod(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/register", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/auth/register = %d, want 405", rec.Code)
	}
}
