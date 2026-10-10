package tenant

import (
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
)

// Tenant is a top-level isolation unit (white-label / multi-tenant).
// Every other entity carries a tenant_id foreign key.
type Tenant struct {
	ID               uuid.UUID `json:"id"                db:"id"`
	Name             string    `json:"name"              db:"name"`
	Slug             string    `json:"slug"              db:"slug"`
	DefaultLanguage  string    `json:"default_language"  db:"default_language"`
	Logo             *string   `json:"logo,omitempty"    db:"logo"`
	PrimaryColor     *string   `json:"primary_color"     db:"primary_color"`
	Domain           *string   `json:"domain,omitempty"  db:"domain"`
	Active           bool      `json:"active"            db:"active"`
	Settings         Settings  `json:"settings"          db:"settings"`
	StorageUsedBytes int64     `json:"storage_used_bytes" db:"storage_used_bytes"`
	// StorageQuotaBytes = 0 means unlimited.
	StorageQuotaBytes int64     `json:"storage_quota_bytes" db:"storage_quota_bytes"`
	CreatedAt         time.Time `json:"created_at"          db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"          db:"updated_at"`
}

// StorageAvailable returns how many bytes are still available, or -1 if unlimited.
func (t *Tenant) StorageAvailable() int64 {
	if t.StorageQuotaBytes == 0 {
		return -1
	}
	avail := t.StorageQuotaBytes - t.StorageUsedBytes
	if avail < 0 {
		return 0
	}
	return avail
}

// StorageUsedPercent returns 0–100 (or 0 when unlimited).
func (t *Tenant) StorageUsedPercent() float64 {
	if t.StorageQuotaBytes == 0 {
		return 0
	}
	return float64(t.StorageUsedBytes) / float64(t.StorageQuotaBytes) * 100
}

// Features is the legacy flat flag shape (website/blog). Prefer Plugins.
// Kept for JSON compatibility with older clients; NormalizePlugins keeps them in sync.
type Features struct {
	Website   bool `json:"website"`
	Blog      bool `json:"blog"`
	Ecommerce bool `json:"ecommerce"`
}

// Plugins is the entitlement + install model for blueprint modules.
// Allowed   = which plugins this tenant may see/install (super-admin).
// Installed = which plugins have been installed (idempotent Install API).
// Enabled   = legacy mirror of Installed for older clients (features.website/blog).
type Plugins struct {
	Allowed   []string        `json:"allowed"`
	Installed []string        `json:"installed"`
	Enabled   map[string]bool `json:"enabled"`
}

// EcommerceSettings holds per-tenant ecommerce plugin configuration.
// TrackInventory gates whether checkout validates and decrements stock;
// AllowBackorder (only meaningful when TrackInventory is on) permits stock to
// go negative instead of rejecting the order.
type EcommerceSettings struct {
	TrackInventory bool `json:"track_inventory"`
	AllowBackorder bool `json:"allow_backorder"`
}

// DefaultEcommerceSettings is the configuration a freshly activated ecommerce
// plugin starts with. It is provisioned once, on first install, and never
// re-applied on idempotent re-installs so an operator's later changes survive.
// Conservative defaults: inventory tracking off (nothing to reconcile until a
// store opts in), backorders off.
func DefaultEcommerceSettings() EcommerceSettings {
	return EcommerceSettings{TrackInventory: false, AllowBackorder: false}
}

// StorageSettings is a tenant's file-storage configuration. When Inherit is true
// (or the fields below are incomplete) the tenant uses the environment/default S3
// credentials, with files stored under tenants/{tenant_id}/… in the default
// bucket. Otherwise the tenant's own S3 credentials are used.
type StorageSettings struct {
	Inherit        bool   `json:"inherit"`
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	AccessKeyID    string `json:"access_key_id"`
	SecretKey      string `json:"secret_key"`
	PublicBaseURL  string `json:"public_base_url"`
	CDNBaseURL     string `json:"cdn_base_url"`
	ForcePathStyle bool   `json:"force_path_style"`
}

// Configured reports whether the tenant supplied its own usable S3 credentials.
func (s StorageSettings) Configured() bool {
	return s.Bucket != "" && s.AccessKeyID != "" && s.SecretKey != ""
}

// UseOwn reports whether the tenant's own S3 config should be used (vs. the
// environment/default). Inherit always wins; otherwise own creds must be complete.
func (s StorageSettings) UseOwn() bool {
	return !s.Inherit && s.Configured()
}

// Settings holds tenant-scoped configuration stored as JSONB.
type Settings struct {
	AllowRegistration bool              `json:"allow_registration"`
	MaxUsers          *int              `json:"max_users,omitempty"`
	Timezone          string            `json:"timezone"`
	DateFormat        string            `json:"date_format"`
	Features          Features          `json:"features"`
	Plugins           Plugins           `json:"plugins"`
	Ecommerce         EcommerceSettings `json:"ecommerce"`
	Storage           StorageSettings   `json:"storage"`
	// Languages is the list of ISO 639-1 language codes enabled for this tenant.
	// The first entry is treated as the default.  An empty slice defaults to ["en"].
	Languages []string `json:"languages,omitempty"`
}

// NormalizePlugins migrates legacy Features/Enabled into Installed and keeps mirrors synced.
func (s *Settings) NormalizePlugins() {
	if s.Plugins.Enabled == nil {
		s.Plugins.Enabled = map[string]bool{}
	}
	if s.Plugins.Installed == nil {
		s.Plugins.Installed = []string{}
	}
	legacy := s.Features.Website || s.Features.Blog || s.Features.Ecommerce

	// Enabled → Installed (one-time from older plugin model).
	if len(s.Plugins.Installed) == 0 {
		for id, on := range s.Plugins.Enabled {
			if on {
				s.Plugins.Installed = append(s.Plugins.Installed, id)
			}
		}
	}
	if len(s.Plugins.Installed) == 0 && legacy {
		if s.Features.Website {
			s.Plugins.Installed = append(s.Plugins.Installed, "website")
		}
		if s.Features.Blog {
			s.Plugins.Installed = append(s.Plugins.Installed, "blog")
		}
		if s.Features.Ecommerce {
			s.Plugins.Installed = append(s.Plugins.Installed, "ecommerce")
		}
	}

	// Keep Enabled + Features in sync with Installed for older UIs.
	s.Plugins.Enabled = map[string]bool{}
	for _, id := range s.Plugins.Installed {
		s.Plugins.Enabled[id] = true
	}
	s.Features.Website = s.Plugins.Enabled["website"]
	s.Features.Blog = s.Plugins.Enabled["blog"]
	s.Features.Ecommerce = s.Plugins.Enabled["ecommerce"]

	if s.Plugins.Allowed == nil {
		if legacy || len(s.Plugins.Installed) > 0 {
			s.Plugins.Allowed = []string{"website", "blog", "ecommerce"}
		} else {
			s.Plugins.Allowed = []string{}
		}
	}
}

// PluginAllowed reports whether the tenant may see/install the plugin.
func (s Settings) PluginAllowed(id string) bool {
	for _, a := range s.Plugins.Allowed {
		if a == id {
			return true
		}
	}
	return false
}

// PluginInstalled reports whether Install has been recorded for the plugin.
func (s Settings) PluginInstalled(id string) bool {
	for _, a := range s.Plugins.Installed {
		if a == id {
			return true
		}
	}
	return false
}

// PluginEnabled is an alias of PluginInstalled (legacy name).
func (s Settings) PluginEnabled(id string) bool {
	return s.PluginInstalled(id)
}

// MarkPluginInstalled appends id to Installed if missing.
func (s *Settings) MarkPluginInstalled(id string) {
	s.NormalizePlugins()
	if s.PluginInstalled(id) {
		return
	}
	s.Plugins.Installed = append(s.Plugins.Installed, id)
	s.NormalizePlugins()
}

func New(name, slug, defaultLanguage string) *Tenant {
	now := time.Now().UTC()
	return &Tenant{
		ID:              ids.New(),
		Name:            name,
		Slug:            slug,
		DefaultLanguage: defaultLanguage,
		Active:          true,
		Settings: Settings{
			AllowRegistration: false,
			Timezone:          "UTC",
			DateFormat:        "YYYY-MM-DD",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
