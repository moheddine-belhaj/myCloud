package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

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
// POST /api/auth/invites (admin only) arrives in PR 3, together with sessions.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", h.register)
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
