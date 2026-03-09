package handlers

import (
	"bytes"
	"fmt"
	"html"
	"log"
	"net/smtp"
	"time"

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
            <h1 style="margin:0;color:#fff;font-size:1.5rem;font-weight:700;letter-spacing:-0.02em;">Fluentica</h1>
            <p style="margin:4px 0 0;color:rgba(255,255,255,0.8);font-size:0.875rem;">Your AI-powered language tutor</p>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding:40px;">
            <h2 style="margin:0 0 16px;color:#f0f0f8;font-size:1.25rem;font-weight:600;">Verify your email address</h2>
            <p style="margin:0 0 12px;color:#a0a0b8;line-height:1.6;">Hi %s,</p>
            <p style="margin:0 0 28px;color:#a0a0b8;line-height:1.6;">
              Thanks for signing up! Click the button below to verify your email address and complete your Fluentica account setup.
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
              This link will expire once used. If you didn't create a Fluentica account, you can safely ignore this email.
            </p>
          </td>
        </tr>
        <!-- Footer -->
        <tr>
          <td style="padding:20px 40px;border-top:1px solid #2a2a38;text-align:center;">
            <p style="margin:0;color:#404058;font-size:0.75rem;">© 2025 Fluentica · All rights reserved</p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, safeUsername, safeURL, safeURL, safeURL)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: Fluentica <%s>\r\n", cfg.EmailFrom)
	fmt.Fprintf(&msg, "To: %s\r\n", toEmail)
	fmt.Fprintf(&msg, "Subject: Verify your Fluentica email address\r\n")
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "\r\n")
	msg.WriteString(htmlBody)

	addr := cfg.SMTPHost + ":" + cfg.SMTPPort
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)

	return smtp.SendMail(addr, auth, cfg.EmailFrom, []string{toEmail}, msg.Bytes())
}

// sendBetaInviteEmail sends an invite email to a beta tester with a password-set link.
// Returns nil when SMTP is unconfigured (logs URL to stdout instead).
// Returns a non-nil error only when SMTP IS configured but the send fails.
func sendBetaInviteEmail(cfg *config.Config, toEmail, username, resetURL string, trialEndsAt time.Time) error {
	if cfg.SMTPHost == "" || cfg.EmailFrom == "" {
		log.Printf("[INVITE] SMTP not configured. Share this set-password URL manually with %s: %s", toEmail, resetURL)
		return nil
	}

	safeUsername := html.EscapeString(username)
	safeURL := html.EscapeString(resetURL)
	trialDate := trialEndsAt.Format("January 2, 2006")

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#0f0f13;font-family:'Segoe UI',Arial,sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background:#0f0f13;padding:40px 20px;">
    <tr><td align="center">
      <table width="560" cellpadding="0" cellspacing="0" style="background:#1a1a24;border-radius:16px;overflow:hidden;max-width:560px;width:100%%;">
        <tr>
          <td style="background:linear-gradient(135deg,#7c3aed,#2563eb);padding:32px 40px;text-align:center;">
            <div style="font-size:2rem;margin-bottom:8px;">🌐</div>
            <h1 style="margin:0;color:#fff;font-size:1.5rem;font-weight:700;letter-spacing:-0.02em;">Fluentica</h1>
            <p style="margin:4px 0 0;color:rgba(255,255,255,0.8);font-size:0.875rem;">Your AI-powered language tutor</p>
          </td>
        </tr>
        <tr>
          <td style="padding:40px;">
            <h2 style="margin:0 0 16px;color:#f0f0f8;font-size:1.25rem;font-weight:600;">You're invited to try Fluentica! 🎉</h2>
            <p style="margin:0 0 12px;color:#a0a0b8;line-height:1.6;">Hi %s,</p>
            <p style="margin:0 0 20px;color:#a0a0b8;line-height:1.6;">
              You've been given <strong style="color:#f0f0f8;">30 days of free beta access</strong> to Fluentica — an AI-powered language tutor for Italian, Spanish, and Portuguese. No credit card required.
            </p>
            <div style="background:#12122a;border:1px solid #2a2a40;border-radius:10px;padding:16px 20px;margin:0 0 28px;">
              <p style="margin:0 0 4px;color:#7c3aed;font-size:0.8rem;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;">Beta Trial</p>
              <p style="margin:0;color:#f0f0f8;font-size:1rem;font-weight:600;">Full access · Free until %s</p>
            </div>
            <p style="margin:0 0 24px;color:#a0a0b8;line-height:1.6;">
              Click the button below to set your password and start learning:
            </p>
            <table cellpadding="0" cellspacing="0" style="margin:0 auto 28px;">
              <tr>
                <td align="center" style="background:linear-gradient(135deg,#7c3aed,#2563eb);border-radius:10px;">
                  <a href="%s" style="display:inline-block;padding:14px 32px;color:#fff;font-weight:600;font-size:1rem;text-decoration:none;letter-spacing:0.01em;">
                    Set My Password &amp; Get Started →
                  </a>
                </td>
              </tr>
            </table>
            <p style="margin:0 0 8px;color:#606080;font-size:0.8rem;line-height:1.5;">
              If the button doesn't work, copy and paste this link:
            </p>
            <p style="margin:0 0 28px;word-break:break-all;">
              <a href="%s" style="color:#7c3aed;font-size:0.8rem;">%s</a>
            </p>
            <p style="margin:0;color:#606080;font-size:0.8rem;line-height:1.5;">
              This link expires in 48 hours. After your trial ends, you'll have the option to subscribe and keep all your progress.
            </p>
          </td>
        </tr>
        <tr>
          <td style="padding:20px 40px;border-top:1px solid #2a2a38;text-align:center;">
            <p style="margin:0;color:#404058;font-size:0.75rem;">© 2025 Fluentica · All rights reserved</p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, safeUsername, trialDate, safeURL, safeURL, safeURL)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: Fluentica <%s>\r\n", cfg.EmailFrom)
	fmt.Fprintf(&msg, "To: %s\r\n", toEmail)
	fmt.Fprintf(&msg, "Subject: You're invited — 30-day free trial of Fluentica\r\n")
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "\r\n")
	msg.WriteString(htmlBody)

	addr := cfg.SMTPHost + ":" + cfg.SMTPPort
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	return smtp.SendMail(addr, auth, cfg.EmailFrom, []string{toEmail}, msg.Bytes())
}

// sendPasswordResetEmail sends an HTML password reset email via SMTP.
func sendPasswordResetEmail(cfg *config.Config, toEmail, username, resetURL string) error {
	if cfg.SMTPHost == "" || cfg.EmailFrom == "" {
		log.Printf("SMTP not configured — skipping reset email to %s. Reset URL: %s", toEmail, resetURL)
		return nil
	}

	safeUsername := html.EscapeString(username)
	safeURL := html.EscapeString(resetURL)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#0f0f13;font-family:'Segoe UI',Arial,sans-serif;">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background:#0f0f13;padding:40px 20px;">
    <tr><td align="center">
      <table width="560" cellpadding="0" cellspacing="0" style="background:#1a1a24;border-radius:16px;overflow:hidden;max-width:560px;width:100%%;">
        <tr>
          <td style="background:linear-gradient(135deg,#7c3aed,#2563eb);padding:32px 40px;text-align:center;">
            <div style="font-size:2rem;margin-bottom:8px;">🌐</div>
            <h1 style="margin:0;color:#fff;font-size:1.5rem;font-weight:700;letter-spacing:-0.02em;">Fluentica</h1>
            <p style="margin:4px 0 0;color:rgba(255,255,255,0.8);font-size:0.875rem;">Your AI-powered language tutor</p>
          </td>
        </tr>
        <tr>
          <td style="padding:40px;">
            <h2 style="margin:0 0 16px;color:#f0f0f8;font-size:1.25rem;font-weight:600;">Reset your password</h2>
            <p style="margin:0 0 12px;color:#a0a0b8;line-height:1.6;">Hi %s,</p>
            <p style="margin:0 0 28px;color:#a0a0b8;line-height:1.6;">
              We received a request to reset your Fluentica password. Click the button below to choose a new password. This link expires in 1 hour.
            </p>
            <table cellpadding="0" cellspacing="0" style="margin:0 auto 28px;">
              <tr>
                <td align="center" style="background:linear-gradient(135deg,#7c3aed,#2563eb);border-radius:10px;">
                  <a href="%s" style="display:inline-block;padding:14px 32px;color:#fff;font-weight:600;font-size:1rem;text-decoration:none;letter-spacing:0.01em;">
                    Reset Password →
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
              If you didn't request a password reset, you can safely ignore this email. Your password won't change.
            </p>
          </td>
        </tr>
        <tr>
          <td style="padding:20px 40px;border-top:1px solid #2a2a38;text-align:center;">
            <p style="margin:0;color:#404058;font-size:0.75rem;">© 2025 Fluentica · All rights reserved</p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, safeUsername, safeURL, safeURL, safeURL)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: Fluentica <%s>\r\n", cfg.EmailFrom)
	fmt.Fprintf(&msg, "To: %s\r\n", toEmail)
	fmt.Fprintf(&msg, "Subject: Reset your Fluentica password\r\n")
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "\r\n")
	msg.WriteString(htmlBody)

	addr := cfg.SMTPHost + ":" + cfg.SMTPPort
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)

	return smtp.SendMail(addr, auth, cfg.EmailFrom, []string{toEmail}, msg.Bytes())
}
