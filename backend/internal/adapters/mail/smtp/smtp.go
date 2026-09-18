package smtp

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
)

type Mailer struct {
	host string
	port string
	user string
	pass string
	from string
}

func New(cfg config.SMTPConfig) *Mailer {
	return &Mailer{
		host: cfg.Host,
		port: cfg.Port,
		user: cfg.User,
		pass: cfg.Password,
		from: cfg.From,
	}
}

func (m *Mailer) Send(_ context.Context, msg outbound.Message) error {
	if m.host == "" {
		return fmt.Errorf("smtp not configured")
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("smtp: no recipients")
	}
	from := m.from
	if from == "" {
		from = m.user
	}
	addr := net.JoinHostPort(m.host, m.port)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	if len(msg.Cc) > 0 {
		b.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(msg.Cc, ", ")))
	}
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	b.WriteString("MIME-Version: 1.0\r\n")

	boundary := "cms-wf-boundary"
	if len(msg.Attachments) == 0 {
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Body)
	} else {
		b.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", boundary))
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Body)
		b.WriteString("\r\n")
		for _, att := range msg.Attachments {
			ct := att.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			b.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", ct, att.Filename))
			b.WriteString("Content-Transfer-Encoding: base64\r\n")
			b.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", att.Filename))
			encoded := base64.StdEncoding.EncodeToString(att.Data)
			for i := 0; i < len(encoded); i += 76 {
				end := i + 76
				if end > len(encoded) {
					end = len(encoded)
				}
				b.WriteString(encoded[i:end])
				b.WriteString("\r\n")
			}
		}
		b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	}

	recipients := append([]string{}, msg.To...)
	recipients = append(recipients, msg.Cc...)

	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}
	return smtp.SendMail(addr, auth, from, recipients, []byte(b.String()))
}
