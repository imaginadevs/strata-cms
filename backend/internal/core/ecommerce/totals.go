package ecommerce

import (
	"context"
	"fmt"
	"math"
)

// QuoteResult contains the calculated order totals.
type QuoteResult struct {
	Currency        string         `json:"currency"`
	Subtotal        float64        `json:"subtotal"`
	ShippingTotal   float64        `json:"shipping_total"`
	TaxTotal        float64        `json:"tax_total"`
	TaxMode         string         `json:"tax_mode"`
	PaymentFeeTotal float64        `json:"payment_fee_total"`
	Total           float64        `json:"total"`
	TaxMetadata     map[string]any `json:"tax_metadata,omitempty"`
}

// Quote runs tax, shipment, then payment calculators and computes the total.
func Quote(ctx context.Context, draft OrderDraft) (QuoteResult, error) {
	if draft.TenantID == [16]byte{} {
		return QuoteResult{}, fmt.Errorf("tenant_id is required")
	}
	for _, line := range draft.Lines {
		if line.Quantity < 0 || line.UnitPrice < 0 {
			return QuoteResult{}, fmt.Errorf("line quantity and unit_price must be non-negative")
		}
		draft.Subtotal += float64(line.Quantity) * line.UnitPrice
	}
	draft.Subtotal = roundMoney(draft.Subtotal)
	taxMode := TaxModeCharge
	var tax CalcResult
	var err error
	if draft.TaxMethod != nil && draft.TaxMethod.Provider != "" {
		tax, err = Call(ctx, KindTax, draft.TaxMethod.Provider, draft.TenantID, draft, draft.TaxMethod.Config)
		if err != nil {
			return QuoteResult{}, fmt.Errorf("calculate tax: %w", err)
		}
		if tax.TaxMode != "" {
			taxMode = tax.TaxMode
		}
	}
	draft.TaxTotal, draft.TaxMode = roundMoney(tax.Amount), taxMode
	if draft.ShipmentMethod != nil && draft.ShipmentMethod.Provider != "" {
		shipment, callErr := Call(ctx, KindShipment, draft.ShipmentMethod.Provider, draft.TenantID, draft, draft.ShipmentMethod.Config)
		if callErr != nil {
			return QuoteResult{}, fmt.Errorf("calculate shipment: %w", callErr)
		}
		draft.ShippingTotal = roundMoney(shipment.Amount)
	}
	paymentFee := 0.0
	if draft.PaymentMethod != nil && draft.PaymentMethod.Provider != "" {
		payment, callErr := Call(ctx, KindPayment, draft.PaymentMethod.Provider, draft.TenantID, draft, draft.PaymentMethod.Config)
		if callErr != nil {
			return QuoteResult{}, fmt.Errorf("calculate payment: %w", callErr)
		}
		paymentFee = roundMoney(payment.Amount)
	}
	total := draft.Subtotal + draft.ShippingTotal + paymentFee
	if taxMode != TaxModeDeclareExternal {
		total += draft.TaxTotal
	}
	return QuoteResult{Currency: draft.Currency, Subtotal: draft.Subtotal, ShippingTotal: draft.ShippingTotal, TaxTotal: draft.TaxTotal, TaxMode: taxMode, PaymentFeeTotal: paymentFee, Total: roundMoney(total), TaxMetadata: tax.Metadata}, nil
}

func roundMoney(value float64) float64 { return math.Round(value*100) / 100 }
