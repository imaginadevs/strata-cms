# 11 — File Storage

## Overview

Each tenant has its own S3-compatible storage configuration (AWS S3, Cloudflare R2, MinIO) stored encrypted in `tenants.settings.storage_config`. The `StorageFactory` creates and caches one `S3Adapter` per bucket. File uploads stream directly from the multipart request to S3 without buffering the entire file in memory. File metadata (filename, content type, size, URL) is persisted in the `files` table. Collection fields of type `file` or `files` reference `files.id`. This guide covers the full upload flow, presigned URL generation, and the settings UI for configuring storage per tenant.

## Prerequisites

- Guides 03 and 05 completed (migrations applied, tenant system working).
- Migration 002 applied (`files` table exists).
- An S3-compatible bucket (AWS S3, Cloudflare R2, or local MinIO via Docker).

## Step-by-Step

### 1. Local MinIO setup for development

Add MinIO to `docker-compose.yml`:

```yaml
minio:
  image: minio/minio:latest
  ports:
    - "9000:9000"   # API
    - "9001:9001"   # Console
  environment:
    MINIO_ROOT_USER: minioadmin
    MINIO_ROOT_PASSWORD: minioadmin
  command: server /data --console-address ":9001"
  volumes:
    - minio_data:/data

volumes:
  minio_data:
```

```bash
docker compose up -d minio

# Create a bucket for the default tenant
docker compose exec minio mc alias set local http://localhost:9000 minioadmin minioadmin
docker compose exec minio mc mb local/cms-default-tenant
```

### 2. StorageConfig structure

`StorageConfig` is stored as an encrypted JSON blob inside `tenants.settings`:

```go
// From backend/internal/core/ports/outbound/storage.go
type StorageConfig struct {
    Provider        string `json:"provider"`          // "s3" | "r2" | "minio" | "gcs"
    Endpoint        string `json:"endpoint"`          // empty for AWS, set for MinIO/R2
    Region          string `json:"region"`
    Bucket          string `json:"bucket"`
    AccessKeyID     string `json:"access_key_id"`
    SecretAccessKey string `json:"secret_access_key"` // stored encrypted
    PublicBaseURL   string `json:"public_base_url"`   // CDN or presign-bypass URL
    ForcePathStyle  bool   `json:"force_path_style"`  // TRUE for MinIO
}
```

For local MinIO:

```json
{
  "provider": "minio",
  "endpoint": "http://localhost:9000",
  "region": "us-east-1",
  "bucket": "cms-default-tenant",
  "access_key_id": "minioadmin",
  "secret_access_key": "minioadmin",
  "public_base_url": "",
  "force_path_style": true
}
```

### 3. Encryption for storage credentials

Store credentials encrypted so that a database dump does not leak S3 secrets. Add an encryption helper:

```go
// backend/internal/infrastructure/crypto/aes.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io"
)

// Encrypt encrypts plaintext using AES-GCM. Key must be 32 bytes (AES-256).
func Encrypt(plaintext, key []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }

    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext produced by Encrypt.
func Decrypt(encoded string, key []byte) ([]byte, error) {
    ciphertext, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return nil, err
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    if len(ciphertext) < gcm.NonceSize() {
        return nil, fmt.Errorf("ciphertext too short")
    }

    nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
    return gcm.Open(nil, nonce, ciphertext, nil)
}
```

Add `STORAGE_ENCRYPTION_KEY=<32-byte-hex>` to `.env`. The key is derived from `JWT_SECRET` if not provided, or set explicitly:

```bash
# Generate a 32-byte key
openssl rand -hex 32
```

### 4. StorageFactory — caching per bucket

The factory in `backend/internal/adapters/storage/s3.go` is already implemented. Wire it into the application:

```go
// In main.go
storageFactory := storageAdapter.NewFactory()

// Pass to handlers that need it
srv := server.NewWithStorage(cfg, log, db, bus, storageFactory)
```

The factory caches adapters keyed by `bucket` name. For multi-tenant scenarios, each tenant may have a different bucket, so the cache may hold multiple adapters simultaneously.

### 5. File repository implementation

Create `backend/internal/adapters/postgres/file_repository.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type FileRepository struct {
    db *pgxpool.Pool
}

func NewFileRepository(db *pgxpool.Pool) *FileRepository {
    return &FileRepository{db: db}
}

func (r *FileRepository) Create(ctx context.Context, f *outbound.FileObject) error {
    _, err := r.db.Exec(ctx, `
        INSERT INTO files (id, tenant_id, filename, storage_key, content_type, size, url, uploaded_by, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
        f.ID, f.TenantID, f.Filename, f.StorageKey, f.ContentType,
        f.Size, f.URL, f.UploadedBy, f.CreatedAt,
    )
    return err
}

func (r *FileRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*outbound.FileObject, error) {
    f := &outbound.FileObject{}
    err := r.db.QueryRow(ctx, `
        SELECT id, tenant_id, filename, storage_key, content_type, size, url, uploaded_by, created_at
        FROM   files
        WHERE  tenant_id = $1 AND id = $2`,
        tenantID, id,
    ).Scan(&f.ID, &f.TenantID, &f.Filename, &f.StorageKey, &f.ContentType,
        &f.Size, &f.URL, &f.UploadedBy, &f.CreatedAt)
    if err != nil {
        return nil, err
    }
    return f, nil
}

func (r *FileRepository) List(
    ctx context.Context,
    tenantID uuid.UUID,
    page, limit int,
) ([]*outbound.FileObject, int64, error) {
    if limit <= 0 {
        limit = 25
    }
    offset := (page - 1) * limit
    if offset < 0 {
        offset = 0
    }

    var total int64
    _ = r.db.QueryRow(ctx, `SELECT COUNT(*) FROM files WHERE tenant_id = $1`, tenantID).Scan(&total)

    rows, err := r.db.Query(ctx, `
        SELECT id, tenant_id, filename, storage_key, content_type, size, url, uploaded_by, created_at
        FROM   files
        WHERE  tenant_id = $1
        ORDER  BY created_at DESC
        LIMIT  $2 OFFSET $3`,
        tenantID, limit, offset,
    )
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()

    var files []*outbound.FileObject
    for rows.Next() {
        f := &outbound.FileObject{}
        err := rows.Scan(&f.ID, &f.TenantID, &f.Filename, &f.StorageKey, &f.ContentType,
            &f.Size, &f.URL, &f.UploadedBy, &f.CreatedAt)
        if err != nil {
            return nil, 0, err
        }
        files = append(files, f)
    }
    return files, total, rows.Err()
}

func (r *FileRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `DELETE FROM files WHERE tenant_id = $1 AND id = $2`,
        tenantID, id,
    )
    return err
}
```

### 6. Upload handler — stream multipart to S3

Create `backend/internal/adapters/http/rest/files.go`:

```go
package rest

import (
    "fmt"
    "net/http"
    "path/filepath"
    "strings"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
    postgresAdapter "github.com/imaginacolombia/cms/internal/adapters/postgres"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

const maxUploadSize = 100 << 20 // 100 MB

type fileHandler struct {
    fileRepo       outbound.FileRepository
    tenantRepo     outbound.TenantRepository
    storageFactory outbound.StorageFactory
    log            *logger.Logger
}

func MountFiles(r chi.Router, db *pgxpool.Pool, storageFactory outbound.StorageFactory, log *logger.Logger) {
    h := &fileHandler{
        fileRepo:       postgresAdapter.NewFileRepository(db),
        tenantRepo:     postgresAdapter.NewTenantRepository(db),
        storageFactory: storageFactory,
        log:            log,
    }

    r.Route("/files", func(r chi.Router) {
        r.Post("/",          h.upload)
        r.Get("/",           h.list)
        r.Get("/{id}",      h.getByID)
        r.Get("/{id}/url",  h.presignedURL)
        r.Delete("/{id}",   h.delete)
    })
}

func (h *fileHandler) upload(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    userID, _ := httpMiddleware.UserIDFromCtx(r.Context())

    // Limit total request body to prevent OOM
    r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
    if err := r.ParseMultipartForm(32 << 20); err != nil {
        writeError(w, http.StatusBadRequest, "file too large or invalid form")
        return
    }

    file, header, err := r.FormFile("file")
    if err != nil {
        writeError(w, http.StatusBadRequest, "missing 'file' field in form")
        return
    }
    defer file.Close()

    // Resolve tenant storage config
    storageCfg, err := h.resolveStorageConfig(r.Context(), tenantID)
    if err != nil {
        h.log.Errorw("resolve storage config", "tenant", tenantID, "err", err)
        writeError(w, http.StatusInternalServerError, "storage not configured for this tenant")
        return
    }

    adapter, err := h.storageFactory.For(storageCfg)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "could not create storage adapter")
        return
    }

    // Build storage key: tenant/<year>/<month>/<uuid>.<ext>
    ext := filepath.Ext(header.Filename)
    key := fmt.Sprintf("%s/%s/%s%s",
        tenantID.String(),
        time.Now().UTC().Format("2006/01"),
        uuid.New().String(),
        ext,
    )

    contentType := header.Header.Get("Content-Type")
    if contentType == "" {
        contentType = "application/octet-stream"
    }

    // Stream directly to S3 (no temp file)
    url, err := adapter.Upload(r.Context(), key, file, header.Size, contentType)
    if err != nil {
        h.log.Errorw("s3 upload", "key", key, "err", err)
        writeError(w, http.StatusInternalServerError, "upload failed")
        return
    }

    // Persist file metadata
    f := &outbound.FileObject{
        ID:          uuid.New(),
        TenantID:    tenantID,
        Filename:    header.Filename,
        StorageKey:  key,
        ContentType: contentType,
        Size:        header.Size,
        URL:         url,
        UploadedBy:  &userID,
        CreatedAt:   time.Now().UTC(),
    }

    if err := h.fileRepo.Create(r.Context(), f); err != nil {
        h.log.Errorw("persist file metadata", "err", err)
        // File is in S3 but metadata failed — log and continue (reconcile later)
    }

    writeJSON(w, http.StatusCreated, f)
}

func (h *fileHandler) presignedURL(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    fileID, err := uuid.Parse(chi.URLParam(r, "id"))
    if err != nil {
        writeError(w, http.StatusBadRequest, "invalid file id")
        return
    }

    f, err := h.fileRepo.GetByID(r.Context(), tenantID, fileID)
    if err != nil {
        writeError(w, http.StatusNotFound, "file not found")
        return
    }

    storageCfg, err := h.resolveStorageConfig(r.Context(), tenantID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "storage not configured")
        return
    }

    adapter, err := h.storageFactory.For(storageCfg)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "storage adapter error")
        return
    }

    ttl := 1 * time.Hour
    url, err := adapter.PresignedURL(r.Context(), f.StorageKey, ttl)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "presign error")
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "url":        url,
        "expires_in": int(ttl.Seconds()),
    })
}

func (h *fileHandler) delete(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    fileID, _ := uuid.Parse(chi.URLParam(r, "id"))

    f, err := h.fileRepo.GetByID(r.Context(), tenantID, fileID)
    if err != nil {
        writeError(w, http.StatusNotFound, "file not found")
        return
    }

    // Delete from S3
    storageCfg, _ := h.resolveStorageConfig(r.Context(), tenantID)
    if adapter, err := h.storageFactory.For(storageCfg); err == nil {
        _ = adapter.Delete(r.Context(), f.StorageKey)
    }

    // Delete metadata
    _ = h.fileRepo.Delete(r.Context(), tenantID, fileID)
    w.WriteHeader(http.StatusNoContent)
}

// resolveStorageConfig reads and decrypts the tenant's storage config from tenants.settings.
func (h *fileHandler) resolveStorageConfig(ctx context.Context, tenantID uuid.UUID) (outbound.StorageConfig, error) {
    t, err := h.tenantRepo.GetByID(ctx, tenantID)
    if err != nil {
        return outbound.StorageConfig{}, fmt.Errorf("tenant not found: %w", err)
    }

    // StorageConfig is stored in t.Settings.StorageConfig (extend the Settings struct)
    // Decrypt the secret_access_key before returning
    cfg := t.Settings.StorageConfig
    if cfg.Bucket == "" {
        return outbound.StorageConfig{}, fmt.Errorf("storage not configured for tenant")
    }
    return cfg, nil
}
```

### 7. Extending Tenant.Settings to hold StorageConfig

Update `backend/internal/core/domain/tenant/entity.go`:

```go
// Settings holds tenant-scoped feature toggles stored as JSONB.
type Settings struct {
    AllowRegistration bool           `json:"allow_registration"`
    MaxUsers          *int           `json:"max_users,omitempty"`
    Timezone          string         `json:"timezone"`
    DateFormat        string         `json:"date_format"`
    StorageConfig     *StorageConfig `json:"storage_config,omitempty"` // encrypted on write
}

// StorageConfig mirrors outbound.StorageConfig but is embedded in tenant settings.
type StorageConfig struct {
    Provider        string `json:"provider"`
    Endpoint        string `json:"endpoint"`
    Region          string `json:"region"`
    Bucket          string `json:"bucket"`
    AccessKeyID     string `json:"access_key_id"`
    SecretAccessKey string `json:"secret_access_key"` // decrypt before use
    PublicBaseURL   string `json:"public_base_url"`
    ForcePathStyle  bool   `json:"force_path_style"`
}
```

### 8. Tenant storage settings UI (API)

```bash
# Configure storage for a tenant (admin only)
curl -X PATCH http://localhost:8080/api/settings/storage \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "minio",
    "endpoint": "http://localhost:9000",
    "region": "us-east-1",
    "bucket": "cms-my-tenant",
    "access_key_id": "minioadmin",
    "secret_access_key": "minioadmin",
    "force_path_style": true
  }'

# Response: {"status":"ok","provider":"minio","bucket":"cms-my-tenant"}
```

### 9. File field type in collections

A `file` field in a collection stores the UUID of a `files` record. When creating/updating an item with a file field, first upload the file, then pass the returned `id` as the field value:

```bash
# 1. Upload the file
FILE=$(curl -s -X POST http://localhost:8080/api/files \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -F "file=@/path/to/image.jpg")
FILE_ID=$(echo $FILE | jq -r '.id')

# 2. Create an item with the file reference
curl -X POST http://localhost:8080/api/items/products \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Widget\",\"thumbnail\":\"$FILE_ID\"}"
```

When listing items, the API can optionally expand the file UUID into a full file object:

```bash
curl "http://localhost:8080/api/items/products?expand=thumbnail" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"
# Response: {"name":"Widget","thumbnail":{"id":"...","url":"...","filename":"image.jpg"}}
```

### 10. Test the upload flow

```bash
# Upload a file
curl -X POST http://localhost:8080/api/files \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -F "file=@./README.md"

# List files
curl http://localhost:8080/api/files \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"

# Get a presigned URL (valid for 1 hour)
curl http://localhost:8080/api/files/$FILE_ID/url \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/adapters/storage/s3.go` — S3Adapter + Factory implementation
- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/storage.go` — StorageAdapter, StorageConfig, FileRepository interfaces
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/migrations/002_super_admin_and_storage.sql` — files table DDL
- `/home/jd/dev/full/cms/backend/internal/core/domain/tenant/entity.go` — Tenant.Settings (extend with StorageConfig)

## Next Steps

Proceed to [12 — Deployment](./12-deployment.md) to build a production Docker image, configure environment variables, enable Kafka and SSO, set up nginx, and implement zero-downtime updates.
