package workflows

import (
	"context"
	"fmt"
	"strings"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type emailWithPDFAction struct {
	mail outbound.Mailer
	pdf  outbound.PDFRenderer
}

func NewEmailWithPDFAction(mail outbound.Mailer, pdf outbound.PDFRenderer) ActionExecutor {
	return &emailWithPDFAction{mail: mail, pdf: pdf}
}

func (a *emailWithPDFAction) Type() wfdomain.ActionType { return wfdomain.ActionEmailWithPDF }

func (a *emailWithPDFAction) ValidateConfig(cfg map[string]any) error {
	if mapString(cfg, "to") == "" {
		return errf("email_with_pdf: to is required")
	}
	if mapString(cfg, "subject") == "" {
		return errf("email_with_pdf: subject is required")
	}
	pdf := nestedMap(cfg, "pdf")
	if pdf == nil || mapString(pdf, "template") == "" {
		return errf("email_with_pdf: pdf.template is required")
	}
	return nil
}

func (a *emailWithPDFAction) Execute(ctx context.Context, in ActionInput) (ActionOutput, error) {
	if a.mail == nil {
		return ActionOutput{}, errf("email_with_pdf: mailer not configured")
	}
	if a.pdf == nil {
		return ActionOutput{}, errf("email_with_pdf: pdf renderer not configured")
	}

	toRaw := renderTemplate(mapString(in.Config, "to"), in.Item)
	to := splitEmails(toRaw)
	if len(to) == 0 {
		return ActionOutput{}, errf("email_with_pdf: resolved to is empty")
	}
	subject := renderTemplate(mapString(in.Config, "subject"), in.Item)
	body := renderTemplate(mapString(in.Config, "body"), in.Item)
	cc := asStringSlice(in.Config["cc"])
	for i := range cc {
		cc[i] = renderTemplate(cc[i], in.Item)
	}

	pdfCfg := nestedMap(in.Config, "pdf")
	template := mapString(pdfCfg, "template")
	filenameTpl := mapString(pdfCfg, "filename")
	filenameTpl = renderTemplate(filenameTpl, in.Item)

	fname, pdfBytes, err := a.pdf.Render(ctx, outbound.PDFRenderRequest{
		TenantID:   in.TenantID,
		Collection: in.Collection,
		Template:   template,
		Filename:   filenameTpl,
		ItemID:     in.ItemID,
		Item:       in.Item,
	})
	if err != nil {
		return ActionOutput{}, fmt.Errorf("pdf render: %w", err)
	}

	err = a.mail.Send(ctx, outbound.Message{
		To:      to,
		Cc:      cc,
		Subject: subject,
		Body:    body,
		Attachments: []outbound.Attachment{{
			Filename:    fname,
			ContentType: "application/pdf",
			Data:        pdfBytes,
		}},
	})
	if err != nil {
		return ActionOutput{}, fmt.Errorf("smtp send: %w", err)
	}

	return ActionOutput{Data: map[string]any{
		"pdf_filename": fname,
		"pdf_bytes":    len(pdfBytes),
		"to":           to,
		"subject":      subject,
	}}, nil
}

func splitEmails(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
