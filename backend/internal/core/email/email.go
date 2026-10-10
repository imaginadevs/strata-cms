// Package email renders branded transactional emails from registered templates.
//
// Templates are code-owned defaults (shared by every tenant — no duplication).
// Branding (logo, color, name) is injected per tenant at render time from a base
// layout, so one template serves all tenants. A tenant may supply an override; it
// is validated on save and, if it fails to render at send time, the renderer falls
// back per-field to the built-in default so a broken custom template never blocks
// a critical email.
package email

import (
	"bytes"
	"context"
	"fmt"
	htmltemplate "html/template"
	"sort"
	"sync"
	texttemplate "text/template"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// Brand is the per-tenant presentation injected into the base layout.
type Brand struct {
	TenantName   string
	AppName      string
	LogoURL      string
	PrimaryColor string
}

func (b Brand) color() string {
	if b.PrimaryColor != "" {
		return b.PrimaryColor
	}
	return "#4f46e5"
}

// Template is one email's parts. Subject/Text use text/template; HTML uses
// html/template (auto-escaped) and is wrapped by the branded base layout.
type Template struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

type renderData struct {
	Brand Brand
	Data  map[string]any
}

// OverrideStore returns a tenant's custom template for a key, or nil if none
// (or if the override is disabled → the built-in default is used).
type OverrideStore interface {
	Get(ctx context.Context, tenantID uuid.UUID, key string) (*Template, error)
}

// StoredTemplate is a persisted tenant override (for listing/management).
type StoredTemplate struct {
	Key      string `json:"key"`
	Template        // embedded Subject/HTML/Text
	Enabled  bool   `json:"enabled"`
}

// TemplateStore is the full CRUD surface for tenant overrides. The renderer only
// needs OverrideStore (Get).
type TemplateStore interface {
	OverrideStore
	Upsert(ctx context.Context, tenantID uuid.UUID, key string, t Template, enabled bool, actor *uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID) ([]StoredTemplate, error)
	Delete(ctx context.Context, tenantID uuid.UUID, key string) error
}

// Registry holds the code-owned default templates, keyed by a stable id
// (e.g. "user.invite", "blog.entry.created").
type Registry struct {
	mu       sync.RWMutex
	defaults map[string]Template
}

func NewRegistry() *Registry { return &Registry{defaults: map[string]Template{}} }

func (r *Registry) Register(key string, t Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaults[key] = t
}

func (r *Registry) Get(key string) (Template, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.defaults[key]
	return t, ok
}

func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.defaults))
	for k := range r.defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Validate parses every field of a template and returns the first parse error.
// Used to reject invalid tenant overrides on save.
func Validate(t Template) error {
	if _, err := texttemplate.New("s").Parse(t.Subject); err != nil {
		return fmt.Errorf("subject: %w", err)
	}
	if t.HTML != "" {
		if _, err := htmltemplate.New("h").Parse(t.HTML); err != nil {
			return fmt.Errorf("html: %w", err)
		}
	}
	if t.Text != "" {
		if _, err := texttemplate.New("t").Parse(t.Text); err != nil {
			return fmt.Errorf("text: %w", err)
		}
	}
	return nil
}

// Renderer turns a template key + brand + data into a ready-to-send Message.
type Renderer struct {
	reg       *Registry
	overrides OverrideStore // optional
	appName   string
}

func NewRenderer(reg *Registry, overrides OverrideStore, appName string) *Renderer {
	return &Renderer{reg: reg, overrides: overrides, appName: appName}
}

// Render resolves override→default per field (with render-time fallback), wraps
// the HTML in the branded layout, and returns the Message.
func (rn *Renderer) Render(ctx context.Context, tenantID uuid.UUID, key string, brand Brand, data map[string]any) (outbound.Message, error) {
	def, ok := rn.reg.Get(key)
	if !ok {
		return outbound.Message{}, fmt.Errorf("unknown email template %q", key)
	}
	if brand.AppName == "" {
		brand.AppName = rn.appName
	}
	if brand.PrimaryColor == "" {
		brand.PrimaryColor = brand.color()
	}
	rd := renderData{Brand: brand, Data: data}

	var ov *Template
	if rn.overrides != nil {
		if o, err := rn.overrides.Get(ctx, tenantID, key); err == nil && o != nil && Validate(*o) == nil {
			ov = o
		}
	}
	ovSubj, ovHTML, ovText := "", "", ""
	if ov != nil {
		ovSubj, ovHTML, ovText = ov.Subject, ov.HTML, ov.Text
	}

	subject := renderOrDefault(ovSubj, def.Subject, rd, false)
	htmlInner := renderOrDefault(ovHTML, def.HTML, rd, true)
	text := renderOrDefault(ovText, def.Text, rd, false)
	if text == "" {
		text = subject
	}

	return outbound.Message{
		Subject:  subject,
		Body:     text,
		HTMLBody: wrapLayout(brand, htmlInner),
	}, nil
}

// renderOrDefault tries the override value first; on parse/exec error (or empty)
// it renders the built-in default.
func renderOrDefault(overrideVal, defaultVal string, rd renderData, isHTML bool) string {
	if overrideVal != "" {
		if out, err := execField(overrideVal, rd, isHTML); err == nil {
			return out
		}
	}
	out, _ := execField(defaultVal, rd, isHTML)
	return out
}

func execField(tpl string, rd renderData, isHTML bool) (string, error) {
	if tpl == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if isHTML {
		t, err := htmltemplate.New("f").Parse(tpl)
		if err != nil {
			return "", err
		}
		if err := t.Execute(&buf, rd); err != nil {
			return "", err
		}
	} else {
		t, err := texttemplate.New("f").Parse(tpl)
		if err != nil {
			return "", err
		}
		if err := t.Execute(&buf, rd); err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

// wrapLayout injects already-rendered inner HTML into the branded email shell.
func wrapLayout(brand Brand, innerHTML string) string {
	data := struct {
		Brand
		Content htmltemplate.HTML
		Color   string
	}{Brand: brand, Content: htmltemplate.HTML(innerHTML), Color: brand.color()} //nolint:gosec // inner is engine-rendered

	t := htmltemplate.Must(htmltemplate.New("layout").Parse(baseLayout))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return innerHTML // extremely unlikely; return unwrapped content
	}
	return buf.String()
}

const baseLayout = `<!doctype html>
<html><body style="margin:0;padding:0;background:#f4f5f7;font-family:Arial,Helvetica,sans-serif;color:#1f2937;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f5f7;padding:24px 0;"><tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;background:#ffffff;border-radius:12px;overflow:hidden;border:1px solid #e5e7eb;">
<tr><td style="padding:20px 28px;border-bottom:3px solid {{.Color}};">
{{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.TenantName}}" height="36" style="display:block;">{{else}}<span style="font-size:18px;font-weight:bold;color:{{.Color}};">{{.TenantName}}</span>{{end}}
</td></tr>
<tr><td style="padding:28px;font-size:15px;line-height:1.6;">{{.Content}}</td></tr>
<tr><td style="padding:16px 28px;border-top:1px solid #e5e7eb;color:#9ca3af;font-size:12px;">Sent by {{.AppName}}{{if .TenantName}} · {{.TenantName}}{{end}}</td></tr>
</table>
</td></tr></table>
</body></html>`
