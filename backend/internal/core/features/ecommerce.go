package features

import collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"

func ecommerceCollections() []Blueprint {
	money := func(name, label string, required bool) FieldBlueprint {
		return FieldBlueprint{Name: name, DisplayName: label, Type: collectiondomain.FieldTypeDecimal, DBType: "numeric(12,2)", Required: required, DefaultValue: 0}
	}
	return []Blueprint{
		{
			Name: "ecommerce_product_categories", DisplayName: "Product Categories", Icon: "folder", Color: "#10b981",
			CompositeUnique: [][]string{{"slug", "language"}},
			Fields: []FieldBlueprint{
				{Name: "name", DisplayName: "Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "slug", DisplayName: "Slug", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "description", DisplayName: "Description", Type: collectiondomain.FieldTypeText, DBType: "text"},
				{Name: "parent_id", DisplayName: "Parent Category", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
			},
		},
		{
			Name: "ecommerce_products", DisplayName: "Products", Icon: "package", Color: "#3b82f6",
			CompositeUnique: [][]string{{"slug", "language"}},
			Fields: []FieldBlueprint{
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "slug", DisplayName: "Slug", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "sku", DisplayName: "SKU", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
				{Name: "description", DisplayName: "Description", Type: collectiondomain.FieldTypeRichText, DBType: "text"},
				money("price", "Price", true),
				currencyField(),
				{Name: "stock", DisplayName: "Stock", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 0},
				statusField("draft", "draft", "published", "archived"),
				{Name: "category_id", DisplayName: "Category", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "featured_image", DisplayName: "Featured Image", Type: collectiondomain.FieldTypeFile, DBType: "text"},
			},
		},
		{
			Name: "ecommerce_product_variants", DisplayName: "Product Variants", Icon: "boxes", Color: "#6366f1",
			Fields: []FieldBlueprint{
				{Name: "product_id", DisplayName: "Product", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "sku", DisplayName: "SKU", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
				{Name: "options", DisplayName: "Options", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
				money("price", "Price", true), currencyField(),
				{Name: "stock", DisplayName: "Stock", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 0},
			},
		},
		{
			Name: "ecommerce_locations", DisplayName: "Locations", Icon: "map-pin", Color: "#14b8a6",
			Fields: []FieldBlueprint{
				{Name: "name", DisplayName: "Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "code", DisplayName: "Code", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
				{Name: "address", DisplayName: "Address", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
				{Name: "active", DisplayName: "Active", Type: collectiondomain.FieldTypeBoolean, DBType: "boolean", Required: true, DefaultValue: true},
			},
		},
		{
			Name: "ecommerce_inventory_levels", DisplayName: "Inventory Levels", Icon: "warehouse", Color: "#0f766e", Hidden: true,
			CompositeUnique: [][]string{{"location_id", "product_id", "variant_id"}},
			Fields: []FieldBlueprint{
				{Name: "location_id", DisplayName: "Location", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "product_id", DisplayName: "Product", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "variant_id", DisplayName: "Variant", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "quantity", DisplayName: "Quantity", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 0},
			},
		},
		{
			Name: "ecommerce_customers", DisplayName: "Customers", Icon: "users", Color: "#8b5cf6", Type: collectiondomain.CollectionTypeAuth,
			Fields: []FieldBlueprint{
				{Name: "email", DisplayName: "Email", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true, Unique: true},
				{Name: "first_name", DisplayName: "First Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "last_name", DisplayName: "Last Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "phone", DisplayName: "Phone", Type: collectiondomain.FieldTypeString, DBType: "varchar(50)"},
				{Name: "identification_type", DisplayName: "Identification Type", Type: collectiondomain.FieldTypeString, DBType: "varchar(50)"},
				{Name: "identification_number", DisplayName: "Identification Number", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "addresses", DisplayName: "Addresses", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: []any{}},
			},
		},
		{
			// Carts support both signed-in customers (customer_id) and guests
			// (token). A cart converts into an order at checkout.
			Name: "ecommerce_carts", DisplayName: "Carts", Icon: "shopping-bag", Color: "#f472b6",
			Fields: []FieldBlueprint{
				{Name: "token", DisplayName: "Token", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Unique: true},
				{Name: "customer_id", DisplayName: "Customer", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "email", DisplayName: "Email", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				currencyField(),
				statusField("active", "active", "converted", "abandoned"),
				{Name: "order_id", DisplayName: "Order", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
			},
		},
		{
			Name: "ecommerce_cart_items", DisplayName: "Cart Items", Icon: "list", Color: "#fb7185", Hidden: true,
			Fields: []FieldBlueprint{
				{Name: "cart_id", DisplayName: "Cart", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "product_id", DisplayName: "Product", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "variant_id", DisplayName: "Variant", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				{Name: "sku", DisplayName: "SKU", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "quantity", DisplayName: "Quantity", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 1},
				money("unit_price", "Unit Price", false),
				{Name: "options", DisplayName: "Options", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
			},
		},
		methodCollection("ecommerce_payment_methods", "Payment Methods", "credit-card", "#f59e0b"),
		methodCollection("ecommerce_shipment_methods", "Shipment Methods", "truck", "#06b6d4"),
		methodCollection("ecommerce_tax_methods", "Tax Methods", "receipt", "#ef4444"),
		{
			Name: "ecommerce_orders", DisplayName: "Orders", Icon: "shopping-cart", Color: "#f97316",
			Fields: []FieldBlueprint{
				{Name: "order_number", DisplayName: "Order Number", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
				statusField("pending", "pending", "awaiting_payment", "paid", "fulfilled", "cancelled", "refunded"),
				{Name: "customer_id", DisplayName: "Customer", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "customer_email", DisplayName: "Customer Email", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "customer_name", DisplayName: "Customer Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				{Name: "customer_phone", DisplayName: "Customer Phone", Type: collectiondomain.FieldTypeString, DBType: "varchar(50)"},
				{Name: "identification_type", DisplayName: "Identification Type", Type: collectiondomain.FieldTypeString, DBType: "varchar(50)"},
				{Name: "identification_number", DisplayName: "Identification Number", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				currencyField(), money("subtotal", "Subtotal", true), money("shipping_total", "Shipping Total", true), money("tax_total", "Tax Total", true), money("payment_fee_total", "Payment Fee Total", true), money("total", "Total", true),
				{Name: "payment_method_id", DisplayName: "Payment Method", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "shipment_method_id", DisplayName: "Shipment Method", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "tax_method_id", DisplayName: "Tax Method", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "tax_country_code", DisplayName: "Tax Country Code", Type: collectiondomain.FieldTypeString, DBType: "varchar(2)"},
				choiceField("tax_mode", "Tax Mode", "charge", "charge", "declare_external"),
				{Name: "shipping_address", DisplayName: "Shipping Address", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
				{Name: "billing_address", DisplayName: "Billing Address", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
				{Name: "placed_at", DisplayName: "Placed At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
			},
		},
		{
			Name: "ecommerce_order_items", DisplayName: "Order Items", Icon: "list", Color: "#fb923c", Hidden: true,
			Fields: []FieldBlueprint{
				{Name: "order_id", DisplayName: "Order", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "product_id", DisplayName: "Product", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "variant_id", DisplayName: "Variant", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "sku", DisplayName: "SKU", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "quantity", DisplayName: "Quantity", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 1},
				money("unit_price", "Unit Price", true), money("subtotal", "Subtotal", true),
				{Name: "options", DisplayName: "Options", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
			},
		},
		{
			Name: "ecommerce_order_transactions", DisplayName: "Order Transactions", Icon: "landmark", Color: "#eab308", Hidden: true,
			Fields: []FieldBlueprint{
				{Name: "order_id", DisplayName: "Order", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "payment_method_id", DisplayName: "Payment Method", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				statusField("pending", "pending", "authorized", "captured", "failed", "refunded"),
				money("amount", "Amount", true), currencyField(),
				{Name: "provider_reference", DisplayName: "Provider Reference", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				{Name: "data", DisplayName: "Data", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
			},
		},
		{
			Name: "ecommerce_order_shipments", DisplayName: "Order Shipments", Icon: "package-check", Color: "#22c55e", Hidden: true,
			Fields: []FieldBlueprint{
				{Name: "order_id", DisplayName: "Order", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "shipment_method_id", DisplayName: "Shipment Method", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				statusField("pending", "pending", "packed", "shipped", "delivered", "returned", "cancelled"),
				{Name: "tracking_number", DisplayName: "Tracking Number", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				{Name: "tracking_url", DisplayName: "Tracking URL", Type: collectiondomain.FieldTypeString, DBType: "text"},
				{Name: "shipped_at", DisplayName: "Shipped At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
				{Name: "delivered_at", DisplayName: "Delivered At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
			},
		},
	}
}

func methodCollection(name, displayName, icon, color string) Blueprint {
	return Blueprint{Name: name, DisplayName: displayName, Icon: icon, Color: color, Fields: []FieldBlueprint{
		{Name: "name", DisplayName: "Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
		{Name: "code", DisplayName: "Code", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
		{Name: "provider", DisplayName: "Provider", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
		{Name: "active", DisplayName: "Active", Type: collectiondomain.FieldTypeBoolean, DBType: "boolean", Required: true, DefaultValue: true},
		{Name: "config", DisplayName: "Config", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
		{Name: "sort", DisplayName: "Sort", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 0},
	}}
}

func currencyField() FieldBlueprint {
	return FieldBlueprint{Name: "currency", DisplayName: "Currency", Type: collectiondomain.FieldTypeString, DBType: "varchar(3)", Required: true, DefaultValue: "USD"}
}

func statusField(defaultValue string, values ...string) FieldBlueprint {
	return choiceField("status", "Status", defaultValue, values...)
}

func choiceField(name, displayName, defaultValue string, values ...string) FieldBlueprint {
	choices := make([]map[string]string, 0, len(values))
	for _, value := range values {
		choices = append(choices, map[string]string{"value": value, "label": choiceLabel(value)})
	}
	return FieldBlueprint{Name: name, DisplayName: displayName, Type: collectiondomain.FieldTypeSelect, DBType: "varchar(50)", Required: true, DefaultValue: defaultValue, Options: map[string]any{"choices": choices}}
}

func choiceLabel(value string) string {
	label := ""
	upper := true
	for _, r := range value {
		if r == '_' || r == '-' {
			label += " "
			upper = true
			continue
		}
		if upper && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		label += string(r)
		upper = false
	}
	return label
}

func ecommerceRelations() []RelationSpec {
	m2o := collectiondomain.RelationManyToOne
	return []RelationSpec{
		{CollectionMany: "ecommerce_carts", FieldMany: "customer_id", CollectionOne: "ecommerce_customers", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_cart_items", FieldMany: "cart_id", CollectionOne: "ecommerce_carts", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_cart_items", FieldMany: "product_id", CollectionOne: "ecommerce_products", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_cart_items", FieldMany: "variant_id", CollectionOne: "ecommerce_product_variants", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_product_categories", FieldMany: "parent_id", CollectionOne: "ecommerce_product_categories", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_products", FieldMany: "category_id", CollectionOne: "ecommerce_product_categories", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_product_variants", FieldMany: "product_id", CollectionOne: "ecommerce_products", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_inventory_levels", FieldMany: "location_id", CollectionOne: "ecommerce_locations", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_inventory_levels", FieldMany: "product_id", CollectionOne: "ecommerce_products", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_inventory_levels", FieldMany: "variant_id", CollectionOne: "ecommerce_product_variants", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_orders", FieldMany: "customer_id", CollectionOne: "ecommerce_customers", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_orders", FieldMany: "payment_method_id", CollectionOne: "ecommerce_payment_methods", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "ecommerce_orders", FieldMany: "shipment_method_id", CollectionOne: "ecommerce_shipment_methods", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "ecommerce_orders", FieldMany: "tax_method_id", CollectionOne: "ecommerce_tax_methods", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "ecommerce_order_items", FieldMany: "order_id", CollectionOne: "ecommerce_orders", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_order_items", FieldMany: "product_id", CollectionOne: "ecommerce_products", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_order_items", FieldMany: "variant_id", CollectionOne: "ecommerce_product_variants", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_order_transactions", FieldMany: "order_id", CollectionOne: "ecommerce_orders", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_order_transactions", FieldMany: "payment_method_id", CollectionOne: "ecommerce_payment_methods", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "ecommerce_order_shipments", FieldMany: "order_id", CollectionOne: "ecommerce_orders", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "ecommerce_order_shipments", FieldMany: "shipment_method_id", CollectionOne: "ecommerce_shipment_methods", Type: m2o, OnDelete: "set_null"},
	}
}
