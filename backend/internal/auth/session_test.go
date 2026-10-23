package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestAPI returns the full API (routes + cross-origin protection) on a
// fresh database.
func newTestAPI(t *testing.T) (http.Handler, *Service) {
	t.Helper()
	mux, s := newTestMux(t)
	return Protect(mux), s
}

// do sends a request with an optional JSON body and session token.
func do(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// login logs in over HTTP and returns the session cookie.
func login(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	rec := do(h, http.MethodPost, "/api/auth/login", `{"email":"`+email+`","password":"`+goodPassword+`"}`, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login %s = %d %s, want 204", email, rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	t.Fatalf("login %s: no %s cookie", email, CookieName)
	return nil
}

func TestLoginCookie(t *testing.T) {
	h, s := newTestAPI(t)
	newUser(t, s, "mo@example.com", RoleUser)

	c := login(t, h, "mo@example.com")
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Strict, Path=/, no Domain", c)
	}
	if c.MaxAge < int(SessionTTL.Seconds())-60 || c.MaxAge > int(SessionTTL.Seconds()) {
		t.Errorf("Max-Age = %d, want ~%d", c.MaxAge, int(SessionTTL.Seconds()))
	}
	if len(c.Value) != 43 {
		t.Errorf("token length = %d, want 43", len(c.Value))
	}
}

func TestLoginHTTPFailures(t *testing.T) {
	h, s := newTestAPI(t)
	newUser(t, s, "mo@example.com", RoleUser)

	for name, body := range map[string]string{
		"wrong password": `{"email":"mo@example.com","password":"wrong password here"}`,
		"unknown email":  `{"email":"nobody@example.com","password":"` + goodPassword + `"}`,
	} {
		rec := do(h, http.MethodPost, "/api/auth/login", body, "")
		if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "invalid_credentials" {
			t.Errorf("%s: %d %s, want 401 invalid_credentials", name, rec.Code, rec.Body)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: a cookie was set", name)
		}
	}
	if rec := do(h, http.MethodPost, "/api/auth/login", `{"email":"mo@example.com","pass":"x"}`, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d, want 400", rec.Code)
	}
}

func TestMe(t *testing.T) {
	h, s := newTestAPI(t)
	u := newUser(t, s, "mo@example.com", RoleUser)

	for name, tok := range map[string]string{"no cookie": "", "random cookie": "made-up-token"} {
		rec := do(h, http.MethodGet, "/api/auth/me", "", tok)
		if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthenticated" {
			t.Errorf("%s: %d %s, want 401 unauthenticated", name, rec.Code, rec.Body)
		}
	}

	c := login(t, h, "mo@example.com")
	rec := do(h, http.MethodGet, "/api/auth/me", "", c.Value)
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d %s, want 200", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["id"] != u.ID.String() || got["email"] != "mo@example.com" || got["role"] != RoleUser || got["createdAt"] == nil {
		t.Errorf("me = %v", got)
	}
	if _, ok := got["passwordHash"]; ok {
		t.Error("me leaks the password hash")
	}
}

func TestLogoutHTTP(t *testing.T) {
	h, s := newTestAPI(t)
	newUser(t, s, "mo@example.com", RoleUser)
	c := login(t, h, "mo@example.com")

	rec := do(h, http.MethodPost, "/api/auth/logout", "", c.Value)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", rec.Code)
	}
	cleared := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == CookieName && ck.MaxAge < 0 && ck.Value == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("logout did not clear the cookie: %v", rec.Header().Values("Set-Cookie"))
	}
	if rec := do(h, http.MethodGet, "/api/auth/me", "", c.Value); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", rec.Code)
	}
	// Again, and with no cookie at all: still 204.
	for _, tok := range []string{c.Value, ""} {
		if rec := do(h, http.MethodPost, "/api/auth/logout", "", tok); rec.Code != http.StatusNoContent {
			t.Errorf("logout(%q) = %d, want 204", tok, rec.Code)
		}
	}
}

func TestCreateInviteHTTP(t *testing.T) {
	h, s := newTestAPI(t)
	admin := newUser(t, s, "admin@example.com", RoleAdmin)
	newUser(t, s, "user@example.com", RoleUser)
	adminCookie := login(t, h, "admin@example.com")
	userCookie := login(t, h, "user@example.com")

	if rec := do(h, http.MethodPost, "/api/auth/invites", `{}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: %d, want 401", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/auth/invites", `{}`, userCookie.Value); rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
		t.Errorf("plain user: %d %s, want 403 forbidden", rec.Code, rec.Body)
	}
	for _, body := range []string{`{"expiresInHours":0}`, `{"expiresInHours":721}`, `{"expiresInHours":9223372036854775807}`} {
		if rec := do(h, http.MethodPost, "/api/auth/invites", body, adminCookie.Value); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_ttl" {
			t.Errorf("%s: %d %s, want 400 invalid_ttl", body, rec.Code, rec.Body)
		}
	}

	rec := do(h, http.MethodPost, "/api/auth/invites", `{"expiresInHours":24}`, adminCookie.Value)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin: %d %s, want 201", rec.Code, rec.Body)
	}
	var inv Invite
	if err := json.NewDecoder(rec.Body).Decode(&inv); err != nil || len(inv.Code) != 43 || inv.ExpiresAt.IsZero() {
		t.Fatalf("invite = %+v, err %v", inv, err)
	}

	// created_by is the admin, and the code works for registration.
	var createdBy string
	if err := s.pool.QueryRow(context.Background(), "SELECT created_by::text FROM invites WHERE code_hash = sha256(convert_to($1, 'UTF8'))", inv.Code).Scan(&createdBy); err != nil {
		t.Fatalf("read invite: %v", err)
	}
	if createdBy != admin.ID.String() {
		t.Errorf("created_by = %s, want %s", createdBy, admin.ID)
	}
	if rec := do(h, http.MethodPost, "/api/auth/register", registerBody(inv.Code, "friend@example.com", goodPassword), ""); rec.Code != http.StatusCreated {
		t.Errorf("register with HTTP invite = %d %s, want 201", rec.Code, rec.Body)
	}
}

func TestCrossOriginPostIsRejected(t *testing.T) {
	h, s := newTestAPI(t)
	newUser(t, s, "mo@example.com", RoleUser)
	body := `{"email":"mo@example.com","password":"` + goodPassword + `"}`

	for _, tc := range []struct {
		name   string
		header map[string]string
		want   int
	}{
		{"other site (Sec-Fetch-Site)", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"other site (Origin only)", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"same origin browser", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"curl / Postman (no headers)", nil, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, tc.want)
			}
			if tc.want == http.StatusForbidden && errorCode(t, rec) != "cross_origin" {
				t.Errorf("error code is not cross_origin")
			}
		})
	}
}

// TestFullFlow walks the whole M2 story through the HTTP API.
func TestFullFlow(t *testing.T) {
	h, s := newTestAPI(t)
	newUser(t, s, "admin@example.com", RoleAdmin) // the CLI's create-admin

	admin := login(t, h, "admin@example.com")
	rec := do(h, http.MethodPost, "/api/auth/invites", `{}`, admin.Value)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite = %d %s", rec.Code, rec.Body)
	}
	var inv Invite
	_ = json.NewDecoder(rec.Body).Decode(&inv)

	if rec := do(h, http.MethodPost, "/api/auth/register", registerBody(inv.Code, "friend@example.com", goodPassword), ""); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", rec.Code, rec.Body)
	}
	friend := login(t, h, "friend@example.com")
	if rec := do(h, http.MethodGet, "/api/auth/me", "", friend.Value); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"user"`) {
		t.Fatalf("me = %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/auth/invites", `{}`, friend.Value); rec.Code != http.StatusForbidden {
		t.Errorf("friend creates invite = %d, want 403", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/auth/logout", "", friend.Value); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/api/auth/me", "", friend.Value); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", rec.Code)
	}
	// The admin's session is untouched.
	if rec := do(h, http.MethodGet, "/api/auth/me", "", admin.Value); rec.Code != http.StatusOK {
		t.Errorf("admin me = %d, want 200", rec.Code)
	}
}
