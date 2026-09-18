// Package tenants holds optional tenant-scoped ecommerce calculator registrations.
//
// Add one Go file per tenant exception, register with ecommerce.Register and a
// fixed tenant UUID, then ensure this package is imported from server wiring
// (see internal/adapters/http/router.go). Do not evaluate tenant-supplied source.
package tenants