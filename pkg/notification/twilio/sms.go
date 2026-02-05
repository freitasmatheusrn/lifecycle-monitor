package twilio

import (
	"regexp"

	"github.com/twilio/twilio-go"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

type Sms struct {
	kind   string
	from   string
	client *twilio.RestClient
}

func InitClient(accountSid, authToken string) *twilio.RestClient {
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSid,
		Password: authToken,
	})
	return client
}

func NewSMS(from string, client *twilio.RestClient) *Sms {
	return &Sms{
		kind:   "sms",
		from:   from,
		client: client,
	}
}

func (s *Sms) Send(to, msg string) error {
	params := &api.CreateMessageParams{}
	params.SetBody(msg)
	params.SetFrom(s.from)
	params.SetTo(formatNumber(to))

	_, err := s.client.Api.CreateMessage(params)
	if err != nil {
		return err
	}
	return nil
}

func formatNumber(phone string) string {
	re := regexp.MustCompile(`\D`)
	digits := re.ReplaceAllString(phone, "")

	// Prefixa com +55
	return "+55" + digits
}
