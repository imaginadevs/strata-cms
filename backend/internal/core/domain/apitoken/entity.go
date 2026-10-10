// Package apitoken defines programmatic access tokens scoped by policy.
//
// A token's plaintext secret is shown only once (at creation); only a SHA-256
// hash is stored. Policies reuse the same (collection, action) model as RBAC
// permissions and can be edited after creation. Tokens support a 24h-grace
// rotation and an optional monthly auto-rotation that emails admins beforehand.
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	domain "github.com/imaginacolombia/cms/internal/core/domain"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
)

// Lifetime controls automatic rotation cadence.
type Lifetime string

const (
	LifetimeForever Lifetime = "forever" // never auto-rotates
	LifetimeMonthly Lifetime = "monthly" // auto-rotates ~every 30 days, admins emailed 24h prior
)

// MonthlyInterval is the rotation cadence for monthly tokens.
const MonthlyInterval = 30 * 24 * time.Hour

// NotifyLead is how long before an auto-rotation admins are emailed.
const NotifyLead = 24 * time.Hour

// GraceWindow is how long the previous secret stays valid after a grace rotation.
const GraceWindow = 24 * time.Hour

// TokenPrefix is the human-recognisable prefix on every secret.
const tokenPrefix = "cms"

// Policy is one (collection, action) grant. Mirrors userdomain.Permission so the
// permission middleware can evaluate it identically ("*" collection / "all"|"*"
// action are wildcards).
type Policy struct {
	Collection string            `json:"collection"`
	Action     userdomain.Action `json:"action"`
	Fields     []string          `json:"fields,omitempty"`
	Filter     any               `json:"filter,omitempty"`
}

// APIToken is a stored token record (never holds the plaintext secret).
type APIToken struct {
	domain.Base

	Name               string     `json:"name"`
	SecretPrefix       string     `json:"secret_prefix"` // lookup key + display (e.g. "a1b2c3d4")
	SecretHash         string     `json:"-"`             // sha256 of the active secret
	OldSecretHash      *string    `json:"-"`             // sha256 of the previous secret during a grace window
	OldSecretExpiresAt *time.Time `json:"old_secret_expires_at,omitempty"`
	Policies           []Policy   `json:"policies"`
	Lifetime           Lifetime   `json:"lifetime"`
	RotateAt           *time.Time `json:"rotate_at,omitempty"`
	LastNotifiedAt     *time.Time `json:"last_notified_at,omitempty"`
	Revoked            bool       `json:"revoked"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
}

// RotateAtFor returns the next auto-rotation time for a lifetime (nil = forever).
func RotateAtFor(lt Lifetime, now time.Time) *time.Time {
	if lt == LifetimeMonthly {
		r := now.Add(MonthlyInterval)
		return &r
	}
	return nil
}

// Rotate issues a new secret under the same prefix and returns the new plaintext
// (shown once). With grace=true the previous secret stays valid for GraceWindow;
// otherwise it is invalidated immediately. Also reschedules the next auto-rotation
// and clears the notify stamp. Returns "" if secret generation fails.
func (t *APIToken) Rotate(grace bool, now time.Time) string {
	plaintext, hash, err := SecretForPrefix(t.SecretPrefix)
	if err != nil {
		return ""
	}
	if grace {
		old := t.SecretHash
		exp := now.Add(GraceWindow)
		t.OldSecretHash = &old
		t.OldSecretExpiresAt = &exp
	} else {
		t.OldSecretHash = nil
		t.OldSecretExpiresAt = nil
	}
	t.SecretHash = hash
	t.RotateAt = RotateAtFor(t.Lifetime, now)
	t.LastNotifiedAt = nil
	return plaintext
}

// DueForRotation reports whether an auto-rotation is due at `now`.
func (t *APIToken) DueForRotation(now time.Time) bool {
	return !t.Revoked && t.Lifetime == LifetimeMonthly && t.RotateAt != nil && !now.Before(*t.RotateAt)
}

// DueForNotify reports whether admins should be warned now that an auto-rotation
// is within NotifyLead, and they haven't been warned for this cycle yet.
func (t *APIToken) DueForNotify(now time.Time) bool {
	if t.Revoked || t.Lifetime != LifetimeMonthly || t.RotateAt == nil {
		return false
	}
	if now.Before(t.RotateAt.Add(-NotifyLead)) {
		return false // not yet within the lead window
	}
	if now.After(*t.RotateAt) {
		return false // rotation itself is due; handled by DueForRotation
	}
	// Only notify once per cycle: skip if already notified within this lead window.
	return t.LastNotifiedAt == nil || t.LastNotifiedAt.Before(t.RotateAt.Add(-NotifyLead))
}

// Match returns the first policy that grants (collection, action), honoring the
// "*" collection and "all"/"*" action wildcards — same semantics as RBAC.
func (t *APIToken) Match(collection string, action userdomain.Action) (*Policy, bool) {
	for i := range t.Policies {
		p := &t.Policies[i]
		if (p.Collection == collection || p.Collection == "*") &&
			(p.Action == action || p.Action == "all" || p.Action == "*") {
			return p, true
		}
	}
	return nil, false
}

// Accepts reports whether the supplied plaintext secret matches the active secret,
// or the previous secret while still inside its grace window at time `now`.
func (t *APIToken) Accepts(plaintext string, now time.Time) bool {
	if t.Revoked {
		return false
	}
	h := Hash(plaintext)
	if subtleEqual(h, t.SecretHash) {
		return true
	}
	if t.OldSecretHash != nil && t.OldSecretExpiresAt != nil && now.Before(*t.OldSecretExpiresAt) {
		return subtleEqual(h, *t.OldSecretHash)
	}
	return false
}

// GenerateSecret returns a fresh (plaintext, prefix, hash) with a new prefix.
// Used when creating a token.
func GenerateSecret() (plaintext, prefix, hash string, err error) {
	pb := make([]byte, 4)
	if _, err = rand.Read(pb); err != nil {
		return "", "", "", err
	}
	prefix = hex.EncodeToString(pb) // 8 hex chars
	plaintext, hash, err = SecretForPrefix(prefix)
	return plaintext, prefix, hash, err
}

// SecretForPrefix generates a new secret under an EXISTING prefix. Rotation keeps
// the prefix stable so a grace-window old secret still resolves by prefix.
func SecretForPrefix(prefix string) (plaintext, hash string, err error) {
	sb := make([]byte, 24)
	if _, err = rand.Read(sb); err != nil {
		return "", "", err
	}
	plaintext = tokenPrefix + "_" + prefix + "_" + hex.EncodeToString(sb)
	return plaintext, Hash(plaintext), nil
}

// Hash returns the hex SHA-256 of a plaintext token.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ParsePrefix extracts the lookup prefix from a presented token ("cms_<prefix>_<secret>").
// Returns ok=false for anything that isn't shaped like one of our tokens.
func ParsePrefix(plaintext string) (prefix string, ok bool) {
	parts := strings.SplitN(plaintext, "_", 3)
	if len(parts) != 3 || parts[0] != tokenPrefix || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// subtleEqual is a constant-time compare over equal-length hex digests.
func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
