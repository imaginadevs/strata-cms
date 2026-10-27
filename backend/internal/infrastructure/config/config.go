package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App        AppConfig
	Server     ServerConfig
	Database   DatabaseConfig
	Auth       AuthConfig
	Logto      LogtoConfig
	Kafka      KafkaConfig
	Storage    StorageConfig
	SMTP       SMTPConfig
	SuperAdmin SuperAdminConfig
	Features   FeatureFlags
	Security   SecurityConfig
}

// SecurityConfig holds secrets-at-rest configuration.
type SecurityConfig struct {
	// EncryptionKey encrypts secrets stored in the DB (e.g. tenant S3 secret).
	// Falls back to JWT_SECRET when unset. Changing it makes existing ciphertext
	// unreadable — rotate deliberately.
	EncryptionKey string
}

type AppConfig struct {
	Environment string
	Name        string
	Version     string
}

type ServerConfig struct {
	Host         string
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// StorageConfig holds global S3-compatible credentials.
// One bucket is used for all tenants; files are stored under
// tenants/{tenant_id}/{collection}/{uuid}.ext — no cross-tenant
// access is possible since the app controls key generation.
// Tenants may optionally override this with their own bucket in settings.
type StorageConfig struct {
	Endpoint       string
	Region         string
	Bucket         string
	AccessKeyID    string
	SecretKey      string
	PublicBaseURL  string
	ForcePathStyle bool // required for MinIO
	// CDNBaseURL is the origin of the image/transform CDN used to serve files
	// marked public (e.g. https://images.imaginahosting.com). Public files are
	// served through the CDN; private files use short-lived presigned URLs.
	CDNBaseURL string
	// MaxUploadBytes caps a single upload. 0 means no explicit limit.
	MaxUploadBytes int64
	// KeyPrefix namespaces this app's objects inside a bucket shared with other
	// apps (e.g. "cms"). Keys become {prefix}/tenants/{id}/{collection}/{uuid}.ext.
	// Empty = no prefix.
	KeyPrefix string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	MaxConns int32
	MinConns int32
	DSN      string
}

type AuthConfig struct {
	JWTSecret          string
	JWTExpiry          time.Duration
	RefreshTokenExpiry time.Duration
}

type LogtoConfig struct {
	Endpoint    string
	AppID       string
	AppSecret   string
	RedirectURI string
}

type FeatureFlags struct {
	GraphQL   bool
	REST      bool
	SSO       bool
	Kafka     bool
	Workflows bool
}

type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

type KafkaConfig struct {
	Brokers     []string
	TopicPrefix string
}

// SuperAdminConfig holds settings that ONLY live in the environment.
// These values are never read from or written to the database.
type SuperAdminConfig struct {
	// AllowedIPs is the IP/CIDR whitelist for super-admin access.
	// Empty slice means super-admin login is completely disabled.
	AllowedIPs []string

	// Email + Password seed the platform super-admin account on startup. The
	// account lives in the platform tenant's schema. The password is used only
	// to CREATE the account; an existing account is never re-passworded, so the
	// password env var can be removed after first start.
	Email    string
	Password string

	// PlatformDomain, when set, is applied to the platform tenant's `domain` on
	// startup so the super-admin can sign in at that host in production. Leave
	// empty to keep the platform tenant's existing domain (e.g. localhost in dev).
	PlatformDomain string
}

func Load() *Config {
	v := viper.New()

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("..")
	_ = v.ReadInConfig()

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults(v)

	cfg := &Config{}

	cfg.App.Environment = v.GetString("APP_ENV")
	cfg.App.Name = v.GetString("APP_NAME")
	cfg.App.Version = v.GetString("APP_VERSION")

	cfg.Server.Host = v.GetString("APP_HOST")
	cfg.Server.Port = v.GetString("APP_PORT")
	cfg.Server.ReadTimeout = v.GetDuration("SERVER_READ_TIMEOUT")
	cfg.Server.WriteTimeout = v.GetDuration("SERVER_WRITE_TIMEOUT")

	cfg.Database.Host = v.GetString("DB_HOST")
	cfg.Database.Port = v.GetString("DB_PORT")
	cfg.Database.Name = v.GetString("DB_NAME")
	cfg.Database.User = v.GetString("DB_USER")
	cfg.Database.Password = v.GetString("DB_PASSWORD")
	cfg.Database.MaxConns = int32(v.GetInt("DB_MAX_CONNS"))
	cfg.Database.MinConns = int32(v.GetInt("DB_MIN_CONNS"))
	cfg.Database.DSN = buildDSN(cfg.Database)

	cfg.Auth.JWTSecret = v.GetString("JWT_SECRET")
	cfg.Auth.JWTExpiry = v.GetDuration("JWT_EXPIRY")
	cfg.Auth.RefreshTokenExpiry = v.GetDuration("REFRESH_TOKEN_EXPIRY")

	// Secrets-at-rest key; falls back to the JWT secret when not set explicitly.
	cfg.Security.EncryptionKey = v.GetString("SECRETS_ENCRYPTION_KEY")
	if cfg.Security.EncryptionKey == "" {
		cfg.Security.EncryptionKey = cfg.Auth.JWTSecret
	}

	cfg.Logto.Endpoint = v.GetString("LOGTO_ENDPOINT")
	cfg.Logto.AppID = v.GetString("LOGTO_APP_ID")
	cfg.Logto.AppSecret = v.GetString("LOGTO_APP_SECRET")
	cfg.Logto.RedirectURI = v.GetString("LOGTO_REDIRECT_URI")

	cfg.Storage.Endpoint = v.GetString("S3_ENDPOINT")
	cfg.Storage.Region = v.GetString("S3_REGION")
	cfg.Storage.Bucket = v.GetString("S3_BUCKET")
	cfg.Storage.AccessKeyID = v.GetString("S3_ACCESS_KEY_ID")
	cfg.Storage.SecretKey = v.GetString("S3_SECRET_KEY")
	cfg.Storage.PublicBaseURL = v.GetString("S3_PUBLIC_BASE_URL")
	cfg.Storage.ForcePathStyle = v.GetBool("S3_FORCE_PATH_STYLE")
	cfg.Storage.CDNBaseURL = v.GetString("S3_CDN_BASE_URL")
	cfg.Storage.MaxUploadBytes = int64(v.GetInt("S3_MAX_UPLOAD_MB")) * 1024 * 1024
	cfg.Storage.KeyPrefix = v.GetString("S3_KEY_PREFIX")

	cfg.Features.GraphQL = v.GetBool("ENABLE_GRAPHQL")
	cfg.Features.REST = v.GetBool("ENABLE_REST")
	cfg.Features.SSO = v.GetBool("ENABLE_SSO")
	cfg.Features.Kafka = v.GetBool("ENABLE_KAFKA")
	cfg.Features.Workflows = v.GetBool("ENABLE_WORKFLOWS")

	cfg.SMTP.Host = v.GetString("SMTP_HOST")
	cfg.SMTP.Port = v.GetString("SMTP_PORT")
	cfg.SMTP.User = v.GetString("SMTP_USER")
	cfg.SMTP.Password = v.GetString("SMTP_PASSWORD")
	cfg.SMTP.From = v.GetString("SMTP_FROM")

	cfg.Kafka.Brokers = splitList(v.GetString("KAFKA_BROKERS"))
	cfg.Kafka.TopicPrefix = v.GetString("KAFKA_TOPIC_PREFIX")

	// Super admin settings — sourced from env only, never from DB.
	// Parsed with splitList because viper's GetStringSlice splits a .env string on
	// whitespace, not commas — so "a,b,c" would collapse into one bogus entry.
	cfg.SuperAdmin.AllowedIPs = splitList(v.GetString("SUPER_ADMIN_ALLOWED_IPS"))
	cfg.SuperAdmin.Email = v.GetString("SUPER_ADMIN_EMAIL")
	cfg.SuperAdmin.Password = v.GetString("SUPER_ADMIN_PASSWORD")
	cfg.SuperAdmin.PlatformDomain = v.GetString("PLATFORM_DOMAIN")

	return cfg
}

// splitList parses a comma- or whitespace-separated env value into a trimmed,
// non-empty slice (e.g. "1.2.3.4, 5.6.7.8" → ["1.2.3.4","5.6.7.8"]).
func splitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("APP_NAME", "CMS")
	v.SetDefault("APP_VERSION", "0.1.0")
	v.SetDefault("APP_HOST", "0.0.0.0")
	v.SetDefault("APP_PORT", "8080")
	v.SetDefault("SERVER_READ_TIMEOUT", "15s")
	v.SetDefault("SERVER_WRITE_TIMEOUT", "15s")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_NAME", "cms")
	v.SetDefault("DB_USER", "cms")
	v.SetDefault("DB_MAX_CONNS", 25)
	v.SetDefault("DB_MIN_CONNS", 5)
	v.SetDefault("JWT_EXPIRY", "24h")
	v.SetDefault("REFRESH_TOKEN_EXPIRY", "168h")
	v.SetDefault("ENABLE_GRAPHQL", true)
	v.SetDefault("ENABLE_REST", true)
	v.SetDefault("ENABLE_SSO", false)
	v.SetDefault("ENABLE_KAFKA", false)
	v.SetDefault("ENABLE_WORKFLOWS", true)
	v.SetDefault("SMTP_PORT", "587")
	v.SetDefault("KAFKA_TOPIC_PREFIX", "cms")
	v.SetDefault("S3_REGION", "us-east-1")
	v.SetDefault("S3_FORCE_PATH_STYLE", false)
	v.SetDefault("S3_MAX_UPLOAD_MB", 25)
}

func buildDSN(db DatabaseConfig) string {
	return "postgres://" + db.User + ":" + db.Password +
		"@" + db.Host + ":" + db.Port + "/" + db.Name +
		"?sslmode=disable"
}
