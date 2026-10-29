package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/moheddine-belhaj/mycloud/backend/internal/auth/password"
)

// maxBodyBytes caps every JSON request body.
const maxBodyBytes = 1 << 20 // 1 MiB

// Handler serves the auth HTTP API under /api/auth/.
type Handler struct {
	svc *Service
}

// NewHandler returns the HTTP layer for svc.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the API routes on mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/login/totp", h.loginTOTP)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.Handle("GET /api/auth/me", h.RequireSession(http.HandlerFunc(h.me)))
	mux.Handle("POST /api/auth/invites", h.RequireSession(RequireAdmin(http.HandlerFunc(h.createInvite))))
	mux.Handle("POST /api/auth/2fa/setup", h.RequireSession(http.HandlerFunc(h.totpSetup)))
	mux.Handle("POST /api/auth/2fa/enable", h.RequireSession(http.HandlerFunc(h.totpEnable)))
	mux.Handle("POST /api/auth/2fa/disable", h.RequireSession(http.HandlerFunc(h.totpDisable)))
	mux.Handle("POST /api/auth/2fa/recovery-codes", h.RequireSession(http.HandlerFunc(h.recoveryCodes)))
}

type registerRequest struct {
	InviteCode string `json:"inviteCode"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

type registerResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := h.svc.Register(r.Context(), req.InviteCode, req.Email, req.Password)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, registerResponse{ID: u.ID.String(), Email: u.Email})
	case errors.Is(err, ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email", "Enter a valid email address.")
	case errors.Is(err, password.ErrLength):
		writeError(w, http.StatusBadRequest, "invalid_password",
			fmt.Sprintf("Use a password of %d to %d characters.", password.MinLength, password.MaxLength))
	case errors.Is(err, ErrInvalidInvite):
		writeError(w, http.StatusBadRequest, "invalid_invite", "The invite code is invalid, already used or expired.")
	case errors.Is(err, ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "An account with this email already exists.")
	default:
		internalError(w, r, err)
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.svc.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	var mfa *MFARequiredError
	switch {
	case err == nil:
		setSessionCookie(w, s)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	case errors.As(err, &mfa):
		// Password OK, 2FA on: no session yet. The client sends the code to
		// /api/auth/login/totp; the browser sends the __Host-mfa cookie with it.
		setMFACookie(w, mfa.Challenge)
		writeJSON(w, http.StatusOK, loginResponse{MFARequired: true})
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Wrong email or password.")
	default:
		internalError(w, r, err)
	}
}

type loginResponse struct {
	MFARequired bool `json:"mfaRequired"`
}

type loginTOTPRequest struct {
	Code string `json:"code"` // 6-digit TOTP code or a recovery code
}

// loginTOTP is the second login step.
func (h *Handler) loginTOTP(w http.ResponseWriter, r *http.Request) {
	var req loginTOTPRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.svc.LoginTOTP(r.Context(), mfaToken(r), req.Code, r.UserAgent())
	switch {
	case err == nil:
		clearMFACookie(w)
		setSessionCookie(w, s)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrInvalidCode):
		writeError(w, http.StatusBadRequest, "invalid_code", "The code is wrong, expired or already used.")
	case errors.Is(err, ErrNoChallenge):
		clearMFACookie(w)
		writeError(w, http.StatusUnauthorized, "mfa_expired", "Log in with your password again.")
	default:
		writeTOTPError(w, r, err)
	}
}

// logout always answers 204 and clears the cookie, even without a session.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), sessionToken(r)); err != nil {
		internalError(w, r, err)
		return
	}
	clearSessionCookie(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context()) // RequireSession guarantees it
	writeJSON(w, http.StatusOK, u)
}

type createInviteRequest struct {
	ExpiresInHours *int `json:"expiresInHours"` // optional; default 168 (7 days)
}

func (h *Handler) createInvite(w http.ResponseWriter, r *http.Request) {
	var req createInviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ttl := DefaultInviteTTL
	if req.ExpiresInHours != nil {
		// Check the range before multiplying, so a huge number can't overflow
		// into a valid-looking duration.
		hours := *req.ExpiresInHours
		if hours < 1 || hours > int(MaxInviteTTL/time.Hour) {
			writeError(w, http.StatusBadRequest, "invalid_ttl", "expiresInHours must be between 1 and 720.")
			return
		}
		ttl = time.Duration(hours) * time.Hour
	}
	admin, _ := UserFromContext(r.Context())
	inv, err := h.svc.CreateInvite(r.Context(), &admin.ID, ttl)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, inv)
	case errors.Is(err, ErrInvalidTTL):
		writeError(w, http.StatusBadRequest, "invalid_ttl", "expiresInHours must be between 1 and 720.")
	default:
		internalError(w, r, err)
	}
}

// totpSetup starts 2FA setup. The body is an empty JSON object: {}.
func (h *Handler) totpSetup(w http.ResponseWriter, r *http.Request) {
	var req struct{}
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := UserFromContext(r.Context())
	setup, err := h.svc.SetupTOTP(r.Context(), u)
	if err != nil {
		writeTOTPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, setup) // writeJSON sets Cache-Control: no-store
}

type totpEnableRequest struct {
	Code string `json:"code"`
}

type totpEnableResponse struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}

func (h *Handler) totpEnable(w http.ResponseWriter, r *http.Request) {
	var req totpEnableRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := UserFromContext(r.Context())
	codes, err := h.svc.EnableTOTP(r.Context(), u, req.Code, sessionToken(r))
	if err != nil {
		writeTOTPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, totpEnableResponse{RecoveryCodes: codes})
}

type totpDisableRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (h *Handler) totpDisable(w http.ResponseWriter, r *http.Request) {
	var req totpDisableRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := UserFromContext(r.Context())
	if err := h.svc.DisableTOTP(r.Context(), u, req.Password, req.Code); err != nil {
		writeTOTPError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

type recoveryCodesRequest struct {
	Code string `json:"code"` // from the authenticator app; recovery codes are refused
}

// recoveryCodes replaces the user's recovery codes with 10 new ones.
func (h *Handler) recoveryCodes(w http.ResponseWriter, r *http.Request) {
	var req recoveryCodesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, _ := UserFromContext(r.Context())
	codes, err := h.svc.RegenerateRecoveryCodes(r.Context(), u, req.Code)
	if err != nil {
		writeTOTPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, totpEnableResponse{RecoveryCodes: codes})
}

// writeTOTPError maps the 2FA errors to responses.
func writeTOTPError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCode):
		writeError(w, http.StatusBadRequest, "invalid_code", "The code is wrong, expired or already used.")
	case errors.Is(err, ErrWrongPassword):
		writeError(w, http.StatusForbidden, "wrong_password", "The password is wrong.")
	case errors.Is(err, ErrTOTPAlreadyEnabled):
		writeError(w, http.StatusConflict, "totp_already_enabled", "Two-factor authentication is already on.")
	case errors.Is(err, ErrTOTPNotSetUp):
		writeError(w, http.StatusConflict, "totp_not_set_up", "Start two-factor setup first.")
	case errors.Is(err, ErrTOTPNotEnabled):
		writeError(w, http.StatusConflict, "totp_not_enabled", "Two-factor authentication is not on.")
	case errors.Is(err, ErrTOTPUnavailable):
		writeError(w, http.StatusServiceUnavailable, "totp_unavailable", "Two-factor authentication is not available.")
	default:
		internalError(w, r, err)
	}
}

// decodeJSON reads one JSON object from the body into v. On failure it writes
// the error response and returns false.
//
// Requiring Content-Type: application/json is also a CSRF defence: a plain
// HTML form on another site can't send that content type without a CORS
// preflight, which this API never approves.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Send the body as application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "The request body is too large.")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "The request body is not valid JSON for this endpoint.")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF { // exactly one JSON value
		writeError(w, http.StatusBadRequest, "invalid_json", "The request body must contain a single JSON object.")
		return false
	}
	return true
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorResponse{Error: code, Message: msg})
}

// internalError logs the real error and returns a generic 500, so internals
// (SQL, hostnames) never reach the client.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return // client went away; nobody is reading the response
	}
	slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong. Try again later.")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
