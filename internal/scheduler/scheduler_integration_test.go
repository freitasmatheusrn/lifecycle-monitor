//go:build integration

package scheduler

import (
	"os"
	"strconv"
	"testing"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/email/smtp"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/products"
	"go.uber.org/zap"
)

// TestSendRealEmail_Integration sends a real email to verify formatting
// Run with: go test -v -tags=integration ./internal/scheduler/... -run TestSendRealEmail_Integration
//
// Required environment variables:
//   - SMTP_HOST
//   - SMTP_PORT
//   - SMTP_USER
//   - SMTP_PASS
//   - TEST_EMAIL_RECIPIENT (your email to receive the test)
func TestSendRealEmail_Integration(t *testing.T) {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	recipients := []string{"freitasmatheus@lunaltas.com", "bruno.rc@outlook.com.br"}

	if smtpHost == "" || smtpUser == "" || smtpPass == "" {
		t.Skip("Skipping integration test: SMTP_HOST, SMTP_USER, and SMTP_PASS not set")
	}

	smtpPort, err := strconv.Atoi(smtpPortStr)
	if err != nil {
		smtpPort = 587
	}

	// Create SMTP client
	emailClient := smtp.New(smtpHost, smtpUser, smtpPass, smtpPort)

	// Create logger
	logger, _ := zap.NewDevelopment()

	// Create scheduler with real email client
	scheduler := &Scheduler{
		logger: logger,
		email:  emailClient,
	}

	// Simulate status changes
	changes := []products.LifecycleStatusChange{
		{
			ProductCode: "6ES7214-1AG40-0XB0",
			OldStatus:   "Active",
			NewStatus:   "Phase Out",
		},
		{
			ProductCode: "6ES7215-1AG40-0XB0",
			OldStatus:   "Phase Out",
			NewStatus:   "Discontinued",
		},
		{
			ProductCode: "6ES7216-2AD23-0XB8",
			OldStatus:   "Active",
			NewStatus:   "Discontinued",
		},
	}

	// Send the email
	t.Log("Sending test email to:", recipients)

	// We need to temporarily override the recipients in sendStatusChangeEmail
	// Since we can't modify it, let's send directly using the same format

	subject := "Mudança de Lifecycle de equipamentos detectada"

	// Build plain text version (same as scheduler)
	var textBuilder string
	textBuilder = "Os seguintes equipamentos mudaram o  lifecycle status:\n\n"
	for _, change := range changes {
		textBuilder += "Product: " + change.ProductCode + "\n"
		textBuilder += "  Old Status: " + change.OldStatus + "\n"
		textBuilder += "  New Status: " + change.NewStatus + "\n\n"
	}

	// Build HTML version (same as scheduler)
	htmlBuilder := `<!DOCTYPE html>
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; }
		table { border-collapse: collapse; width: 100%; margin-top: 20px; }
		th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
		th { background-color: #4CAF50; color: white; }
		tr:nth-child(even) { background-color: #f2f2f2; }
		h2 { color: #333; }
	</style>
</head>
<body>
	<h2>Lifecycle Status Changes Detected</h2>
	<p>The following products have changed their lifecycle status:</p>
	<table>
		<tr>
			<th>Product Code</th>
			<th>Status Antigo</th>
			<th>Novo Status</th>
		</tr>`

	for _, change := range changes {
		htmlBuilder += "<tr>"
		htmlBuilder += "<td>" + change.ProductCode + "</td>"
		htmlBuilder += "<td>" + change.OldStatus + "</td>"
		htmlBuilder += "<td>" + change.NewStatus + "</td>"
		htmlBuilder += "</tr>"
	}

	htmlBuilder += `
	</table>
</body>
</html>`

	err = scheduler.email.Send(subject, textBuilder, htmlBuilder, recipients)
	if err != nil {
		t.Fatalf("Failed to send email: %v", err)
	}

	t.Log("Email sent successfully! Check your inbox at:", recipients)
}

// TestSendRealEmail_WithEnvFile loads .env file and sends email
// Run with: go test -v -tags=integration ./internal/scheduler/... -run TestSendRealEmail_WithEnvFile
func TestSendRealEmail_WithEnvFile(t *testing.T) {
	// Try to load from .env file using viper
	// This is useful if you have your credentials in .env

	// First check if env vars are already set
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	if smtpHost == "" || smtpUser == "" || smtpPass == "" {
		t.Skip("Skipping: Set SMTP_HOST, SMTP_USER, and SMTP_PASS environment variables")
	}

	smtpPort, err := strconv.Atoi(smtpPortStr)
	if err != nil {
		smtpPort = 587
	}

	// Use your email for testing
	recipient := os.Getenv("TEST_EMAIL_RECIPIENT")
	if recipient == "" {
		recipient = "freitasmatheusrn@gmail.com" // Default to your email
	}

	emailClient := smtp.New(smtpHost, smtpUser, smtpPass, smtpPort)
	logger, _ := zap.NewDevelopment()

	scheduler := &Scheduler{
		logger: logger,
		email:  emailClient,
	}

	changes := []products.LifecycleStatusChange{
		{
			ProductCode: "TEST-001",
			OldStatus:   "Active",
			NewStatus:   "Phase Out",
		},
	}

	// Use the actual sendStatusChangeEmail method
	scheduler.sendStatusChangeEmail(changes)

	t.Log("Email sent using scheduler.sendStatusChangeEmail() - check inbox")
}
