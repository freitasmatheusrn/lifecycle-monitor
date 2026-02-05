package notification

import "fmt"

type Notification interface {
	Send(to, msg string) error
}

func OtpMessage(code string) string {
	return fmt.Sprintf("seu código de verificação para siemens é: %s", code)
}
