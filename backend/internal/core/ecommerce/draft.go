// Package ecommerce provides safe, compiled order calculators and quote totals.
package ecommerce

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Kind identifies a calculator category.
type Kind string

const (
	KindPayment  Kind = "payment"
	KindShipment Kind = "shipment"
	KindTax      Kind = "tax"

	TaxModeCharge          = "charge"
	TaxModeDeclareExternal = "declare_external"
)

// OrderDraft is the stable calculator input contract.
type OrderDraft struct {
	TenantID        uuid.UUID      `json:"tenant_id"`
	Currency        string         `json:"currency"`
	Lines           []DraftLine    `json:"lines"`
	Customer        *DraftCustomer `json:"customer,omitempty"`
	PaymentMethod   *MethodRef     `json:"payment_method,omitempty"`
	ShipmentMethod  *MethodRef     `json:"shipment_method,omitempty"`
	TaxMethod       *MethodRef     `json:"tax_method,omitempty"`
	TaxCountryCode  string         `json:"tax_country_code,omitempty"`
	ShippingAddress map[string]any `json:"shipping_address,omitempty"`
	BillingAddress  map[string]any `json:"billing_address,omitempty"`

	Subtotal      float64 `json:"-"`
	ShippingTotal float64 `json:"-"`
	TaxTotal      float64 `json:"-"`
	TaxMode       string  `json:"-"`
}

// DraftLine is an order line before persistence.
type DraftLine struct {
	ProductID uuid.UUID      `json:"product_id,omitempty"`
	VariantID *uuid.UUID     `json:"variant_id,omitempty"`
	Title     string         `json:"title,omitempty"`
	SKU       string         `json:"sku,omitempty"`
	Quantity  int            `json:"quantity"`
	UnitPrice float64        `json:"unit_price"`
	Options   map[string]any `json:"options,omitempty"`
}

// DraftCustomer contains calculator-relevant customer data.
type DraftCustomer struct {
	ID                   *uuid.UUID `json:"id,omitempty"`
	Email                string     `json:"email,omitempty"`
	Name                 string     `json:"name,omitempty"`
	Phone                string     `json:"phone,omitempty"`
	IdentificationType   string     `json:"identification_type,omitempty"`
	IdentificationNumber string     `json:"identification_number,omitempty"`
}

// MethodRef selects a registered provider and passes its persisted configuration.
type MethodRef struct {
	Provider string          `json:"provider"`
	Config   json.RawMessage `json:"config,omitempty"`
}

// CalcResult is returned by an individual calculator.
type CalcResult struct {
	Amount   float64        `json:"amount"`
	TaxMode  string         `json:"tax_mode,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
