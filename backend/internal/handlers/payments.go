package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/payments"
	"github.com/mikhail/anitaqlet/backend/internal/store"
)

const premiumDays = 30

type PaymentsHandler struct {
	gateway payments.Gateway
	store   *store.PaymentStore
	users   *store.UserStore
	cfg     config.Config
}

func NewPaymentsHandler(gateway payments.Gateway, payStore *store.PaymentStore, users *store.UserStore, cfg config.Config) *PaymentsHandler {
	return &PaymentsHandler{gateway: gateway, store: payStore, users: users, cfg: cfg}
}

func (h *PaymentsHandler) Create(c *gin.Context) {
	if h.gateway == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payments_disabled"})
		return
	}
	u := middleware.UserFrom(c)
	if u.PremiumActive() {
		c.JSON(http.StatusConflict, gin.H{"error": "already_premium"})
		return
	}
	payment, err := h.gateway.CreatePayment(
		c.Request.Context(),
		h.cfg.PremiumPriceRUB,
		h.cfg.YooKassaReturnURL,
		"Anitaqlet Premium, 30 дней",
		u.ID,
		uuid.NewString(),
	)
	if err != nil {
		slog.Error("payments: create failed", slog.String("error", err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{"error": "payment create failed"})
		return
	}
	if payment.Confirmation == nil || payment.Confirmation.RedirectURL == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "payment confirmation missing"})
		return
	}
	_ = h.store.InsertPending(u.ID, payment.ID, h.cfg.PremiumPriceRUB*100, "RUB")
	c.JSON(http.StatusOK, gin.H{
		"payment_id":   payment.ID,
		"redirect_url": payment.Confirmation.RedirectURL,
	})
}

type webhookRequest struct {
	Type   string `json:"type"`
	Object struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount struct {
			Value    string `json:"value"`
			Currency string `json:"currency"`
		} `json:"amount"`
		Metadata map[string]string `json:"metadata"`
	} `json:"object"`
}

func (h *PaymentsHandler) YooKassaWebhook(c *gin.Context) {
	if h.gateway == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payments_disabled"})
		return
	}
	var req webhookRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Object.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook"})
		return
	}
	if req.Type != "payment.succeeded" {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": true})
		return
	}
	payment, err := h.gateway.GetPayment(c.Request.Context(), req.Object.ID)
	if err != nil {
		slog.Error("payments: webhook verify failed", slog.String("error", err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{"error": "verification failed"})
		return
	}
	if payment.Status != "succeeded" {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": true, "status": payment.Status})
		return
	}
	if payment.Amount.Value != formatRUB(h.cfg.PremiumPriceRUB) || payment.Amount.Currency != "RUB" {
		slog.Warn("payments: amount mismatch",
			slog.String("payment_id", payment.ID),
			slog.String("value", payment.Amount.Value),
			slog.String("currency", payment.Amount.Currency),
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount mismatch"})
		return
	}
	userID := ""
	if payment.Metadata != nil {
		userID = payment.Metadata["user_id"]
	}
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing user_id metadata"})
		return
	}
	user, err := h.users.GetByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown user"})
		return
	}
	created, err := h.store.UpsertSucceeded(userID, payment.ID, h.cfg.PremiumPriceRUB*100, "RUB")
	if err != nil {
		slog.Error("payments: save failed", slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}
	if created {
		if err := h.users.ActivatePremium(userID, premiumDays); err != nil {
			slog.Error("payments: premium activate failed", slog.String("error", err.Error()))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "premium activate failed"})
			return
		}
		slog.Info("payments: premium activated", slog.String("user_id", userID), slog.String("payment_id", payment.ID))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func formatRUB(rub int) string {
	return fmt.Sprintf("%d.00", rub)
}
