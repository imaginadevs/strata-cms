package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/crypto"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// presignTTL is how long a private file's download URL stays valid.
const presignTTL = time.Hour

// maxMultipartMemory is how much of an upload is buffered in RAM before spilling
// to a temp file. The hard size cap is cfg.Storage.MaxUploadBytes.
const maxMultipartMemory = 16 << 20 // 16 MiB

var safeCollectionRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type fileHandler struct {
	repos   *postgresrepo.Repos
	factory outbound.StorageFactory
	cfg     *config.Config
	enc     *crypto.Encryptor
	log     *logger.Logger
}

// MountFiles wires the file upload/serve API. It is mounted in the protected
// (JWT-required) group — the admin CMS uploads and previews files here. Public
// website delivery of files flagged is_public happens through the CDN URL
// returned on each file (no auth), not through these endpoints.
func MountFiles(r chi.Router, repos *postgresrepo.Repos, factory outbound.StorageFactory, cfg *config.Config, enc *crypto.Encryptor, log *logger.Logger) {
	h := &fileHandler{repos: repos, factory: factory, cfg: cfg, enc: enc, log: log}
	r.Route("/files", func(r chi.Router) {
		r.Post("/", h.upload)
		r.Get("/", h.list)
		r.Get("/{id}", h.meta)
		r.Patch("/{id}", h.setPublic)
		r.Delete("/{id}", h.delete)
	})
}

// resolvedStorage is the storage backend chosen for the current request: either
// the tenant's own S3 or the environment/default S3.
type resolvedStorage struct {
	adapter   outbound.StorageAdapter
	bucket    string // for CDN URL construction
	cdnBase   string // CDN origin for public files ("" → presign public too)
	keyPrefix string // extra prefix inside the bucket (default bucket is shared across apps)
}

// resolve picks the storage backend for the request's tenant. A tenant with its
// own complete S3 credentials (and Inherit=false) uses them; otherwise it falls
// back to the environment/default S3 under tenants/{tenant_id}/… .
func (h *fileHandler) resolve(r *http.Request) (*resolvedStorage, error) {
	if t, ok := httpMiddleware.TenantFromCtx(r.Context()); ok && t != nil && t.Settings.Storage.UseOwn() {
		st := t.Settings.Storage
		a, err := h.factory.For(outbound.StorageConfig{
			Endpoint:       st.Endpoint,
			Region:         st.Region,
			Bucket:         st.Bucket,
			AccessKeyID:    st.AccessKeyID,
			SecretKey:      h.enc.Decrypt(st.SecretKey), // stored encrypted at rest
			PublicBaseURL:  st.PublicBaseURL,
			ForcePathStyle: st.ForcePathStyle,
		})
		if err != nil {
			return nil, err
		}
		return &resolvedStorage{adapter: a, bucket: st.Bucket, cdnBase: st.CDNBaseURL, keyPrefix: ""}, nil
	}

	// Environment / default S3.
	if strings.TrimSpace(h.cfg.Storage.Bucket) == "" {
		return nil, fmt.Errorf("object storage is not configured")
	}
	a, err := h.factory.For(outbound.StorageConfig{
		Endpoint:       h.cfg.Storage.Endpoint,
		Region:         h.cfg.Storage.Region,
		Bucket:         h.cfg.Storage.Bucket,
		AccessKeyID:    h.cfg.Storage.AccessKeyID,
		SecretKey:      h.cfg.Storage.SecretKey,
		PublicBaseURL:  h.cfg.Storage.PublicBaseURL,
		ForcePathStyle: h.cfg.Storage.ForcePathStyle,
	})
	if err != nil {
		return nil, err
	}
	return &resolvedStorage{
		adapter:   a,
		bucket:    h.cfg.Storage.Bucket,
		cdnBase:   h.cfg.Storage.CDNBaseURL,
		keyPrefix: strings.Trim(h.cfg.Storage.KeyPrefix, "/"),
	}, nil
}

// resolveURL computes a usable URL for a stored file:
//   - public files → CDN URL (edge-cached) when a CDN is configured
//   - everything else → short-lived presigned URL (private bucket)
func (h *fileHandler) resolveURL(ctx context.Context, rs *resolvedStorage, f *outbound.FileObject) string {
	if f.IsPublic && rs.cdnBase != "" {
		base := strings.TrimRight(rs.cdnBase, "/")
		return fmt.Sprintf("%s/%s/%s?originalPath=%s", base, rs.bucket, path.Base(f.StorageKey), url.QueryEscape(f.StorageKey))
	}
	signed, err := rs.adapter.PresignedURL(ctx, f.StorageKey, presignTTL)
	if err != nil {
		h.log.Warnw("presign failed", "err", err, "key", f.StorageKey)
		return ""
	}
	return signed
}

// POST /files — multipart upload (form field "file", optional "collection", "is_public")
func (h *fileHandler) upload(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	actorID, hasActor := httpMiddleware.UserIDFromCtx(r.Context())
	if !hasActor {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	rs, err := h.resolve(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	src, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer src.Close()

	size := header.Size
	if max := h.cfg.Storage.MaxUploadBytes; max > 0 && size > max {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("file exceeds the %d MB limit", max/(1024*1024)))
		return
	}

	// Quota check (0 = unlimited). Tenant is already loaded into the context.
	if t, ok := httpMiddleware.TenantFromCtx(r.Context()); ok && t.StorageQuotaBytes > 0 {
		if t.StorageUsedBytes+size > t.StorageQuotaBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
			return
		}
	}

	coll := strings.TrimSpace(r.FormValue("collection"))
	if coll == "" {
		coll = "uploads"
	}
	if !safeCollectionRe.MatchString(coll) {
		writeError(w, http.StatusBadRequest, "invalid collection name")
		return
	}
	isPublic := r.FormValue("is_public") == "true"

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id := uuid.New()
	ext := strings.ToLower(path.Ext(header.Filename))
	// Always namespaced by tenant (tenants/{tenant_id}/…); the default shared
	// bucket adds an app keyPrefix on top.
	key := outbound.StorageKey(tenantID.String(), coll, id.String(), ext)
	if rs.keyPrefix != "" {
		key = rs.keyPrefix + "/" + key
	}

	if _, err := rs.adapter.Upload(r.Context(), key, src, size, contentType); err != nil {
		h.log.Errorw("file upload failed", "err", err, "key", key)
		writeError(w, http.StatusBadGateway, "upload to storage failed")
		return
	}

	f := &outbound.FileObject{
		ID:          id,
		TenantID:    tenantID,
		Filename:    header.Filename,
		StorageKey:  key,
		ContentType: contentType,
		Size:        size,
		Collection:  coll,
		IsPublic:    isPublic,
		UploadedBy:  &actorID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.repos.File.Create(r.Context(), f); err != nil {
		// Roll back the stored object so we don't leak orphaned bytes.
		_ = rs.adapter.Delete(r.Context(), key)
		h.log.Errorw("file metadata insert failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save file metadata")
		return
	}

	if err := h.repos.Tenant.AddStorageUsed(r.Context(), tenantID, size); err != nil {
		h.log.Warnw("storage counter increment failed", "err", err, "tenant", tenantID)
	}

	f.URL = h.resolveURL(r.Context(), rs, f)
	writeJSON(w, http.StatusCreated, f)
}

// GET /files — paginated list for the media library
func (h *fileHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	page, limit := parsePage(r)

	rs, err := h.resolve(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	files, total, err := h.repos.File.List(r.Context(), tenantID, page, limit)
	if err != nil {
		h.log.Errorw("file list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	for _, f := range files {
		f.URL = h.resolveURL(r.Context(), rs, f)
	}
	paginated(w, files, total, page, limit)
}

// GET /files/{id} — metadata + a ready-to-use URL (presigned or CDN)
func (h *fileHandler) meta(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}
	rs, err := h.resolve(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	f, err := h.repos.File.GetByID(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	f.URL = h.resolveURL(r.Context(), rs, f)
	writeJSON(w, http.StatusOK, f)
}

// PATCH /files/{id} — toggle public/private (changes how the file is served)
func (h *fileHandler) setPublic(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}
	var body struct {
		IsPublic bool `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.repos.File.SetPublic(r.Context(), tenantID, id, body.IsPublic); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	h.meta(w, r)
}

// DELETE /files/{id} — remove from storage + metadata, release quota
func (h *fileHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}
	rs, err := h.resolve(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	f, err := h.repos.File.GetByID(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err := rs.adapter.Delete(r.Context(), f.StorageKey); err != nil {
		h.log.Warnw("storage delete failed", "err", err, "key", f.StorageKey)
	}
	if err := h.repos.File.Delete(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if err := h.repos.Tenant.AddStorageUsed(r.Context(), tenantID, -f.Size); err != nil {
		h.log.Warnw("storage counter release failed", "err", err, "tenant", tenantID)
	}
	w.WriteHeader(http.StatusNoContent)
}
