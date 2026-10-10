package workflows

import (
	"context"
	"strings"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/email"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// sendTemplateEmailAction renders a registered email template with the tenant's
// branding + the item data and sends it. Recipients may be a comma list, a
// {{item.field}} template, or the literal "admins" (all tenant admins).
type sendTemplateEmailAction struct {
	renderer *email.Renderer
	mail     outbound.Mailer
	tenants  outbound.TenantRepository
	users    outbound.UserRepository
}

func NewSendTemplateEmailAction(renderer *email.Renderer, mail outbound.Mailer, tenants outbound.TenantRepository, users outbound.UserRepository) ActionExecutor {
	return &sendTemplateEmailAction{renderer: renderer, mail: mail, tenants: tenants, users: users}
}

func (a *sendTemplateEmailAction) Type() wfdomain.ActionType {
	return wfdomain.ActionSendTemplateEmail
}

func (a *sendTemplateEmailAction) ValidateConfig(cfg map[string]any) error {
	if mapString(cfg, "template_key") == "" {
		return errf("send_template_email: template_key is required")
	}
	if mapString(cfg, "to") == "" {
		return errf("send_template_email: to is required")
	}
	return nil
}

func (a *sendTemplateEmailAction) Execute(ctx context.Context, in ActionInput) (ActionOutput, error) {
	if a.mail == nil {
		return ActionOutput{}, errf("send_template_email: email is not configured (SMTP)")
	}
	key := mapString(in.Config, "template_key")

	// Branding from the tenant.
	var brand email.Brand
	if t, err := a.tenants.GetByID(ctx, in.TenantID); err == nil && t != nil {
		brand.TenantName = t.Name
		if t.Logo != nil {
			brand.LogoURL = *t.Logo
		}
		if t.PrimaryColor != nil {
			brand.PrimaryColor = *t.PrimaryColor
		}
	}

	// Recipients.
	toRaw := mapString(in.Config, "to")
	var recipients []string
	if strings.EqualFold(strings.TrimSpace(toRaw), "admins") {
		if emails, err := a.users.ListAdminEmails(ctx, in.TenantID); err == nil {
			recipients = emails
		}
	} else {
		for _, part := range strings.Split(renderTemplate(toRaw, in.Item), ",") {
			if p := strings.TrimSpace(part); p != "" {
				recipients = append(recipients, p)
			}
		}
	}
	if len(recipients) == 0 {
		return ActionOutput{}, errf("send_template_email: no recipients resolved")
	}

	var cc []string
	for _, c := range asStringSlice(in.Config["cc"]) {
		if c = strings.TrimSpace(renderTemplate(c, in.Item)); c != "" {
			cc = append(cc, c)
		}
	}

	data := map[string]any{"item": in.Item, "collection": in.Collection}
	msg, err := a.renderer.Render(ctx, in.TenantID, key, brand, data)
	if err != nil {
		return ActionOutput{}, errf("send_template_email: render %q: %v", key, err)
	}
	msg.To = recipients
	msg.Cc = cc

	if err := a.mail.Send(ctx, msg); err != nil {
		return ActionOutput{}, errf("send_template_email: send: %v", err)
	}
	return ActionOutput{Data: map[string]any{"sent_to": recipients, "template": key}}, nil
}
