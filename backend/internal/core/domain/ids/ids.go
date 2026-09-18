// Package ids centralises entity identifier generation.
//
// Primary keys use UUIDv7, which is time-ordered: the leading bits are a
// Unix-millisecond timestamp, so freshly minted ids sort roughly by creation
// time. That keeps Postgres B-tree inserts near the right edge of the index
// instead of scattering randomly like UUIDv4, reducing page splits and index
// bloat on the high-insert multitenant collection tables.
//
// Do NOT use these for security tokens (session/refresh/reset): a v7 id leaks
// its creation time and is partially structured. Use crypto/rand for secrets.
package ids

import "github.com/google/uuid"

// New returns a time-ordered UUIDv7 for use as an entity primary key.
// uuid.NewV7 only errors if the system RNG is unavailable — fatal anyway — so
// Must is appropriate here.
func New() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}
