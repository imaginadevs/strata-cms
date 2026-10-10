package database

// tenantMigrationList is the ordered list of per-tenant schema migrations.
// Add new entries at the end — never change existing versions.
//
// Note: entities carry no tenant_id column — tenancy is enforced by the
// per-tenant schema, so uniqueness/indexes are naturally scoped to the tenant.
var tenantMigrationList = []tenantMigration{
	{
		version: 1,
		sql: `
-- Core tables: users, roles, user_roles, permissions, sessions
CREATE TABLE IF NOT EXISTS {schema}.users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL,
    first_name    TEXT        NOT NULL DEFAULT '',
    last_name     TEXT        NOT NULL DEFAULT '',
    avatar        TEXT,
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    provider      VARCHAR(20) NOT NULL DEFAULT 'local',
    provider_uid  TEXT,
    password_hash TEXT,
    is_super_admin BOOLEAN    NOT NULL DEFAULT FALSE,
    last_login    TIMESTAMPTZ,
    language      VARCHAR(10) NOT NULL DEFAULT 'en',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by    UUID,
    updated_by    UUID,
    UNIQUE (email)
);
CREATE INDEX IF NOT EXISTS users_email_idx    ON {schema}.users (email);
CREATE INDEX IF NOT EXISTS users_provider_idx ON {schema}.users (provider, provider_uid);

CREATE TABLE IF NOT EXISTS {schema}.roles (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    icon        TEXT,
    is_system   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    updated_by  UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS {schema}.user_roles (
    user_id   UUID NOT NULL REFERENCES {schema}.users(id) ON DELETE CASCADE,
    role_id   UUID NOT NULL REFERENCES {schema}.roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS {schema}.permissions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id     UUID        NOT NULL REFERENCES {schema}.roles(id) ON DELETE CASCADE,
    collection  TEXT        NOT NULL,
    action      VARCHAR(20) NOT NULL,
    fields      TEXT[]      NOT NULL DEFAULT '{}',
    filter      JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    updated_by  UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    UNIQUE (role_id, collection, action)
);

CREATE TABLE IF NOT EXISTS {schema}.sessions (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES {schema}.users(id) ON DELETE CASCADE,
    refresh_token TEXT        NOT NULL UNIQUE,
    expires_at    TIMESTAMPTZ NOT NULL,
    ip_address    INET,
    user_agent    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS sessions_token_idx ON {schema}.sessions (refresh_token);

-- Schema metadata tables
CREATE TABLE IF NOT EXISTS {schema}.collections (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    display_name TEXT        NOT NULL,
    type         VARCHAR(20) NOT NULL DEFAULT 'base'
                             CHECK (type IN ('base', 'auth')),
    icon         TEXT,
    color        VARCHAR(7),
    note         TEXT,
    is_system    BOOLEAN     NOT NULL DEFAULT FALSE,
    is_hidden    BOOLEAN     NOT NULL DEFAULT FALSE,
    is_singleton BOOLEAN     NOT NULL DEFAULT FALSE,
    sort_field   TEXT,
    meta         JSONB       NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by   UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    updated_by   UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    UNIQUE (name)
);
CREATE INDEX IF NOT EXISTS collections_name_idx ON {schema}.collections (name);

CREATE TABLE IF NOT EXISTS {schema}.fields (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_name TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    display_name    TEXT        NOT NULL,
    type            VARCHAR(50) NOT NULL,
    db_type         TEXT        NOT NULL,
    required        BOOLEAN     NOT NULL DEFAULT FALSE,
    unique_field    BOOLEAN     NOT NULL DEFAULT FALSE,
    readonly        BOOLEAN     NOT NULL DEFAULT FALSE,
    hidden          BOOLEAN     NOT NULL DEFAULT FALSE,
    translatable    BOOLEAN     NOT NULL DEFAULT FALSE,
    default_value   JSONB,
    validation      JSONB,
    options         JSONB       NOT NULL DEFAULT '{}',
    sort            INT         NOT NULL DEFAULT 0,
    is_system       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by      UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    updated_by      UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    UNIQUE (collection_name, name)
);
CREATE INDEX IF NOT EXISTS fields_collection_idx ON {schema}.fields (collection_name);

CREATE TABLE IF NOT EXISTS {schema}.relations (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_many TEXT        NOT NULL,
    field_many      TEXT        NOT NULL,
    collection_one  TEXT        NOT NULL,
    field_one       TEXT,
    type            VARCHAR(30) NOT NULL,
    junction_table  TEXT,
    on_delete       TEXT        NOT NULL DEFAULT 'set_null',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by      UUID REFERENCES {schema}.users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS {schema}.audit_logs (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        REFERENCES {schema}.users(id) ON DELETE SET NULL,
    user_email     TEXT,
    collection     TEXT        NOT NULL,
    item_id        TEXT,
    operation      VARCHAR(20) NOT NULL,
    old_data       JSONB,
    new_data       JSONB,
    changed_fields TEXT[]      NOT NULL DEFAULT '{}',
    ip_address     INET,
    user_agent     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS audit_tenant_idx ON {schema}.audit_logs (collection, created_at);

CREATE TABLE IF NOT EXISTS {schema}.files (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    mime_type    TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL DEFAULT 0,
    storage_path TEXT        NOT NULL,
    uploaded_by  UUID        REFERENCES {schema}.users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS {schema}.collection_indexes (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_name TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    type            TEXT        NOT NULL CHECK (type IN ('btree', 'fulltext')),
    fields          TEXT[]      NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by      UUID        REFERENCES {schema}.users(id) ON DELETE SET NULL,
    UNIQUE (name)
);
CREATE INDEX IF NOT EXISTS ci_collection_idx ON {schema}.collection_indexes (collection_name);

-- updated_at triggers for tables that need it
CREATE OR REPLACE FUNCTION {schema}.update_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$;

CREATE OR REPLACE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON {schema}.users FOR EACH ROW EXECUTE FUNCTION {schema}.update_updated_at();
CREATE OR REPLACE TRIGGER trg_roles_updated_at
    BEFORE UPDATE ON {schema}.roles FOR EACH ROW EXECUTE FUNCTION {schema}.update_updated_at();
CREATE OR REPLACE TRIGGER trg_collections_updated_at
    BEFORE UPDATE ON {schema}.collections FOR EACH ROW EXECUTE FUNCTION {schema}.update_updated_at();
CREATE OR REPLACE TRIGGER trg_fields_updated_at
    BEFORE UPDATE ON {schema}.fields FOR EACH ROW EXECUTE FUNCTION {schema}.update_updated_at();
CREATE OR REPLACE TRIGGER trg_relations_updated_at
    BEFORE UPDATE ON {schema}.relations FOR EACH ROW EXECUTE FUNCTION {schema}.update_updated_at();
`,
	},
	{
		version: 2,
		sql: `
-- Add quick_edit flag to fields: marks a field for the quick-edit aside panel.
ALTER TABLE {schema}.fields
    ADD COLUMN IF NOT EXISTS quick_edit BOOLEAN NOT NULL DEFAULT FALSE;
`,
	},
	{
		version: 3,
		sql: `
-- Sections: Collection -> Section -> Field (strict ownership)
CREATE TABLE IF NOT EXISTS {schema}.sections (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_name TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    display_name    TEXT        NOT NULL,
    note            TEXT,
    sort            INT         NOT NULL DEFAULT 0,
    visibility      JSONB,
    translations    JSONB       NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by      UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    updated_by      UUID REFERENCES {schema}.users(id) ON DELETE SET NULL,
    UNIQUE (collection_name, name)
);
CREATE INDEX IF NOT EXISTS sections_collection_sort_idx
    ON {schema}.sections (collection_name, sort, name);

ALTER TABLE {schema}.fields
    ADD COLUMN IF NOT EXISTS section_id UUID;

INSERT INTO {schema}.sections (collection_name, name, display_name, sort)
SELECT c.name, 'general', 'General', 0
FROM {schema}.collections c
WHERE NOT EXISTS (
    SELECT 1 FROM {schema}.sections s
    WHERE s.collection_name = c.name AND s.name = 'general'
);

UPDATE {schema}.fields f
SET section_id = s.id
FROM {schema}.sections s
WHERE s.collection_name = f.collection_name
  AND s.name = 'general'
  AND f.section_id IS NULL;

ALTER TABLE {schema}.fields
    ALTER COLUMN section_id SET NOT NULL;

DO $$ BEGIN
    ALTER TABLE {schema}.fields
        ADD CONSTRAINT fields_section_id_fkey
        FOREIGN KEY (section_id) REFERENCES {schema}.sections(id) ON DELETE RESTRICT;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE {schema}.fields
    DROP CONSTRAINT IF EXISTS fields_collection_name_name_key;

DROP INDEX IF EXISTS {schema}.fields_collection_idx;

ALTER TABLE {schema}.fields
    DROP COLUMN IF EXISTS collection_name;

CREATE UNIQUE INDEX IF NOT EXISTS fields_section_name_uidx
    ON {schema}.fields (section_id, name);

CREATE INDEX IF NOT EXISTS fields_section_sort_idx
    ON {schema}.fields (section_id, sort, name);

CREATE OR REPLACE TRIGGER trg_sections_updated_at
    BEFORE UPDATE ON {schema}.sections
    FOR EACH ROW EXECUTE FUNCTION {schema}.update_updated_at();
`,
	},
	{
		version: 4,
		sql: `
-- show_in_table: field appears as a column on /content/{collection} list tables.
ALTER TABLE {schema}.fields
    ADD COLUMN IF NOT EXISTS show_in_table BOOLEAN NOT NULL DEFAULT FALSE;

-- Preserve existing list UX: previously non-hidden custom fields were always shown.
UPDATE {schema}.fields
SET show_in_table = TRUE
WHERE COALESCE(hidden, FALSE) = FALSE
  AND COALESCE(is_system, FALSE) = FALSE;
`,
	},
	{
		version: 5,
		sql: `
-- Per-user inbox notifications (SSE: notification.created).
CREATE TABLE IF NOT EXISTS {schema}.notifications (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id  UUID        NOT NULL REFERENCES {schema}.users(id) ON DELETE CASCADE,
    actor_id      UUID        REFERENCES {schema}.users(id) ON DELETE SET NULL,
    type          VARCHAR(50) NOT NULL DEFAULT 'info',
    title         TEXT        NOT NULL,
    body          TEXT        NOT NULL DEFAULT '',
    link          TEXT,
    meta          JSONB       NOT NULL DEFAULT '{}',
    read_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS notifications_recipient_created_idx
    ON {schema}.notifications (recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS notifications_recipient_unread_idx
    ON {schema}.notifications (recipient_id)
    WHERE read_at IS NULL;
`,
	},

	{
		version: 6,
		sql: `
-- Workflow definitions + execution history.
CREATE TABLE IF NOT EXISTS {schema}.workflows (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT         NOT NULL,
    active          BOOLEAN      NOT NULL DEFAULT TRUE,
    trigger_type    VARCHAR(50)  NOT NULL,
    trigger_config  JSONB        NOT NULL DEFAULT '{}',
    last_fired_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS workflows_active_idx
    ON {schema}.workflows (active);

CREATE TABLE IF NOT EXISTS {schema}.workflow_actions (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id   UUID         NOT NULL REFERENCES {schema}.workflows(id) ON DELETE CASCADE,
    sort          INT          NOT NULL DEFAULT 0,
    action_type   VARCHAR(50)  NOT NULL,
    action_config JSONB        NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS workflow_actions_workflow_idx
    ON {schema}.workflow_actions (workflow_id, sort);

CREATE TABLE IF NOT EXISTS {schema}.workflow_runs (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id   UUID         NOT NULL REFERENCES {schema}.workflows(id) ON DELETE CASCADE,
    status        VARCHAR(20)  NOT NULL DEFAULT 'pending',
    collection    TEXT         NOT NULL DEFAULT '',
    item_id       TEXT         NOT NULL DEFAULT '',
    trigger_type  VARCHAR(50)  NOT NULL,
    error         TEXT         NOT NULL DEFAULT '',
    context       JSONB        NOT NULL DEFAULT '{}',
    started_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS workflow_runs_workflow_started_idx
    ON {schema}.workflow_runs (workflow_id, started_at DESC);
CREATE INDEX IF NOT EXISTS workflow_runs_status_idx
    ON {schema}.workflow_runs (workflow_id, status);

CREATE TABLE IF NOT EXISTS {schema}.workflow_run_steps (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       UUID         NOT NULL REFERENCES {schema}.workflow_runs(id) ON DELETE CASCADE,
    action_id    UUID         NOT NULL,
    action_type  VARCHAR(50)  NOT NULL,
    sort         INT          NOT NULL DEFAULT 0,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
    error        TEXT         NOT NULL DEFAULT '',
    output       JSONB        NOT NULL DEFAULT '{}',
    started_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS workflow_run_steps_run_idx
    ON {schema}.workflow_run_steps (run_id, sort);
`,
	},

	{
		version: 7,
		sql: `
-- Sessions may now belong to a "principal" from any auth-type collection
-- (e.g. a custom customers/operators table) instead of only {schema}.users.
-- principal_collection + principal_id identify the row; user_id stays for
-- system-user sessions and is now nullable.
ALTER TABLE {schema}.sessions
    ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE {schema}.sessions
    ADD COLUMN IF NOT EXISTS principal_collection TEXT;
ALTER TABLE {schema}.sessions
    ADD COLUMN IF NOT EXISTS principal_id UUID;
`,
	},
	{
		version: 8,
		sql: `
-- Guarantee roles.name uniqueness for idempotent role seeds.
CREATE UNIQUE INDEX IF NOT EXISTS roles_name_uidx ON {schema}.roles (name);
`,
	},
	{
		version: 9,
		sql: `
-- File storage metadata: public flag (CDN vs presigned serving) + owning collection.
ALTER TABLE {schema}.files
    ADD COLUMN IF NOT EXISTS is_public  BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE {schema}.files
    ADD COLUMN IF NOT EXISTS collection TEXT;
CREATE INDEX IF NOT EXISTS files_collection_idx ON {schema}.files (collection, created_at);
`,
	},
	{
		version: 10,
		sql: `
-- Collection groups (sidebar folders) + per-collection group membership & order.
CREATE TABLE IF NOT EXISTS {schema}.collection_groups (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    icon       TEXT,
    color      TEXT,
    sort       INT         NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    updated_by UUID
);
ALTER TABLE {schema}.collections
    ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES {schema}.collection_groups(id) ON DELETE SET NULL;
ALTER TABLE {schema}.collections
    ADD COLUMN IF NOT EXISTS sort INT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS collections_group_idx ON {schema}.collections (group_id, sort);
`,
	},
	{
		version: 11,
		sql: `
-- Programmatic API tokens (policy-scoped). Only the SHA-256 hash is stored.
CREATE TABLE IF NOT EXISTS {schema}.api_tokens (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT        NOT NULL,
    secret_prefix         TEXT        NOT NULL,
    secret_hash           TEXT        NOT NULL,
    old_secret_hash       TEXT,
    old_secret_expires_at TIMESTAMPTZ,
    policies              JSONB       NOT NULL DEFAULT '[]',
    lifetime              VARCHAR(16) NOT NULL DEFAULT 'forever',
    rotate_at             TIMESTAMPTZ,
    last_notified_at      TIMESTAMPTZ,
    revoked               BOOLEAN     NOT NULL DEFAULT FALSE,
    last_used_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by            UUID,
    updated_by            UUID,
    UNIQUE (secret_prefix)
);
CREATE INDEX IF NOT EXISTS api_tokens_rotate_idx ON {schema}.api_tokens (rotate_at) WHERE revoked = FALSE;
`,
	},
	{
		version: 12,
		sql: `
-- User invitations: a pending user carries an invite token hash until they accept.
ALTER TABLE {schema}.users ADD COLUMN IF NOT EXISTS invite_token_hash TEXT;
ALTER TABLE {schema}.users ADD COLUMN IF NOT EXISTS invite_expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS users_invite_token_idx ON {schema}.users (invite_token_hash);
`,
	},
	{
		version: 13,
		sql: `
-- Per-tenant email template overrides (optional; built-in defaults used otherwise).
CREATE TABLE IF NOT EXISTS {schema}.email_templates (
    template_key TEXT        PRIMARY KEY,
    subject      TEXT        NOT NULL DEFAULT '',
    html         TEXT        NOT NULL DEFAULT '',
    text         TEXT        NOT NULL DEFAULT '',
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by   UUID
);
`,
	},
	{
		version: 14,
		sql: `
-- Workflows provisioned by a plugin are tagged so install is idempotent and
-- uninstall can remove them.
ALTER TABLE {schema}.workflows ADD COLUMN IF NOT EXISTS origin_plugin TEXT;
CREATE INDEX IF NOT EXISTS workflows_origin_idx ON {schema}.workflows (origin_plugin);
`,
	},
}
