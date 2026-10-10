package outbound

import "context"

type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Message struct {
	To          []string
	Cc          []string
	Subject     string
	Body        string // plain-text part (always sent as the fallback)
	HTMLBody    string // optional HTML part; when set the email is multipart/alternative
	Attachments []Attachment
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}
