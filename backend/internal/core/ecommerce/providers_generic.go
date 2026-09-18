package ecommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func init() { EnsureGenerics() }

// EnsureGenerics registers all built-in providers. Registration is idempotent.
func EnsureGenerics() {
	Register(KindPayment, "generic.payment.manual", nil, fixedZero)
	Register(KindPayment, "generic.payment.surcharge_pct", nil, paymentSurcharge)
	Register(KindShipment, "generic.shipment.flat_rate", nil, flatShipment)
	Register(KindTax, "generic.tax.none", nil, noTax)
	Register(KindTax, "generic.tax.rate_by_country", nil, taxByCountry)
}

func fixedZero(context.Context, OrderDraft, json.RawMessage) (CalcResult, error) {
	return CalcResult{Amount: 0}, nil
}

func paymentSurcharge(_ context.Context, draft OrderDraft, raw json.RawMessage) (CalcResult, error) {
	cfg, err := configMap(raw)
	if err != nil {
		return CalcResult{}, err
	}
	percent, err := number(cfg["percent"])
	if err != nil {
		return CalcResult{}, fmt.Errorf("payment surcharge percent: %w", err)
	}
	basis := draft.Subtotal + draft.ShippingTotal
	if draft.TaxMode != TaxModeDeclareExternal {
		basis += draft.TaxTotal
	}
	return CalcResult{Amount: roundMoney(basis * percent / 100)}, nil
}

func flatShipment(_ context.Context, _ OrderDraft, raw json.RawMessage) (CalcResult, error) {
	cfg, err := configMap(raw)
	if err != nil {
		return CalcResult{}, err
	}
	flat, err := number(cfg["flat"])
	if err != nil {
		return CalcResult{}, fmt.Errorf("shipment flat rate: %w", err)
	}
	return CalcResult{Amount: roundMoney(flat)}, nil
}

func noTax(context.Context, OrderDraft, json.RawMessage) (CalcResult, error) {
	return CalcResult{Amount: 0, TaxMode: TaxModeCharge}, nil
}

func taxByCountry(_ context.Context, draft OrderDraft, raw json.RawMessage) (CalcResult, error) {
	cfg, err := configMap(raw)
	if err != nil {
		return CalcResult{}, err
	}
	rates, _ := cfg["rates"].(map[string]any)
	country := strings.ToUpper(draft.TaxCountryCode)
	value := rates[country]
	if value == nil {
		value = rates[strings.ToLower(country)]
	}
	percent, err := number(value)
	if err != nil && value != nil {
		return CalcResult{}, fmt.Errorf("tax rate for %s: %w", country, err)
	}
	mode := TaxModeCharge
	if draft.PaymentMethod != nil {
		paymentCfg, parseErr := configMap(draft.PaymentMethod.Config)
		if parseErr != nil {
			return CalcResult{}, fmt.Errorf("payment config: %w", parseErr)
		}
		if flexibleBool(paymentCfg["declare_tax_with_provider"]) {
			mode = TaxModeDeclareExternal
		}
	}
	return CalcResult{Amount: roundMoney(draft.Subtotal * percent / 100), TaxMode: mode}, nil
}

func configMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid calculator config: %w", err)
	}
	return cfg, nil
}

func number(value any) (float64, error) {
	if value == nil {
		return 0, nil
	}
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("expected number, got %T", value)
	}
}

func flexibleBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	case float64:
		return v != 0
	default:
		return false
	}
}
