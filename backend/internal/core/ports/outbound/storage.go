package outbound

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

// StorageConfig holds S3-compatible credentials.
//
// Default (global): one bucket for all tenants, files stored under
//   tenants/{tenant_id}/{collection}/{uuid}.ext
//
// S3 internal sharding is by key-prefix hash — prefix-per-tenant paths
// perform identically to separate buckets. Using one bucket avoids AWS's
// 100-bucket soft limit and eliminates per-tenant provisioning.
//
// Override (per-tenant): tenants may configure their own bucket in settings
// (stored encrypted in tenants.settings.storage_config). The StorageFactory
// uses the tenant override when present, otherwise falls back to global config.
type StorageConfig struct {
	Provider       string `json:"provider"`         // "s3" | "r2" | "minio"
	Endpoint       string `json:"endpoint"`         // custom endpoint (MinIO, R2)
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	AccessKeyID    string `json:"access_key_id"`
	SecretKey      string `json:"secret_key"`       // stored encrypted when per-tenant
	PublicBaseURL  string `json:"public_base_url"`  // CDN or public URL prefix
	ForcePathStyle bool   `json:"force_path_style"` // required for MinIO
}

// StorageKey returns the canonical S3 key for a file.
// Format: tenants/{tenant_id}/{collection}/{uuid}.{ext}
func StorageKey(tenantID, collection, uuid, ext string) string {
	if ext != "" && ext[0] != '.' {
		ext = "." + ext
	}
	return "tenants/" + tenantID + "/" + collection + "/" + uuid + ext
}

// FileObject represents a stored file's metadata (not the binary content).
type FileObject struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Filename     string    `json:"filename"`
	StorageKey   string    `json:"storage_key"`  // S3 key
	ContentType  string    `json:"content_type"`
	Size         int64     `json:"size"`
	URL          string    `json:"url"`
	UploadedBy   *uuid.UUID `json:"uploaded_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// StorageAdapter is the port that the storage adapter must implement.
// There is one adapter per tenant (keyed by tenant ID) holding its own credentials.
type StorageAdapter interface {
	// Upload streams a file to the bucket; returns the storage key and public URL.
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (url string, err error)

	// Download returns a reader for the object identified by key.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes an object by key.
	Delete(ctx context.Context, key string) error

	// PresignedURL generates a time-limited download URL for private buckets.
	PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// Exists checks whether a key exists without downloading.
	Exists(ctx context.Context, key string) (bool, error)
}

// StorageFactory creates or returns a cached StorageAdapter for a given tenant config.
type StorageFactory interface {
	For(cfg StorageConfig) (StorageAdapter, error)
}

// FileRepository persists file metadata records (not the binaries).
type FileRepository interface {
	Create(ctx context.Context, f *FileObject) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*FileObject, error)
	List(ctx context.Context, tenantID uuid.UUID, page, limit int) ([]*FileObject, int64, error)
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

// StorageQuota manages per-tenant storage accounting.
//
// Why DB tracking (not S3 metrics):
//   AWS S3 / compatible storage never reports per-prefix usage in real time.
//   CloudWatch bucket metrics update once daily — useless for upload-time enforcement.
//   DB counters give sub-millisecond quota checks with no extra S3 API calls.
//
// Enforcement pattern (inside a single DB transaction):
//   1. Call Claim(tenantID, fileSize)  — atomically checks quota and increments counter.
//      Returns ErrQuotaExceeded if the upload would exceed the limit.
//   2. On success: stream the file to S3.
//   3. On S3 error: call Release(tenantID, fileSize) to roll back the counter.
//   4. On file deletion: call Release(tenantID, fileSize).
//
// Reconciliation:
//   Query the storage_drift view to detect counter drift after crashes.
//   Run periodically (e.g. nightly cron) and call Reconcile to fix drift.
type StorageQuota interface {
	// Claim atomically checks and reserves space for an upload.
	// Returns ErrQuotaExceeded if tenant.storage_quota_bytes would be breached.
	// Uses the storage_claim() PG function — one round-trip, no TOCTOU race.
	Claim(ctx context.Context, tenantID uuid.UUID, bytes int64) error

	// Release decrements the counter after a deletion or a failed upload.
	// Safe to call even if bytes > current counter (floor at 0).
	Release(ctx context.Context, tenantID uuid.UUID, bytes int64) error

	// Usage returns (used, quota) in bytes. quota=0 means unlimited.
	Usage(ctx context.Context, tenantID uuid.UUID) (used, quota int64, err error)

	// Reconcile resets the counter to the actual SUM(files.size) for the tenant.
	// Call after detecting drift in the storage_drift view.
	Reconcile(ctx context.Context, tenantID uuid.UUID) error
}

// ErrQuotaExceeded is returned by StorageQuota.Claim when the upload would
// exceed the tenant's configured storage_quota_bytes.
var ErrQuotaExceeded = errors.New("storage quota exceeded")
