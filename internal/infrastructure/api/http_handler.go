package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-email-api/internal/application"
	"go-email-api/internal/domain"
)

type HTTPHandler struct {
	service *application.Service
}

func NewHTTPHandler(service *application.Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/accounts", h.accounts)
	mux.HandleFunc("/accounts/", h.accountsSubResource)
	return withSecurityHeaders(mux)
}

func (h *HTTPHandler) health(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HTTPHandler) accounts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name     string          `json:"name"`
			Protocol domain.Protocol `json:"protocol"`
			Host     string          `json:"host"`
			Port     int             `json:"port"`
			Username string          `json:"username"`
			Password string          `json:"password"`
			UseTLS   bool            `json:"use_tls"`
			Mailbox  string          `json:"mailbox"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		account, err := h.service.RegisterAccount(ctx, domain.Account{
			Name:     req.Name,
			Protocol: req.Protocol,
			Host:     req.Host,
			Port:     req.Port,
			Username: req.Username,
			Password: req.Password,
			UseTLS:   req.UseTLS,
			Mailbox:  req.Mailbox,
		})
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondJSON(w, http.StatusCreated, account)
	case http.MethodGet:
		accounts, err := h.service.ListAccounts(ctx)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, accounts)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *HTTPHandler) accountsSubResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/accounts/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	accountID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	if len(parts) == 2 && parts[1] == "sync" && r.Method == http.MethodPost {
		stored, err := h.service.SyncAccount(ctx, accountID)
		if err != nil {
			respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"stored": stored})
		return
	}
	if len(parts) == 2 && parts[1] == "emails" && r.Method == http.MethodGet {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		emails, err := h.service.ListEmails(ctx, accountID, limit, offset)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, emails)
		return
	}
	respondError(w, http.StatusNotFound, "not found")
}

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, code int, msg string) {
	respondJSON(w, code, map[string]string{"error": msg})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none';")
		next.ServeHTTP(w, r)
	})
}
