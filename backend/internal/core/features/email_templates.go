package features

import "github.com/imaginacolombia/cms/internal/core/email"

// Plugin-owned email template keys. Templates are code-owned defaults shared by
// every tenant; branding is injected at render time, and a tenant may override.
const (
	EmailKeyBlogEntryCreated = "blog.entry.created"
)

// RegisterEmailTemplates registers every plugin's default email templates into
// the shared registry. Called once at startup from the server wiring.
func RegisterEmailTemplates(r *email.Registry) {
	r.Register(EmailKeyBlogEntryCreated, email.Template{
		Subject: `New blog post{{with index .Data.item "title"}}: {{.}}{{end}}`,
		HTML: `<h2 style="margin:0 0 12px;font-size:20px;">New blog post created</h2>
<p>A new post was just created in <strong>{{.Brand.TenantName}}</strong>.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:16px 0;font-size:14px;">
{{with index .Data.item "title"}}<tr><td style="padding:4px 12px 4px 0;color:#6b7280;">Title</td><td style="padding:4px 0;font-weight:bold;">{{.}}</td></tr>{{end}}
{{with index .Data.item "slug"}}<tr><td style="padding:4px 12px 4px 0;color:#6b7280;">Slug</td><td style="padding:4px 0;">{{.}}</td></tr>{{end}}
{{with index .Data.item "status"}}<tr><td style="padding:4px 12px 4px 0;color:#6b7280;">Status</td><td style="padding:4px 0;">{{.}}</td></tr>{{end}}
</table>
<p style="color:#6b7280;font-size:13px;">Open the admin to review and publish it.</p>`,
		Text: `New blog post created in {{.Brand.TenantName}}.
{{with index .Data.item "title"}}Title: {{.}}
{{end}}{{with index .Data.item "slug"}}Slug: {{.}}
{{end}}Open the admin to review and publish it.`,
	})
}
