// ── Base entity ────────────────────────────────────────────────────────────────
export interface BaseEntity {
  id: string;
  created_at: string;
  updated_at: string;
  created_by: string | null;
  updated_by: string | null;
}

// ── Tenant ────────────────────────────────────────────────────────────────────
export interface Tenant {
  id: string;
  name: string;
  slug: string;
  default_language: string;
  logo: string | null;
  primary_color: string | null;
  domain: string | null;
  active: boolean;
  settings: TenantSettings;
}

export interface TenantFeatures {
  website: boolean;
  blog: boolean;
}

export interface TenantPlugins {
  allowed: string[];
  installed: string[];
  enabled: Record<string, boolean>;
}

export interface TenantStorageSettings {
  /** Use the environment/default S3 credentials; files go to tenants/{id}/… */
  inherit: boolean;
  endpoint: string;
  region: string;
  bucket: string;
  access_key_id: string;
  /** Write-only: send to set; omitted/blank preserves the stored secret. */
  secret_key?: string;
  public_base_url: string;
  cdn_base_url: string;
  force_path_style: boolean;
}

export interface TenantSettings {
  allow_registration: boolean;
  max_users?: number;
  timezone: string;
  date_format: string;
  /** Legacy flat flags; mirrored from plugins.installed. Prefer plugins. */
  features: TenantFeatures;
  plugins?: TenantPlugins;
  storage?: TenantStorageSettings;
  /** ISO 639-1 codes of languages enabled for this tenant. Empty defaults to ["en"]. */
  languages?: string[];
}

/** GET /settings response shape (adds env-storage info + secret presence). */
export interface SettingsResponse {
  tenant: Tenant;
  settings: TenantSettings;
  storage_env?: { configured: boolean; bucket: string; endpoint: string; has_cdn: boolean };
  storage_secret_set?: boolean;
}

/** Catalog entry from GET /plugins (allowed for this tenant). */
export interface Plugin {
  id: string;
  name: string;
  description: string;
  prefix: string;
  installed: boolean;
  collections: string[];
}

export interface PluginOmit {
  name: string;
  reason: string;
}

export interface PluginInstallResult {
  plugin: string;
  collections_created: string[];
  collections_omitted: PluginOmit[];
  relations_created: string[];
  relations_omitted: PluginOmit[];
}

export interface PluginInstallResponse {
  result: PluginInstallResult;
  settings: TenantSettings;
  plugins: Plugin[];
}

// ── Auth ──────────────────────────────────────────────────────────────────────
export interface AuthUser {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  avatar: string | null;
  language?: string;
  roles: string[];
  is_super_admin?: boolean;
}

/** Login / refresh response — the client holds these and sends the access token
 *  as `Authorization: Bearer <access_token>`. */
export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  user: AuthUser;
}

// ── Collection ────────────────────────────────────────────────────────────────
export type CollectionType = "base" | "auth";

export interface Collection extends BaseEntity {
  name: string;
  display_name: string;
  type: CollectionType;
  icon: string | null;
  color: string | null;
  note: string | null;
  is_system: boolean;
  is_hidden: boolean;
  is_singleton: boolean;
  sort_field: string | null;
  meta: Record<string, unknown>;
  /** Sidebar folder this collection belongs to; null = ungrouped. */
  group_id: string | null;
  /** Position within its group (or among ungrouped collections) in the nav. */
  sort: number;
  /** Nested form layout; fields live under each section (not on the collection). */
  sections?: Section[];
  translations?: Translation[];
}

// ── API tokens ────────────────────────────────────────────────────────────────
export interface ApiTokenPolicy {
  /** Collection name, a virtual resource (e.g. cms_users), or "*" for all. */
  collection: string;
  /** read | create | update | delete | all */
  action: string;
  fields?: string[];
  filter?: unknown;
}

export interface ApiToken {
  id: string;
  name: string;
  secret_prefix: string;
  policies: ApiTokenPolicy[];
  lifetime: "forever" | "monthly";
  rotate_at?: string | null;
  last_notified_at?: string | null;
  revoked: boolean;
  last_used_at?: string | null;
  created_at: string;
  updated_at: string;
}

/** Create/rotate responses include the plaintext secret exactly once. */
export interface ApiTokenSecret {
  token: ApiToken;
  secret: string;
}

// ── Collection group (sidebar folder) ───────────────────────────────────────────
export interface CollectionGroup {
  id: string;
  name: string;
  icon: string | null;
  color: string | null;
  sort: number;
  created_at: string;
  updated_at: string;
}

// ── Section ───────────────────────────────────────────────────────────────────
/** Groups fields within a collection for form layout and visibility rules. */
export interface Section extends BaseEntity {
  collection: string;
  name: string;
  display_name: string;
  note?: string | null;
  sort: number;
  visibility?: Condition | null;
  translations?: Translation[];
  fields?: Field[];
}

export type CondOp =
  | "eq" | "neq" | "gt" | "gte" | "lt" | "lte"
  | "in" | "nin" | "empty" | "nempty" | "contains";

/** Visibility tree: exactly one of group (and/or) or leaf (field + op). */
export type Condition =
  | { and: Condition[] }
  | { or: Condition[] }
  | { field: string; op: CondOp; value?: unknown };

export interface Translation {
  language: string;
  display_name: string;
  note?: string;
}

// ── Field ─────────────────────────────────────────────────────────────────────
export type FieldType =
  | "string" | "text" | "rich_text" | "integer" | "float" | "decimal"
  | "boolean" | "date" | "datetime" | "time" | "json" | "uuid"
  | "relation" | "file" | "files" | "select" | "multiselect"
  | "hash" | "ip" | "timestamp" | "csv" | "geometry";

export interface Field extends BaseEntity {
  /** Section slug this field belongs to (ownership is Field → Section → Collection). */
  section: string;
  name: string;
  display_name: string;
  type: FieldType;
  db_type: string;
  required: boolean;
  unique: boolean;
  readonly: boolean;
  hidden: boolean;
  quick_edit: boolean;
  show_in_table: boolean;
  translatable: boolean;
  default_value: unknown;
  validation: FieldValidation | null;
  options: Record<string, unknown>;
  sort: number;
  is_system: boolean;
}

export interface FieldValidation {
  min?: number;
  max?: number;
  regex?: string;
  custom?: string;
}

// ── User ──────────────────────────────────────────────────────────────────────
export type UserStatus = "active" | "inactive" | "suspended";
export type AuthProvider = "local" | "sso";

export interface User extends BaseEntity {
  email: string;
  first_name: string;
  last_name: string;
  avatar: string | null;
  status: UserStatus;
  provider: AuthProvider;
  last_login: string | null;
  language: string;
  roles?: Role[];
}

// ── Role / Permission ─────────────────────────────────────────────────────────
export interface Role extends BaseEntity {
  name: string;
  description: string;
  icon: string | null;
  is_system: boolean;
  permissions?: Permission[];
}

export type PermissionAction =
  | "create"
  | "read"
  | "update"
  | "delete"
  | "all";

export interface Permission extends BaseEntity {
  role_id: string;
  collection: string;
  action: PermissionAction;
  fields: string[];
  filter: Record<string, unknown>;
}

/** Virtual system target from GET /permissions/catalog (not a real collection). */
export interface VirtualPermissionCollection {
  name: string;
  display_name: string;
  description: string;
  actions: PermissionAction[];
}

export interface PermissionCatalog {
  actions: PermissionAction[];
  virtual: VirtualPermissionCollection[];
  note?: string;
}

// ── Relation ──────────────────────────────────────────────────────────────────
export type RelationType = "many_to_one" | "one_to_many" | "many_to_many" | "one_to_one";
export type OnDeleteBehavior = "cascade" | "set_null" | "restrict";

export interface Relation {
  id: string;
  collection_many: string;
  field_many: string;
  collection_one: string;
  field_one: string | null;
  type: RelationType;
  junction_table: string | null;
  on_delete: string;
  created_at: string;
  updated_at: string;
  created_by: string | null;
}

// ── Workflow / Automation ─────────────────────────────────────────────────────
export type WorkflowTriggerType = "field_changed" | "schedule";
export type WorkflowActionType = "email_with_pdf";
export type WorkflowRunStatus = "pending" | "running" | "success" | "failed";
export type WorkflowStepStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "skipped";

export interface WorkflowTrigger {
  type: WorkflowTriggerType;
  config: Record<string, unknown>;
}

export interface WorkflowAction {
  id: string;
  type: WorkflowActionType;
  sort: number;
  config: Record<string, unknown>;
}

export interface Workflow {
  id: string;
  name: string;
  active: boolean;
  trigger: WorkflowTrigger;
  actions: WorkflowAction[];
  last_fired_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkflowRun {
  id: string;
  workflow_id: string;
  status: WorkflowRunStatus;
  collection: string;
  item_id: string;
  trigger_type: WorkflowTriggerType;
  error?: string;
  context?: Record<string, unknown>;
  steps?: WorkflowRunStep[];
  started_at: string;
  finished_at?: string | null;
}

export interface WorkflowRunStep {
  id: string;
  run_id: string;
  action_id: string;
  action_type: WorkflowActionType;
  sort: number;
  status: WorkflowStepStatus;
  error?: string;
  output?: Record<string, unknown>;
  started_at: string;
  finished_at?: string | null;
}

/** Create / PATCH body (action ids optional). */
export interface WorkflowWriteBody {
  name?: string;
  active?: boolean;
  trigger?: {
    type: WorkflowTriggerType;
    config: Record<string, unknown>;
  };
  actions?: Array<{
    id?: string;
    type: WorkflowActionType;
    config: Record<string, unknown>;
  }>;
}

export interface FieldChangedTriggerConfig {
  collection: string;
  field: string;
  to: string;
  from?: string;
  op?: string;
}

export interface ScheduleTriggerConfig {
  cron: string;
  collection: string;
  timezone?: string;
  filter?: Record<string, unknown>;
  limit?: number;
}

export interface EmailWithPdfActionConfig {
  to: string;
  subject: string;
  body?: string;
  cc?: string;
  pdf: {
    template: string;
    filename?: string;
  };
}

export interface WorkflowCatalogField {
  key: string;
  label: string;
  description?: string;
  type: string;
  required: boolean;
}

export interface WorkflowCatalogEntry {
  type: string;
  name: string;
  description: string;
  config_fields: WorkflowCatalogField[];
}

export interface WorkflowCatalog {
  triggers: WorkflowCatalogEntry[];
  actions: WorkflowCatalogEntry[];
}

// ── Audit ─────────────────────────────────────────────────────────────────────
export type AuditOperation = "create" | "update" | "delete" | "login" | "logout";

export interface AuditLog {
  id: string;
  user_id: string | null;
  user_email: string | null;
  collection: string;
  item_id: string | null;
  operation: AuditOperation;
  old_data: Record<string, unknown> | null;
  new_data: Record<string, unknown> | null;
  changed_fields: string[];
  ip_address: string | null;
  created_at: string;
}

// ── API responses ─────────────────────────────────────────────────────────────
export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
}

export interface ApiError {
  error: string;
  details?: Record<string, string[]>;
}

// ── Index ─────────────────────────────────────────────────────────────────────
export type IndexType = "btree" | "fulltext";

export interface CollectionIndex {
  id: string;
  collection_name: string;
  name: string;
  type: IndexType;
  fields: string[];
  created_at: string;
  created_by: string | null;
}

// ── Auto-generation ───────────────────────────────────────────────────────────
export interface AutoGenFunction {
  id: string;
  label: string;
  description: string;
  needs_source_field: boolean;
}

export type AutoGenTrigger = "create" | "always";

// ── Notification ──────────────────────────────────────────────────────────────
export type NotificationType = "info" | "success" | "warning" | "error";

export interface Notification {
  id: string;
  recipient_id: string;
  actor_id?: string | null;
  type: NotificationType;
  title: string;
  body: string;
  link?: string | null;
  meta?: Record<string, unknown>;
  read_at?: string | null;
  created_at: string;
}

/** Payload on SSE `notification.created` (and shape used to prepend into the inbox). */
export interface NotificationCreatedPayload {
  id: string;
  recipient_id: string;
  type: NotificationType;
  title: string;
  body: string;
  link?: string | null;
  meta?: Record<string, unknown>;
  created_at: string;
}

export interface NotificationReadPayload {
  id?: string;
  all?: boolean;
}

// ── SSE event ─────────────────────────────────────────────────────────────────
export interface SSEEvent<T = unknown> {
  type: string;
  collection?: string;
  user_id?: string;
  payload?: T;
  timestamp?: string;
}
