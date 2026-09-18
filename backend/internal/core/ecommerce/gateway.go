package ecommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// A PaymentGateway initiates and verifies real charges for a payment method.
// This is distinct from a fee CalculatorFunc (which only computes an amount):
// a gateway starts a charge (often returning a redirect/checkout), and verifies
// asynchronous webhook callbacks. A payment method's `provider` selects the
// gateway; its `config` (JSON) carries the credentials.
type PaymentGateway interface {
	// ID is the provider identifier matched against a payment method's provider.
	ID() string
	// ValidateConfig checks that the stored method config is usable.
	ValidateConfig(config json.RawMessage) error
	// CreatePayment starts a charge for an already-persisted order.
	CreatePayment(ctx context.Context, req PaymentRequest) (PaymentResult, error)
	// VerifyWebhook authenticates and normalises a raw provider callback.
	VerifyWebhook(body []byte, headers map[string]string, config json.RawMessage) (WebhookEvent, error)
}

// PaymentStatus is the normalised state of a payment across gateways.
type PaymentStatus string

const (
	PayPending  PaymentStatus = "pending"
	PayApproved PaymentStatus = "approved"
	PayDeclined PaymentStatus = "declined"
	PayVoided   PaymentStatus = "voided"
	PayError    PaymentStatus = "error"
)

// TransactionStatus maps a normalised payment status onto the values used by the
// ecommerce_order_transactions.status field.
func (p PaymentStatus) TransactionStatus() string {
	switch p {
	case PayApproved:
		return "captured"
	case PayDeclined, PayError:
		return "failed"
	case PayVoided:
		return "refunded"
	default:
		return "pending"
	}
}

// PaymentRequest is the gateway-agnostic input to start a charge.
type PaymentRequest struct {
	TenantID    uuid.UUID
	Reference   string // our unique order reference
	AmountCents int64  // integer minor units
	Currency    string
	Customer    *DraftCustomer
	RedirectURL string // where the buyer returns after paying
	Config      json.RawMessage
}

// PaymentResult is returned after initiating a charge.
type PaymentResult struct {
	Status            PaymentStatus  `json:"status"`
	ProviderReference string         `json:"provider_reference,omitempty"`
	RedirectURL       string         `json:"redirect_url,omitempty"`
	Checkout          map[string]any `json:"checkout,omitempty"` // params for a client-side widget
	Raw               map[string]any `json:"raw,omitempty"`
}

// WebhookEvent is a normalised, authenticated gateway callback.
type WebhookEvent struct {
	Reference         string
	ProviderReference string
	Status            PaymentStatus
	AmountCents       int64
	Raw               map[string]any
}

// ─── gateway registry ─────────────────────────────────────────────────────────

var gateways = struct {
	sync.RWMutex
	byID map[string]PaymentGateway
}{byID: map[string]PaymentGateway{}}

// RegisterGateway adds or replaces a payment gateway by its ID.
func RegisterGateway(g PaymentGateway) {
	if g == nil || g.ID() == "" {
		panic("ecommerce: gateway and its ID are required")
	}
	gateways.Lock()
	defer gateways.Unlock()
	gateways.byID[g.ID()] = g
}

// Gateway returns the registered gateway for a provider id.
func Gateway(providerID string) (PaymentGateway, error) {
	gateways.RLock()
	defer gateways.RUnlock()
	g, ok := gateways.byID[providerID]
	if !ok {
		return nil, fmt.Errorf("ecommerce: no payment gateway for provider %q", providerID)
	}
	return g, nil
}

// ListGateways returns the ids of all registered gateways.
func ListGateways() []string {
	gateways.RLock()
	defer gateways.RUnlock()
	out := make([]string, 0, len(gateways.byID))
	for id := range gateways.byID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
