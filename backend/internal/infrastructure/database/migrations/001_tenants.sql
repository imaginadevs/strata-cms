-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.tenants (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT        NOT NULL,
    slug                TEXT        NOT NULL UNIQUE,
    default_language    VARCHAR(10) NOT NULL DEFAULT 'en',
    logo                TEXT,
    primary_color       VARCHAR(7),
    domain              TEXT        UNIQUE,
    active              BOOLEAN     NOT NULL DEFAULT TRUE,
    settings            JSONB       NOT NULL DEFAULT '{}',
    storage_used_bytes  BIGINT      NOT NULL DEFAULT 0,
    storage_quota_bytes BIGINT      NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION update_updated_at_public()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$;

CREATE OR REPLACE TRIGGER trg_tenants_updated_at
    BEFORE UPDATE ON public.tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_public();

-- Default tenant — domain localhost resolves in local development.
-- ON CONFLICT ensures this is idempotent on every migration run.
INSERT INTO public.tenants (name, slug, domain, default_language, settings)
VALUES (
    'Default',
    'default',
    'localhost',
    'en',
    '{"allow_registration": false, "timezone": "UTC", "date_format": "YYYY-MM-DD", "features": {}, "languages": ["en"]}'::jsonb
)
ON CONFLICT (slug) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS public.tenants;
-- +goose StatementEnd
