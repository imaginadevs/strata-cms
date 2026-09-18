package ecommerce

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func init() { RegisterGateway(&wompiGateway{}) }

// wompiGateway integrates Wompi (Colombia). Wompi is redirect-based: CreatePayment
// builds a Web Checkout URL (signed with the integrity secret) that the buyer is
// sent to; the authoritative result arrives asynchronously via VerifyWebhook,
// authenticated with the events secret.
type wompiGateway struct{}

func (*wompiGateway) ID() string { return "wompi" }

type wompiConfig struct {
	PublicKey       string `json:"public_key"`
	PrivateKey      string `json:"private_key"`
	EventsSecret    string `json:"events_secret"`
	IntegritySecret string `json:"integrity_secret"`
	Environment     string `json:"environment"` // sandbox | production
	RedirectURL     string `json:"redirect_url"`
}

func parseWompiConfig(raw json.RawMessage) (wompiConfig, error) {
	var c wompiConfig
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &c); err != nil {
			return c, fmt.Errorf("invalid wompi config: %w", err)
		}
	}
	if c.Environment == "" {
		c.Environment = "sandbox"
	}
	return c, nil
}

func (g *wompiGateway) ValidateConfig(raw json.RawMessage) error {
	c, err := parseWompiConfig(raw)
	if err != nil {
		return err
	}
	if c.PublicKey == "" {
		return fmt.Errorf("wompi: public_key is required")
	}
	if c.IntegritySecret == "" {
		return fmt.Errorf("wompi: integrity_secret is required")
	}
	if c.Environment != "sandbox" && c.Environment != "production" {
		return fmt.Errorf("wompi: environment must be sandbox or production")
	}
	return nil
}

func (g *wompiGateway) CreatePayment(_ context.Context, req PaymentRequest) (PaymentResult, error) {
	if err := g.ValidateConfig(req.Config); err != nil {
		return PaymentResult{}, err
	}
	c, _ := parseWompiConfig(req.Config)

	currency := strings.ToUpper(req.Currency)
	amount := strconv.FormatInt(req.AmountCents, 10)
	// Integrity signature: SHA256(reference + amountInCents + currency + integritySecret).
	sig := sha256Hex(req.Reference + amount + currency + c.IntegritySecret)

	redirect := req.RedirectURL
	if redirect == "" {
		redirect = c.RedirectURL
	}
	params := url.Values{}
	params.Set("public-key", c.PublicKey)
	params.Set("currency", currency)
	params.Set("amount-in-cents", amount)
	params.Set("reference", req.Reference)
	params.Set("signature:integrity", sig)
	if redirect != "" {
		params.Set("redirect-url", redirect)
	}
	if req.Customer != nil && req.Customer.Email != "" {
		params.Set("customer-data:email", req.Customer.Email)
	}

	return PaymentResult{
		Status:      PayPending,
		RedirectURL: "https://checkout.wompi.co/p/?" + params.Encode(),
		Checkout: map[string]any{
			"public_key":          c.PublicKey,
			"currency":            currency,
			"amount_in_cents":     req.AmountCents,
			"reference":           req.Reference,
			"signature_integrity": sig,
			"redirect_url":        redirect,
			"environment":         c.Environment,
		},
	}, nil
}

func (g *wompiGateway) VerifyWebhook(body []byte, _ map[string]string, raw json.RawMessage) (WebhookEvent, error) {
	c, err := parseWompiConfig(raw)
	if err != nil {
		return WebhookEvent{}, err
	}
	if c.EventsSecret == "" {
		return WebhookEvent{}, fmt.Errorf("wompi: events_secret is required to verify webhooks")
	}

	var evt struct {
		Event     string         `json:"event"`
		Data      map[string]any `json:"data"`
		Signature struct {
			Checksum   string   `json:"checksum"`
			Properties []string `json:"properties"`
		} `json:"signature"`
		Timestamp json.Number `json:"timestamp"`
	}
	// UseNumber so large COP amounts keep exact digits for the checksum.
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&evt); err != nil {
		return WebhookEvent{}, fmt.Errorf("wompi webhook: invalid body: %w", err)
	}

	// Signed string = concatenated property values + timestamp + events secret.
	var b strings.Builder
	for _, prop := range evt.Signature.Properties {
		b.WriteString(fmt.Sprint(digInto(evt.Data, strings.Split(prop, "."))))
	}
	b.WriteString(evt.Timestamp.String())
	b.WriteString(c.EventsSecret)
	if !strings.EqualFold(sha256Hex(b.String()), evt.Signature.Checksum) {
		return WebhookEvent{}, fmt.Errorf("wompi webhook: signature mismatch")
	}

	tx, _ := evt.Data["transaction"].(map[string]any)
	var amountCents int64
	if n, err := number(tx["amount_in_cents"]); err == nil {
		amountCents = int64(n)
	}
	return WebhookEvent{
		Reference:         fmt.Sprint(tx["reference"]),
		ProviderReference: fmt.Sprint(tx["id"]),
		Status:            wompiStatus(fmt.Sprint(tx["status"])),
		AmountCents:       amountCents,
		Raw:               evt.Data,
	}, nil
}

func wompiStatus(s string) PaymentStatus {
	switch strings.ToUpper(s) {
	case "APPROVED":
		return PayApproved
	case "DECLINED":
		return PayDeclined
	case "VOIDED":
		return PayVoided
	case "ERROR":
		return PayError
	default:
		return PayPending
	}
}

func digInto(m map[string]any, path []string) any {
	var cur any = m
	for _, key := range path {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = asMap[key]
	}
	if cur == nil {
		return ""
	}
	return cur
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
