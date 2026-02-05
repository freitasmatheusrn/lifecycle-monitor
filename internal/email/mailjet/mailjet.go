package mailjet

import (
	 ."github.com/mailjet/mailjet-apiv3-go"
)

type Mailjet struct{
	Client *Client
	Email string
	Name string
}

func New(key, secret string) (*Mailjet){
	client := NewMailjetClient(key, secret)
	return &Mailjet{
		Client: client,
		Email: "freitasmatheus@lunaltas.com",
		Name: "equipe lunaltas",
	}
}

func (m *Mailjet) Send(subject, text, html string, sendTo []string) error{
	recipients := make([]Recipient, 1)
	for i := range sendTo{
		recipients = append(recipients, Recipient{Email: sendTo[i]})
	}
	email := &InfoSendMail{
		FromEmail: m.Email,
		FromName: m.Name,
		Subject: subject,
		TextPart: text,
		HTMLPart: html,
		Recipients: recipients,
	}
	_, err := m.Client.SendMail(email)
	if err != nil{
		return err
	}
	return nil

}