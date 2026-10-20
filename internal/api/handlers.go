// Package api serves the game's HTTP endpoints.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"be-demo/internal/store"
	"be-demo/internal/web"
)

const (
	maxBodyBytes = 1 << 20 // 1 MB
	defaultLimit = 10
	maxLimit     = 100
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

type handler struct {
	store *store.Store
}

// New returns the API routes.
func New(s *store.Store) http.Handler {
	h := &handler{store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /players", h.createPlayer)
	mux.HandleFunc("POST /scores", h.submitScore)
	mux.HandleFunc("GET /leaderboard", h.leaderboard)
	mux.HandleFunc("GET /health", health)
	mux.Handle("GET /{$}", web.Handler())
	return mux
}

type createPlayerRequest struct {
	Username string `json:"username"`
}

func (h *handler) createPlayer(w http.ResponseWriter, r *http.Request) {
	var req createPlayerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username must not be empty")
		return
	}
	if !usernamePattern.MatchString(username) {
		writeError(w, http.StatusBadRequest,
			"username must be 1-32 characters of letters, digits, '_' or '-'")
		return
	}

	p, err := h.store.CreatePlayer(r.Context(), username)
	switch {
	case errors.Is(err, store.ErrUsernameTaken):
		writeError(w, http.StatusConflict, fmt.Sprintf("username %q is already taken", username))
	case err != nil:
		writeInternalError(w, "create player", err)
	default:
		writeJSON(w, http.StatusCreated, p)
	}
}

type submitScoreRequest struct {
	Username string `json:"username"`
	// Distinguish missing/null from zero.
	Score *int64 `json:"score"`
}

func (h *handler) submitScore(w http.ResponseWriter, r *http.Request) {
	var req submitScoreRequest
	if err := decodeJSON(w, r, &req); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field == "score" {
			writeError(w, http.StatusBadRequest, "score must be a non-negative integer")
			return
		}
		writeDecodeError(w, err)
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username must not be empty")
		return
	}
	if req.Score == nil || *req.Score < 0 {
		writeError(w, http.StatusBadRequest, "score must be a non-negative integer")
		return
	}

	sc, err := h.store.AddScore(r.Context(), username, *req.Score)
	switch {
	case errors.Is(err, store.ErrPlayerNotFound):
		writeError(w, http.StatusNotFound, fmt.Sprintf("player %q not found", username))
	case err != nil:
		writeInternalError(w, "add score", err)
	default:
		writeJSON(w, http.StatusCreated, sc)
	}
}

type leaderboardResponse struct {
	Leaderboard []store.Entry `json:"leaderboard"`
	Total       int           `json:"total"`
}

func (h *handler) leaderboard(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryInt(r, "limit", defaultLimit, 1, maxLimit)
	if !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("limit must be an integer between 1 and %d", maxLimit))
		return
	}
	offset, ok := queryInt(r, "offset", 0, 0, math.MaxInt)
	if !ok {
		writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
		return
	}

	entries, total, err := h.store.Leaderboard(r.Context(), limit, offset)
	if err != nil {
		writeInternalError(w, "leaderboard", err)
		return
	}
	writeJSON(w, http.StatusOK, leaderboardResponse{Leaderboard: entries, Total: total})
}

func queryInt(r *http.Request, key string, defaultValue, minValue, maxValue int) (int, bool) {
	values, ok := r.URL.Query()[key]
	if !ok {
		return defaultValue, true
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < minValue || n > maxValue {
		return 0, false
	}
	return n, true
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

var errTrailingData = errors.New("trailing data after JSON object")

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Require EOF while preserving errors such as http.MaxBytesError.
	switch err := dec.Decode(&struct{}{}); {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		return err
	default:
		return errTrailingData
	}
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body must not exceed 1 MB")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid JSON body")
}

func writeInternalError(w http.ResponseWriter, op string, err error) {
	slog.Error(op, "err", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}
