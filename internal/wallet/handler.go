package wallet

import (
	"encoding/json"
	"net/http"

	"github.com/fluxa/fluxa/internal/api"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() func(r chi.Router) {
	return func(r chi.Router) {
		r.Post("/", h.createWallet)
		r.Get("/{id}/balances", h.getBalances)
		r.Post("/{id}/trustlines", h.addTrustline)
	}
}

type addTrustlineRequest struct {
	Asset  string `json:"asset" validate:"required"`
	Issuer string `json:"issuer,omitempty"`
	Limit  string `json:"limit,omitempty"`
}

func (h *Handler) createWallet(w http.ResponseWriter, r *http.Request) {
	wallet, err := h.svc.CreateWallet(r.Context())
	if err != nil {
		api.InternalError(w, err)
		return
	}

	api.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":         wallet.ID,
		"public_key": wallet.PublicKey,
		"created_at": wallet.CreatedAt,
	})
}

func (h *Handler) getBalances(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	includeFX := r.URL.Query().Get("include_fx")

	balances, err := h.svc.GetBalances(r.Context(), id, includeFX)
	if err != nil {
		api.HandleDomainError(w, err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]interface{}{
		"wallet_id": id,
		"balances":  balances,
	})
}

func (h *Handler) addTrustline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req addTrustlineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.BadRequest(w, "invalid request body")
		return
	}
	if err := api.Validate(req); err != nil {
		api.BadRequest(w, err.Error())
		return
	}

	txHash, err := h.svc.AddTrustline(r.Context(), id, req.Asset, req.Issuer, req.Limit)
	if err != nil {
		api.HandleDomainError(w, err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]interface{}{
		"status":    "confirmed",
		"wallet_id": id,
		"asset":     req.Asset,
		"tx_hash":   txHash,
	})
}
