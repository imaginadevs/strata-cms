-- +goose Up
-- +goose StatementBegin
-- The platform tenant is the single "first" tenant that owns the platform
-- super-admin account. Everything that manages OTHER tenants is done from here.
ALTER TABLE public.tenants
    ADD COLUMN IF NOT EXISTS is_platform BOOLEAN NOT NULL DEFAULT FALSE;

-- Flag the bootstrap 'default' tenant (created in 001) as the platform tenant.
-- Bootstrap falls back to the oldest tenant / creates one if this is missing.
UPDATE public.tenants SET is_platform = TRUE WHERE slug = 'default';

-- Enforce at most ONE platform tenant: a partial unique index over the rows
-- where is_platform is true (they all share the value, so only one may exist).
CREATE UNIQUE INDEX IF NOT EXISTS uq_tenants_single_platform
    ON public.tenants ((is_platform)) WHERE is_platform;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS uq_tenants_single_platform;
ALTER TABLE public.tenants DROP COLUMN IF EXISTS is_platform;
-- +goose StatementEnd
