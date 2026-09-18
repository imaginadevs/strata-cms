package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ecommerce"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountEcommerce registers the ecommerce plugin routes. All are gated on the
// plugin being installed for the tenant. Webhooks are public (authenticated by
// the gateway signature); the rest require the relevant permission.
func MountEcommerce(r chi.Router, repos *postgresrepo.Repos, log *logger.Logger) {
	h := &ecommerceHandler{repos: repos, log: log}
	r.Route("/ecommerce", func(r chi.Router) {
		r.Use(requireEcommercePlugin)
		r.With(httpMiddleware.RequirePermission(repos.Permission, userdomain.VirtualSettings, userdomain.ActionRead)).Get("/providers", h.providers)
		r.With(httpMiddleware.RequirePermission(repos.Permission, userdomain.VirtualSettings, userdomain.ActionUpdate)).Post("/quote", h.quote)
		r.With(httpMiddleware.RequirePermission(repos.Permission, userdomain.VirtualSettings, userdomain.ActionUpdate)).Post("/payment-methods/validate", h.validatePaymentMethod)
		// Storefront checkout: cart-scoped, no admin permission required.
		r.Post("/checkout", h.checkout)
		// Public, signature-verified gateway callback.
		r.Post("/webhooks/{provider}", h.webhook)
	})
}

type ecommerceHandler struct {
	repos *postgresrepo.Repos
	log   *logger.Logger
}

func requireEcommercePlugin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, ok := httpMiddleware.TenantFromCtx(r.Context())
		if !ok || t == nil {
			writeError(w, http.StatusUnauthorized, "tenant not resolved")
			return
		}
		t.Settings.NormalizePlugins()
		if !t.Settings.PluginInstalled("ecommerce") {
			writeError(w, http.StatusNotFound, "ecommerce plugin is not installed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *ecommerceHandler) providers(w http.ResponseWriter, r *http.Request) {
	kind := ecommerce.Kind(r.URL.Query().Get("kind"))
	if kind != ecommerce.KindPayment && kind != ecommerce.KindShipment && kind != ecommerce.KindTax {
		writeError(w, http.StatusBadRequest, "kind must be payment, shipment, or tax")
		return
	}
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"data": ecommerce.List(kind, tenantID)})
}

func (h *ecommerceHandler) quote(w http.ResponseWriter, r *http.Request) {
	var draft ecommerce.OrderDraft
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		writeError(w, http.StatusBadRequest, "invalid order draft: "+err.Error())
		return
	}
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	draft.TenantID = tenantID
	result, err := ecommerce.Quote(r.Context(), draft)
	if err != nil {
		h.log.Warnw("ecommerce quote failed", "tenant", tenantID, "err", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// validatePaymentMethod checks a gateway config (by payment_method_id or an
// inline provider+config) without persisting anything.
func (h *ecommerceHandler) validatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	var body struct {
		PaymentMethodID string          `json:"payment_method_id"`
		Provider        string          `json:"provider"`
		Config          json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	provider, config := body.Provider, body.Config
	if body.PaymentMethodID != "" {
		method, err := h.getItem(r, tenantID, "ecommerce_payment_methods", body.PaymentMethodID)
		if err != nil || method == nil {
			writeError(w, http.StatusNotFound, "payment method not found")
			return
		}
		provider = asString(method["provider"])
		config = rawConfig(method["config"])
	}
	gw, err := ecommerce.Gateway(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := gw.ValidateConfig(config); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}

type checkoutRequest struct {
	CartID           string                   `json:"cart_id"`
	CartToken        string                   `json:"cart_token"`
	Customer         *ecommerce.DraftCustomer `json:"customer"`
	PaymentMethodID  string                   `json:"payment_method_id"`
	ShipmentMethodID string                   `json:"shipment_method_id"`
	TaxMethodID      string                   `json:"tax_method_id"`
	TaxCountryCode   string                   `json:"tax_country_code"`
	ShippingAddress  map[string]any           `json:"shipping_address"`
	BillingAddress   map[string]any           `json:"billing_address"`
	RedirectURL      string                   `json:"redirect_url"`
}

// checkout converts a cart into an order: re-quotes server-side, validates and
// decrements stock (when track_inventory is on), creates the order + items +
// a pending transaction, marks the cart converted, then starts the gateway
// payment. Order + items are written atomically (SaveGraph); see the follow-up
// note in docs about making the transaction/inventory/cart writes atomic too.
func (h *ecommerceHandler) checkout(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	tenant, _ := httpMiddleware.TenantFromCtx(r.Context())

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	// 1. Resolve the cart (by id or guest token) and its items.
	cart, err := h.resolveCart(r, tenantID, req.CartID, req.CartToken)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if asString(cart["status"]) == "converted" {
		writeError(w, http.StatusConflict, "cart already checked out")
		return
	}
	cartID := toIDString(cart["id"])
	cartItems, _, err := h.repos.Item.List(r.Context(), tenantID, "ecommerce_cart_items",
		outbound.QueryFilter{Filter: map[string]any{"cart_id": cartID}, Limit: 500})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load cart items failed")
		return
	}
	if len(cartItems) == 0 {
		writeError(w, http.StatusBadRequest, "cart is empty")
		return
	}

	// 2. Build authoritative order lines from products/variants (never trust
	//    client prices), and gather stock for validation.
	currency := asString(cart["currency"])
	if currency == "" {
		currency = "USD"
	}
	lines := make([]ecommerce.DraftLine, 0, len(cartItems))
	type stockLine struct {
		collection, id string
		available, qty int
	}
	var stockLines []stockLine
	for _, ci := range cartItems {
		qty := asInt(ci["quantity"])
		if qty <= 0 {
			continue
		}
		productID := toIDString(ci["product_id"])
		variantID := toIDString(ci["variant_id"])
		var src map[string]any
		coll, srcID := "ecommerce_products", productID
		if variantID != "" {
			coll, srcID = "ecommerce_product_variants", variantID
		}
		src, err = h.getItem(r, tenantID, coll, srcID)
		if err != nil || src == nil {
			writeError(w, http.StatusBadRequest, "product not found for a cart line")
			return
		}
		line := ecommerce.DraftLine{
			Title:     asString(src["title"]),
			SKU:       asString(src["sku"]),
			Quantity:  qty,
			UnitPrice: asFloat(src["price"]),
		}
		if pid, e := uuid.Parse(productID); e == nil {
			line.ProductID = pid
		}
		if variantID != "" {
			if vid, e := uuid.Parse(variantID); e == nil {
				line.VariantID = &vid
			}
		}
		lines = append(lines, line)
		stockLines = append(stockLines, stockLine{collection: coll, id: srcID, available: asInt(src["stock"]), qty: qty})
	}

	// 3. Resolve methods and re-quote server-side.
	draft := ecommerce.OrderDraft{
		TenantID: tenantID, Currency: currency, Lines: lines, Customer: req.Customer,
		TaxCountryCode: req.TaxCountryCode, ShippingAddress: req.ShippingAddress, BillingAddress: req.BillingAddress,
	}
	payMethod, err := h.methodRef(r, tenantID, "ecommerce_payment_methods", req.PaymentMethodID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	draft.PaymentMethod = payMethod.ref
	draft.ShipmentMethod = h.optionalMethodRef(r, tenantID, "ecommerce_shipment_methods", req.ShipmentMethodID)
	draft.TaxMethod = h.optionalMethodRef(r, tenantID, "ecommerce_tax_methods", req.TaxMethodID)

	quote, err := ecommerce.Quote(r.Context(), draft)
	if err != nil {
		writeError(w, http.StatusBadRequest, "quote failed: "+err.Error())
		return
	}

	// 4. Inventory gate.
	track := tenant != nil && tenant.Settings.Ecommerce.TrackInventory
	backorder := tenant != nil && tenant.Settings.Ecommerce.AllowBackorder
	if track && !backorder {
		for _, s := range stockLines {
			if s.qty > s.available {
				writeError(w, http.StatusConflict, "insufficient stock")
				return
			}
		}
	}

	// 5. Create the order + items atomically.
	orderNumber := "ORD-" + strings.ToUpper(uuid.NewString()[:8])
	orderData := map[string]any{
		"order_number": orderNumber, "status": "pending",
		"customer_email": customerEmail(req.Customer), "currency": currency,
		"subtotal": quote.Subtotal, "shipping_total": quote.ShippingTotal, "tax_total": quote.TaxTotal,
		"payment_fee_total": quote.PaymentFeeTotal, "total": quote.Total, "tax_mode": quote.TaxMode,
		"payment_method_id": nullUUID(req.PaymentMethodID), "shipment_method_id": nullUUID(req.ShipmentMethodID),
		"tax_method_id": nullUUID(req.TaxMethodID), "tax_country_code": req.TaxCountryCode,
		"shipping_address": orEmptyObj(req.ShippingAddress), "billing_address": orEmptyObj(req.BillingAddress),
		"placed_at": time.Now().UTC(),
	}
	if req.Customer != nil {
		orderData["customer_name"] = req.Customer.Name
		orderData["customer_phone"] = req.Customer.Phone
		orderData["identification_type"] = req.Customer.IdentificationType
		orderData["identification_number"] = req.Customer.IdentificationNumber
		if req.Customer.ID != nil {
			orderData["customer_id"] = req.Customer.ID.String()
		}
	}
	itemRows := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		row := map[string]any{
			"title": l.Title, "sku": l.SKU, "quantity": l.Quantity,
			"unit_price": l.UnitPrice, "subtotal": roundMoney2(l.UnitPrice * float64(l.Quantity)),
		}
		if l.ProductID != (uuid.UUID{}) {
			row["product_id"] = l.ProductID.String()
		}
		if l.VariantID != nil {
			row["variant_id"] = l.VariantID.String()
		}
		itemRows = append(itemRows, row)
	}
	order, err := h.repos.Item.SaveGraph(r.Context(), tenantID, "ecommerce_orders", "", orderData,
		[]outbound.ChildReconcile{{Collection: "ecommerce_order_items", FKColumn: "order_id", Owned: true, Rows: itemRows}}, true)
	if err != nil || order == nil {
		h.log.Errorw("checkout: create order failed", "err", err, "tenant", tenantID)
		writeError(w, http.StatusInternalServerError, "could not create order")
		return
	}
	orderID := toIDString(order["id"])

	// 6. Pending transaction, inventory decrement, cart conversion. (Not yet in
	//    one DB transaction with step 5 — see the atomicity follow-up.)
	txn, _ := h.repos.Item.Create(r.Context(), tenantID, "ecommerce_order_transactions", map[string]any{
		"order_id": orderID, "payment_method_id": nullUUID(req.PaymentMethodID),
		"status": "pending", "amount": quote.Total, "currency": currency, "data": map[string]any{},
	})
	if track {
		for _, s := range stockLines {
			_, _ = h.repos.Item.Update(r.Context(), tenantID, s.collection, s.id, map[string]any{"stock": s.available - s.qty})
		}
	}
	_, _ = h.repos.Item.Update(r.Context(), tenantID, "ecommerce_carts", cartID, map[string]any{"status": "converted", "order_id": orderID})

	// 7. Start the gateway payment.
	gw, err := ecommerce.Gateway(payMethod.provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pay, err := gw.CreatePayment(r.Context(), ecommerce.PaymentRequest{
		TenantID: tenantID, Reference: orderNumber, AmountCents: int64(roundMoney2(quote.Total) * 100),
		Currency: currency, Customer: req.Customer, RedirectURL: req.RedirectURL, Config: payMethod.config,
	})
	if err != nil {
		h.log.Warnw("checkout: gateway create payment failed", "err", err, "provider", payMethod.provider)
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"order": order, "payment_error": err.Error()}})
		return
	}
	if txn != nil {
		_, _ = h.repos.Item.Update(r.Context(), tenantID, "ecommerce_order_transactions", toIDString(txn["id"]), map[string]any{
			"status": string(pay.Status.TransactionStatus()), "provider_reference": pay.ProviderReference, "data": pay.Checkout,
		})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"order": order, "payment": pay}})
}

// webhook receives a gateway callback, authenticates it against the matching
// payment method's config, and advances the order/transaction status.
func (h *ecommerceHandler) webhook(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusBadRequest, "tenant not resolved")
		return
	}
	provider := chi.URLParam(r, "provider")
	gw, err := ecommerce.Gateway(provider)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown provider")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	config := h.providerConfig(r, tenantID, provider)
	headers := map[string]string{}
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}
	event, err := gw.VerifyWebhook(body, headers, config)
	if err != nil {
		h.log.Warnw("ecommerce webhook rejected", "provider", provider, "err", err)
		writeError(w, http.StatusBadRequest, "invalid webhook")
		return
	}
	// Find the order by reference (order_number) and advance status.
	orders, _, err := h.repos.Item.List(r.Context(), tenantID, "ecommerce_orders",
		outbound.QueryFilter{Filter: map[string]any{"order_number": event.Reference}, Limit: 1})
	if err == nil && len(orders) == 1 {
		orderID := toIDString(orders[0]["id"])
		_, _ = h.repos.Item.Update(r.Context(), tenantID, "ecommerce_orders", orderID,
			map[string]any{"status": orderStatusFor(event.Status)})
		txns, _, _ := h.repos.Item.List(r.Context(), tenantID, "ecommerce_order_transactions",
			outbound.QueryFilter{Filter: map[string]any{"order_id": orderID}, Sort: []outbound.SortField{{Field: "created_at", Desc: true}}, Limit: 1})
		if len(txns) == 1 {
			_, _ = h.repos.Item.Update(r.Context(), tenantID, "ecommerce_order_transactions", toIDString(txns[0]["id"]),
				map[string]any{"status": event.Status.TransactionStatus(), "provider_reference": event.ProviderReference})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
}

func orderStatusFor(s ecommerce.PaymentStatus) string {
	switch s {
	case ecommerce.PayApproved:
		return "paid"
	case ecommerce.PayDeclined, ecommerce.PayError:
		return "awaiting_payment"
	case ecommerce.PayVoided:
		return "cancelled"
	default:
		return "awaiting_payment"
	}
}

// ─── small helpers ─────────────────────────────────────────────────────────────

type resolvedMethod struct {
	provider string
	config   json.RawMessage
	ref      *ecommerce.MethodRef
}

func (h *ecommerceHandler) resolveCart(r *http.Request, tenantID uuid.UUID, id, token string) (map[string]any, error) {
	if id != "" {
		c, err := h.getItem(r, tenantID, "ecommerce_carts", id)
		if err != nil || c == nil {
			return nil, fmt.Errorf("cart not found")
		}
		return c, nil
	}
	if token != "" {
		rows, _, err := h.repos.Item.List(r.Context(), tenantID, "ecommerce_carts",
			outbound.QueryFilter{Filter: map[string]any{"token": token}, Limit: 1})
		if err != nil || len(rows) != 1 {
			return nil, fmt.Errorf("cart not found")
		}
		return rows[0], nil
	}
	return nil, fmt.Errorf("cart_id or cart_token is required")
}

func (h *ecommerceHandler) methodRef(r *http.Request, tenantID uuid.UUID, coll, id string) (resolvedMethod, error) {
	if id == "" {
		return resolvedMethod{}, fmt.Errorf("payment_method_id is required")
	}
	m, err := h.getItem(r, tenantID, coll, id)
	if err != nil || m == nil {
		return resolvedMethod{}, fmt.Errorf("payment method not found")
	}
	provider := asString(m["provider"])
	cfg := rawConfig(m["config"])
	return resolvedMethod{provider: provider, config: cfg, ref: &ecommerce.MethodRef{Provider: provider, Config: cfg}}, nil
}

func (h *ecommerceHandler) optionalMethodRef(r *http.Request, tenantID uuid.UUID, coll, id string) *ecommerce.MethodRef {
	if id == "" {
		return nil
	}
	m, err := h.methodRef(r, tenantID, coll, id)
	if err != nil {
		return nil
	}
	return m.ref
}

func (h *ecommerceHandler) providerConfig(r *http.Request, tenantID uuid.UUID, provider string) json.RawMessage {
	rows, _, err := h.repos.Item.List(r.Context(), tenantID, "ecommerce_payment_methods",
		outbound.QueryFilter{Filter: map[string]any{"provider": provider}, Limit: 1})
	if err != nil || len(rows) != 1 {
		return nil
	}
	return rawConfig(rows[0]["config"])
}

func (h *ecommerceHandler) getItem(r *http.Request, tenantID uuid.UUID, coll, id string) (map[string]any, error) {
	return h.repos.Item.GetByID(r.Context(), tenantID, coll, id, outbound.QueryFilter{})
}

func rawConfig(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	if raw, ok := v.([]byte); ok {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func customerEmail(c *ecommerce.DraftCustomer) string {
	if c == nil {
		return ""
	}
	return c.Email
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func orEmptyObj(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	default:
		return 0
	}
}

func roundMoney2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
