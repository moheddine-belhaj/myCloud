package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// verify calls the ForwardAuth endpoint the way Traefik does: always GET,
// the session cookie from the browser, and the original request described
// in X-Forwarded-* headers.
func verify(s *Service, token string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	NewHandler(s).Verify(rec, req)
	return rec
}

func TestVerify(t *testing.T) {
	h, s := newTestAPI(t)
	user := newUser(t, s, "user@example.com", RoleUser)
	admin := newUser(t, s, "admin@example.com", RoleAdmin)
	admin2FA := newUser(t, s, "admin2fa@example.com", RoleAdmin)
	enableTOTPFor(t, s, admin2FA)

	userTok := login(t, h, user.Email).Value
	adminTok := login(t, h, admin.Email).Value
	admin2FATok := login(t, h, admin2FA.Email).Value

	tests := []struct {
		name     string
		token    string
		status   int
		userID   string
		role     string
		errorKey string
	}{
		{"no cookie", "", http.StatusUnauthorized, "", "", "unauthenticated"},
		{"unknown token", "not-a-real-session-token-xxxxxxxxxxxxxxxxxx", http.StatusUnauthorized, "", "", "unauthenticated"},
		{"user", userTok, http.StatusOK, user.ID.String(), RoleUser, ""},
		{"admin without 2FA is a user here", adminTok, http.StatusOK, admin.ID.String(), RoleUser, ""},
		{"admin with 2FA", admin2FATok, http.StatusOK, admin2FA.ID.String(), RoleAdmin, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := verify(s, tc.token, nil)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.status, rec.Body)
			}
			if got := rec.Header().Get(HeaderUserID); got != tc.userID {
				t.Errorf("%s = %q, want %q", HeaderUserID, got, tc.userID)
			}
			if got := rec.Header().Get(HeaderUserRole); got != tc.role {
				t.Errorf("%s = %q, want %q", HeaderUserRole, got, tc.role)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if tc.errorKey != "" {
				if got := errorCode(t, rec); got != tc.errorKey {
					t.Errorf("error = %q, want %q", got, tc.errorKey)
				}
			} else if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body)
			}
		})
	}
}

func TestVerifyAfterLogout(t *testing.T) {
	h, s := newTestAPI(t)
	u := newUser(t, s, "mo@example.com", RoleUser)
	tok := login(t, h, u.Email).Value

	if rec := verify(s, tok, nil); rec.Code != http.StatusOK {
		t.Fatalf("before logout: %d, want 200", rec.Code)
	}
	if err := s.Logout(context.Background(), tok); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if rec := verify(s, tok, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("after logout: %d, want 401", rec.Code)
	}
}

// Verify applies the CSRF check to the request Traefik is about to forward,
// so services behind ForwardAuth don't need their own.
func TestVerifyCrossOrigin(t *testing.T) {
	h, s := newTestAPI(t)
	u := newUser(t, s, "mo@example.com", RoleUser)
	tok := login(t, h, u.Email).Value

	tests := []struct {
		name   string
		hdr    map[string]string
		status int
	}{
		{"cross-site POST", map[string]string{"X-Forwarded-Method": "POST", "Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-site POST from a subdomain", map[string]string{"X-Forwarded-Method": "POST", "Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"same-origin POST", map[string]string{"X-Forwarded-Method": "POST", "Sec-Fetch-Site": "same-origin"}, http.StatusOK},
		{"cross-site GET (a link)", map[string]string{"X-Forwarded-Method": "GET", "Sec-Fetch-Site": "cross-site"}, http.StatusOK},
		{"DELETE from another Origin", map[string]string{
			"X-Forwarded-Method": "DELETE", "X-Forwarded-Host": "cloud.mohedine.dev", "Origin": "https://evil.example",
		}, http.StatusForbidden},
		{"PUT from the same Origin", map[string]string{
			"X-Forwarded-Method": "PUT", "X-Forwarded-Host": "cloud.mohedine.dev", "Origin": "https://cloud.mohedine.dev",
		}, http.StatusOK},
		{"POST without browser headers (curl)", map[string]string{"X-Forwarded-Method": "POST"}, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := verify(s, tok, tc.hdr)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.status, rec.Body)
			}
			if tc.status == http.StatusForbidden {
				if got := errorCode(t, rec); got != "cross_origin" {
					t.Errorf("error = %q, want cross_origin", got)
				}
				if rec.Header().Get(HeaderUserID) != "" {
					t.Error("rejected request still got an identity header")
				}
			}
		})
	}
}

// A database failure must not let requests through.
func TestVerifyFailsClosed(t *testing.T) {
	h, s := newTestAPI(t)
	u := newUser(t, s, "mo@example.com", RoleUser)
	tok := login(t, h, u.Email).Value

	s.pool.Close()
	rec := verify(s, tok, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if rec.Header().Get(HeaderUserID) != "" {
		t.Error("failed request still got an identity header")
	}
}
