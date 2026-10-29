package auth

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// CookieName is the session cookie. The __Host- prefix makes browsers accept
// it only when it is Secure, has Path=/ and no Domain, so it is locked to the
// exact host that set it (cloud.mohedine.dev, not any subdomain).
const CookieName = "__Host-session"

// setSessionCookie stores the session token in the browser.
//
//   - HttpOnly: JavaScript can't read it, so an XSS bug can't steal it.
//   - Secure: only sent over HTTPS.
//   - SameSite=Strict: not sent on requests started by other sites (CSRF).
func setSessionCookie(w http.ResponseWriter, s Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    s.Token,
		Path:     "/",
		Expires:  s.ExpiresAt,
		MaxAge:   int(time.Until(s.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie tells the browser to delete the cookie.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // sent as Max-Age=0: delete now
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// MFACookieName holds a half-finished login (password OK, code missing).
// Same attributes as the session cookie, but it lives only 5 minutes and
// grants nothing on its own.
const MFACookieName = "__Host-mfa"

func setMFACookie(w http.ResponseWriter, c Challenge) {
	http.SetCookie(w, &http.Cookie{
		Name:     MFACookieName,
		Value:    c.Token,
		Path:     "/",
		Expires:  c.ExpiresAt,
		MaxAge:   int(time.Until(c.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearMFACookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: MFACookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

// mfaToken returns the challenge token from the request's cookie, or "".
func mfaToken(r *http.Request) string {
	c, err := r.Cookie(MFACookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// sessionToken returns the token from the request's cookie, or "".
func sessionToken(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

type userKey struct{}

// UserFromContext returns the user that RequireSession put in the context.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// RequireSession lets the request through only with a valid session cookie,
// and puts the user in the request context for the next handler.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := h.svc.UserBySession(r.Context(), sessionToken(r))
		if errors.Is(err, ErrNoSession) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Log in first.")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}

// RequireAdmin lets only admins through, and only with 2FA on: an admin
// account can create invites, so a password alone must not be enough.
// An admin without 2FA can still log in and turn it on (/api/auth/2fa/*).
// Put it after RequireSession.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Log in first.")
			return
		}
		if u.Role != RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "Only admins can do this.")
			return
		}
		if !u.TOTPEnabled {
			writeError(w, http.StatusForbidden, "totp_required", "Turn on two-factor authentication to use admin features.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Protect rejects cross-origin POST/PUT/PATCH/DELETE requests, using Go's
// http.CrossOriginProtection: a browser sends Sec-Fetch-Site (or Origin) with
// every such request, and anything that isn't same-origin gets 403. Requests
// without those headers (curl, Postman, server-to-server) are allowed: they
// don't carry a victim's cookies. GET, HEAD and OPTIONS always pass.
func Protect(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, "cross_origin", "Cross-origin requests are not allowed.")
	}))
	return cop.Handler(next)
}
