package ecommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// CalculatorFunc computes one component of an order quote.
type CalculatorFunc func(ctx context.Context, draft OrderDraft, config json.RawMessage) (CalcResult, error)

// ProviderInfo describes an available compiled calculator.
type ProviderInfo struct {
	Kind         Kind   `json:"kind"`
	ProviderID   string `json:"provider_id"`
	TenantScoped bool   `json:"tenant_scoped"`
}

type calculatorEntry struct {
	scope *uuid.UUID
	fn    CalculatorFunc
}

var calculators = struct {
	sync.RWMutex
	entries map[Kind]map[string][]calculatorEntry
}{entries: make(map[Kind]map[string][]calculatorEntry)}

// Register adds or replaces a compiled calculator. A nil scope makes it generic.
func Register(kind Kind, providerID string, tenantScope *uuid.UUID, fn CalculatorFunc) {
	if providerID == "" || fn == nil {
		panic("ecommerce: provider ID and calculator are required")
	}
	if kind != KindPayment && kind != KindShipment && kind != KindTax {
		panic("ecommerce: invalid calculator kind")
	}
	calculators.Lock()
	defer calculators.Unlock()
	if calculators.entries[kind] == nil {
		calculators.entries[kind] = make(map[string][]calculatorEntry)
	}
	entries := calculators.entries[kind][providerID]
	for i, entry := range entries {
		if sameScope(entry.scope, tenantScope) {
			entries[i] = calculatorEntry{scope: copyScope(tenantScope), fn: fn}
			calculators.entries[kind][providerID] = entries
			return
		}
	}
	calculators.entries[kind][providerID] = append(entries, calculatorEntry{scope: copyScope(tenantScope), fn: fn})
}

// Call invokes a tenant override when present, otherwise a generic calculator.
func Call(ctx context.Context, kind Kind, providerID string, tenantID uuid.UUID, draft OrderDraft, config json.RawMessage) (CalcResult, error) {
	calculators.RLock()
	entries := append([]calculatorEntry(nil), calculators.entries[kind][providerID]...)
	calculators.RUnlock()
	for _, entry := range entries {
		if entry.scope != nil && *entry.scope == tenantID {
			return entry.fn(ctx, draft, config)
		}
	}
	for _, entry := range entries {
		if entry.scope == nil {
			return entry.fn(ctx, draft, config)
		}
	}
	return CalcResult{}, fmt.Errorf("ecommerce calculator not found: kind=%q provider=%q", kind, providerID)
}

// List returns generic and matching tenant-scoped providers. Tenant overrides win.
func List(kind Kind, tenantID uuid.UUID) []ProviderInfo {
	calculators.RLock()
	defer calculators.RUnlock()
	out := make([]ProviderInfo, 0)
	for providerID, entries := range calculators.entries[kind] {
		hasGeneric, hasTenant := false, false
		for _, entry := range entries {
			hasGeneric = hasGeneric || entry.scope == nil
			hasTenant = hasTenant || (entry.scope != nil && *entry.scope == tenantID)
		}
		if hasTenant || hasGeneric {
			out = append(out, ProviderInfo{Kind: kind, ProviderID: providerID, TenantScoped: hasTenant})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProviderID < out[j].ProviderID })
	return out
}

func sameScope(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func copyScope(scope *uuid.UUID) *uuid.UUID {
	if scope == nil {
		return nil
	}
	copy := *scope
	return &copy
}
