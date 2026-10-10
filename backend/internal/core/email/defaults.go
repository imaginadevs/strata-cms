package email

// Core template keys owned by the platform (not by a plugin).
const (
	KeyUserInvite = "user.invite"
)

// RegisterCoreDefaults registers the platform's built-in email templates. Plugins
// register their own templates separately (see features.RegisterEmailTemplates).
func RegisterCoreDefaults(r *Registry) {
	r.Register(KeyUserInvite, Template{
		Subject: `You've been invited to {{.Brand.TenantName}}`,
		HTML: `<h2 style="margin:0 0 12px;font-size:20px;">You're invited</h2>
<p>You've been invited to join <strong>{{.Brand.TenantName}}</strong>. Set your password to activate your account.</p>
<p style="margin:24px 0;"><a href="{{.Data.invite_link}}" style="background:{{.Brand.PrimaryColor}};color:#ffffff;padding:11px 20px;border-radius:8px;text-decoration:none;display:inline-block;font-weight:bold;">Accept invitation</a></p>
<p style="color:#6b7280;font-size:13px;">Or paste this link into your browser:<br>{{.Data.invite_link}}<br><br>This link expires in 7 days.</p>`,
		Text: `You've been invited to join {{.Brand.TenantName}}.

Set your password to activate your account:
{{.Data.invite_link}}

This link expires in 7 days.`,
	})
}
