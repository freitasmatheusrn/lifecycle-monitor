package smtp

import (
	"context"
	"fmt"

	mail "github.com/wneessen/go-mail"
)

type SMTP struct {
	From string
	Host string
	User string
	Pass string
	Port int
}

func New(host, user, pass string, port int) *SMTP {
	return &SMTP{
		From: "freitasmatheusrn@gmail.com",
		Host: host,
		User: user,
		Pass: pass,
		Port: port,
	}
}

func (s *SMTP) Send(subject, text, html string, recipients []string) error {
	m := mail.NewMsg()
	if err := m.From(s.From); err != nil {
		return fmt.Errorf("erro no smtp, campo 'from': %s", err)
	}
	if err := m.To(recipients...); err != nil {
		return fmt.Errorf("to error: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextHTML, html)
	c, err := mail.NewClient(
		s.Host,
		mail.WithPort(s.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(s.User),
		mail.WithPassword(s.Pass),
	)
	if err != nil {
		return err
	}
	return c.DialAndSendWithContext(context.Background(), m)
}
