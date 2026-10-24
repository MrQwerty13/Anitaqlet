package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/handlers"
	"github.com/mikhail/anitaqlet/backend/internal/payments"
)

type fakeGateway struct {
	payment *payments.GatewayPayment
	err     error
}

func (f *fakeGateway) CreatePayment(ctx context.Context, amountRUB int, returnURL, description, userID, idempotenceKey string) (*payments.GatewayPayment, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.payment, nil
}

func (f *fakeGateway) GetPayment(ctx context.Context, id string) (*payments.GatewayPayment, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.payment, nil
}

func webhookRouter(t *testing.T, h *handlers.PaymentsHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/payments/yookassa/webhook", h.YooKassaWebhook)
	return r
}

func postWebhook(t *testing.T, r *gin.Engine, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/payments/yookassa/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestYooKassaWebhookActivatesPremium(t *testing.T) {
	e := setupRouter(t)
	gw := &fakeGateway{payment: &payments.GatewayPayment{
		ID:       "pay-123",
		Status:   "succeeded",
		Amount:   payments.Amount{Value: "99.00", Currency: "RUB"},
		Metadata: map[string]string{"user_id": e.userID},
	}}
	h := handlers.NewPaymentsHandler(gw, e.payStore(), e.users, config.Config{PremiumPriceRUB: 99})
	r := webhookRouter(t, h)

	payload := map[string]interface{}{
		"type":   "payment.succeeded",
		"object": map[string]interface{}{"id": "pay-123", "status": "succeeded"},
	}
	w := postWebhook(t, r, payload)
	if w.Code != http.StatusOK {
		t.Fatalf("webhook: %d %s", w.Code, w.Body.String())
	}

	u, err := e.users.GetByID(e.userID)
	if err != nil || u == nil {
		t.Fatalf("user reload: %v", err)
	}
	if !u.PremiumActive() {
		t.Fatalf("expected premium active, got is_premium=%v until=%v", u.IsPremium, u.PremiumUntil)
	}

	w2 := postWebhook(t, r, payload)
	if w2.Code != http.StatusOK {
		t.Fatalf("duplicate webhook: %d %s", w2.Code, w2.Body.String())
	}

	p, err := e.payStore().GetByExternalID("pay-123")
	if err != nil || p == nil || p.Status != "succeeded" {
		t.Fatalf("payment row: %+v err=%v", p, err)
	}
}

func TestYooKassaWebhookIgnoresOtherEvents(t *testing.T) {
	e := setupRouter(t)
	gw := &fakeGateway{payment: &payments.GatewayPayment{ID: "x", Status: "pending"}}
	h := handlers.NewPaymentsHandler(gw, e.payStore(), e.users, config.Config{PremiumPriceRUB: 99})
	r := webhookRouter(t, h)

	w := postWebhook(t, r, map[string]interface{}{
		"type":   "payment.canceled",
		"object": map[string]interface{}{"id": "x"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}

	u, _ := e.users.GetByID(e.userID)
	if u.PremiumActive() {
		t.Fatal("premium must not activate on canceled event")
	}
}

func TestYooKassaWebhookRejectsAmountMismatch(t *testing.T) {
	e := setupRouter(t)
	gw := &fakeGateway{payment: &payments.GatewayPayment{
		ID:       "pay-bad",
		Status:   "succeeded",
		Amount:   payments.Amount{Value: "1.00", Currency: "RUB"},
		Metadata: map[string]string{"user_id": e.userID},
	}}
	h := handlers.NewPaymentsHandler(gw, e.payStore(), e.users, config.Config{PremiumPriceRUB: 99})
	r := webhookRouter(t, h)

	w := postWebhook(t, r, map[string]interface{}{
		"type":   "payment.succeeded",
		"object": map[string]interface{}{"id": "pay-bad"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d %s", w.Code, w.Body.String())
	}
	u, _ := e.users.GetByID(e.userID)
	if u.PremiumActive() {
		t.Fatal("premium must not activate on amount mismatch")
	}
}

func TestActivatePremiumExtends(t *testing.T) {
	e := setupRouter(t)
	if err := e.users.ActivatePremium(e.userID, 30); err != nil {
		t.Fatal(err)
	}
	u, _ := e.users.GetByID(e.userID)
	if !u.PremiumActive() {
		t.Fatal("expected active premium")
	}
	if err := e.users.ActivatePremium(e.userID, 30); err != nil {
		t.Fatal(err)
	}
}
