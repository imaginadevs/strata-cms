package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// fileRepository persists file metadata in the per-tenant `files` table.
// Binary content lives in object storage (S3/Wasabi); only metadata is stored here.
type fileRepository struct{ db *pgxpool.Pool }

func (r *fileRepository) Create(ctx context.Context, f *outbound.FileObject) error {
	s := schemaOf(f.TenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.files
		    (id, name, mime_type, size_bytes, storage_path, is_public, collection, uploaded_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s),
		f.ID, f.Filename, f.ContentType, f.Size, f.StorageKey,
		f.IsPublic, f.Collection, f.UploadedBy, f.CreatedAt,
	)
	return err
}

func (r *fileRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*outbound.FileObject, error) {
	s := schemaOf(tenantID)
	f := &outbound.FileObject{TenantID: tenantID}
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, name, mime_type, size_bytes, storage_path,
		       is_public, COALESCE(collection, ''), uploaded_by, created_at
		FROM %s.files WHERE id = $1`, s), id).
		Scan(&f.ID, &f.Filename, &f.ContentType, &f.Size, &f.StorageKey,
			&f.IsPublic, &f.Collection, &f.UploadedBy, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *fileRepository) List(ctx context.Context, tenantID uuid.UUID, page, limit int) ([]*outbound.FileObject, int64, error) {
	s := schemaOf(tenantID)
	page, limit = normalisePage(page, limit)
	offset := (page - 1) * limit

	var total int64
	if err := r.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s.files`, s)).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, name, mime_type, size_bytes, storage_path,
		       is_public, COALESCE(collection, ''), uploaded_by, created_at
		FROM %s.files
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, s), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*outbound.FileObject
	for rows.Next() {
		f := &outbound.FileObject{TenantID: tenantID}
		if err := rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.Size, &f.StorageKey,
			&f.IsPublic, &f.Collection, &f.UploadedBy, &f.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, f)
	}
	return out, total, rows.Err()
}

func (r *fileRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.files WHERE id = $1`, s), id)
	return err
}

func (r *fileRepository) SetPublic(ctx context.Context, tenantID, id uuid.UUID, isPublic bool) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`UPDATE %s.files SET is_public = $1 WHERE id = $2`, s), isPublic, id)
	return err
}
