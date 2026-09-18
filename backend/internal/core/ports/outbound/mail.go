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
	Body        string
	Attachments []Attachment
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}
