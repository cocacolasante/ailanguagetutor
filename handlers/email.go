package handlers

import (
	"bytes"
	"fmt"
	"html"
	"log"
	"net/smtp"

	"github.com/ailanguagetutor/config"
)

// sendVerificationEmail sends an HTML verification email via SMTP.
// If SMTP is not configured (no SMTP_HOST), it logs the verify URL instead
// so developers can test locally without a mail server.
func sendVerificationEmail(cfg *config.Config, toEmail, username, verifyURL string) error {
	if cfg.SMTPHost == "" || cfg.EmailFrom == "" {
		log.Printf("SMTP not configured — skipping email to %s. Verify URL: %s", toEmail, verifyURL)
		return nil
	}

	safeUsername := html.EscapeString(username)
	safeURL := html.EscapeString(verifyURL)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#0f0f13;font-family:'Segoe UI',Arial,sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background:#0f0f13;padding:40px 20px;">
    <tr><td align="center">
      <table width="560" cellpadding="0" cellspacing="0" style="background:#1a1a24;border-radius:16px;overflow:hidden;max-width:560px;width:100%%;">
        <!-- Header -->
        <tr>
          <td style="background:linear-gradient(135deg,#7c3aed,#2563eb);padding:32px 40px;text-align:center;">
            <div style="font-size:2rem;margin-bottom:8px;">🌐</div>
            <h1 style="margin:0;color:#fff;font-size:1.5rem;font-weight:700;letter-spacing:-0.02em;">LinguaAI</h1>
            <p style="margin:4px 0 0;color:rgba(255,255,255,0.8);font-size:0.875rem;">Your AI-powered language tutor</p>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding:40px;">
            <h2 style="margin:0 0 16px;color:#f0f0f8;font-size:1.25rem;font-weight:600;">Verify your email address</h2>
            <p style="margin:0 0 12px;color:#a0a0b8;line-height:1.6;">Hi %s,</p>
            <p style="margin:0 0 28px;color:#a0a0b8;line-height:1.6;">
              Thanks for signing up! Click the button below to verify your email address and complete your LinguaAI account setup.
            </p>
            <table cellpadding="0" cellspacing="0" style="margin:0 auto 28px;">
              <tr>
                <td align="center" style="background:linear-gradient(135deg,#7c3aed,#2563eb);border-radius:10px;">
                  <a href="%s" style="display:inline-block;padding:14px 32px;color:#fff;font-weight:600;font-size:1rem;text-decoration:none;letter-spacing:0.01em;">
                    Verify Email →
                  </a>
                </td>
              </tr>
            </table>
            <p style="margin:0 0 8px;color:#606080;font-size:0.8rem;line-height:1.5;">
              If the button doesn't work, copy and paste this link into your browser:
            </p>
            <p style="margin:0 0 28px;word-break:break-all;">
              <a href="%s" style="color:#7c3aed;font-size:0.8rem;">%s</a>
            </p>
            <p style="margin:0;color:#606080;font-size:0.8rem;line-height:1.5;">
              This link will expire once used. If you didn't create a LinguaAI account, you can safely ignore this email.
            </p>
          </td>
        </tr>
        <!-- Footer -->
        <tr>
          <td style="padding:20px 40px;border-top:1px solid #2a2a38;text-align:center;">
            <p style="margin:0;color:#404058;font-size:0.75rem;">© 2025 LinguaAI · All rights reserved</p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, safeUsername, safeURL, safeURL, safeURL)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: LinguaAI <%s>\r\n", cfg.EmailFrom)
	fmt.Fprintf(&msg, "To: %s\r\n", toEmail)
	fmt.Fprintf(&msg, "Subject: Verify your LinguaAI email address\r\n")
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "\r\n")
	msg.WriteString(htmlBody)

	addr := cfg.SMTPHost + ":" + cfg.SMTPPort
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)

	return smtp.SendMail(addr, auth, cfg.EmailFrom, []string{toEmail}, msg.Bytes())
}
