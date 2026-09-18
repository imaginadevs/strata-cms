package ecommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { RegisterGateway(manualGateway{}) }

// manualGateway is offline/manual payment: the order is created and left pending
// for an operator to reconcile. No external calls and no webhooks.
type manualGateway struct{}

func (manualGateway) ID() string { return "generic.payment.manual" }

func (manualGateway) ValidateConfig(json.RawMessage) error { return nil }

func (manualGateway) CreatePayment(context.Context, PaymentRequest) (PaymentResult, error) {
	return PaymentResult{Status: PayPending}, nil
}

func (manualGateway) VerifyWebhook([]byte, map[string]string, json.RawMessage) (WebhookEvent, error) {
	return WebhookEvent{}, fmt.Errorf("manual payment does not support webhooks")
}
