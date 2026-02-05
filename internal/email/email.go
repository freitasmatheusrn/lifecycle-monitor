package email

type Email interface{
	Send(subject, text, html string, recipients []string) error
}