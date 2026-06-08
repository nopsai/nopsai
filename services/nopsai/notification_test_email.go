package nopsai

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"
)

type notificationMailTestView struct {
	Brand          pipelineNotificationBranding
	PreviewText    string
	Headline       string
	Summary        string
	SMTPServer     string
	Security       string
	Authentication string
	Sender         string
	Recipient      string
	Environment    string
	GeneratedAt    string
	Note           string
	OpenURL        string
	WebsiteDisplay string
	SupportDisplay string
}

func (a *App) renderNotificationMailTest(settings notificationMailSettingsFile, recipient, subject, note string) (notificationMailMessage, error) {
	branding := a.pipelineNotificationBranding()
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "[NopsAI] Mail delivery test successful"
	}
	environment := "Not specified"
	if a != nil && a.cfg != nil {
		if configured := strings.TrimSpace(a.getConfigSnapshot().Environment); configured != "" {
			environment = configured
		}
	}
	view := notificationMailTestView{
		Brand:          branding,
		PreviewText:    "NopsAI successfully delivered this SMTP configuration test.",
		Headline:       "Mail delivery is working",
		Summary:        "This message confirms that NopsAI can connect, authenticate, and deliver mail using the saved notification settings.",
		SMTPServer:     fmt.Sprintf("%s:%d", settings.SMTP.Host, settings.SMTP.Port),
		Security:       notificationMailSecurityLabel(settings.SMTP),
		Authentication: notificationMailAuthenticationLabel(settings.SMTP),
		Sender:         settings.From,
		Recipient:      recipient,
		Environment:    environment,
		GeneratedAt:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		Note:           strings.TrimSpace(note),
		OpenURL:        branding.PublicURL,
		WebsiteDisplay: notificationURLDisplay(branding.WebsiteURL),
		SupportDisplay: notificationURLDisplay(branding.SupportURL),
	}

	var htmlBody bytes.Buffer
	if err := notificationMailTestHTMLTemplate.Execute(&htmlBody, view); err != nil {
		return notificationMailMessage{}, fmt.Errorf("failed to render mail test email: %w", err)
	}
	return notificationMailMessage{
		FromName: branding.Name,
		Subject:  subject,
		TextBody: buildNotificationMailTestTextBody(view),
		HTMLBody: htmlBody.String(),
	}, nil
}

func notificationMailSecurityLabel(settings notificationMailSMTPSettings) string {
	switch {
	case settings.Port == 465 && !settings.StartTLS:
		return "Implicit TLS"
	case settings.StartTLS:
		return "STARTTLS required"
	default:
		return "Unencrypted SMTP"
	}
}

func notificationMailAuthenticationLabel(settings notificationMailSMTPSettings) string {
	if strings.TrimSpace(settings.Username) == "" {
		return "Not configured"
	}
	return "Username and secret reference configured"
}

func buildNotificationMailTestTextBody(view notificationMailTestView) string {
	lines := []string{
		"MAIL DELIVERY VERIFIED",
		view.Headline,
		"",
		view.Summary,
		"",
		"Configuration:",
		"SMTP server: " + view.SMTPServer,
		"Security: " + view.Security,
		"Authentication: " + view.Authentication,
		"Sender: " + view.Sender,
		"Recipient: " + view.Recipient,
		"Environment: " + view.Environment,
		"Generated: " + view.GeneratedAt,
	}
	if view.Note != "" {
		lines = append(lines, "", "Note:", view.Note)
	}
	if view.OpenURL != "" {
		lines = append(lines, "", "Open NopsAI: "+view.OpenURL)
	}
	if view.Brand.WebsiteURL != "" {
		lines = append(lines, "Website: "+view.Brand.WebsiteURL)
	}
	if view.Brand.SupportURL != "" {
		lines = append(lines, "Support: "+view.Brand.SupportURL)
	}
	lines = append(lines, "", "This was a configuration test, not a pipeline notification.")
	if view.Brand.Address != "" {
		lines = append(lines, view.Brand.Address)
	}
	return strings.Join(lines, "\n")
}

var notificationMailTestHTMLTemplate = template.Must(template.New("notification-mail-test").Parse(`<!doctype html>
<html>
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
    <title>{{.Headline}}</title>
  </head>
  <body style="margin:0;padding:0;background:#f2f4f7;color:#101828;font-family:Inter,Segoe UI,Arial,sans-serif;">
    <div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">{{.PreviewText}}</div>
    <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f2f4f7;">
      <tr>
        <td align="center" style="padding:32px 12px;">
          <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:640px;background:#ffffff;border:1px solid #e4e7ec;border-radius:16px;overflow:hidden;box-shadow:0 4px 14px rgba(16,24,40,.06);">
            <tr>
              <td style="padding:20px 28px;border-bottom:1px solid #eaecf0;">
                {{if .Brand.LogoURL}}<img src="{{.Brand.LogoURL}}" width="112" alt="{{.Brand.Name}}" style="display:block;width:112px;height:auto;border:0;">{{else}}<div style="font-size:23px;line-height:28px;font-weight:800;letter-spacing:-.5px;color:#101828;">{{.Brand.Name}}</div>{{end}}
              </td>
            </tr>
            <tr>
              <td style="padding:30px 28px;background:#16794b;color:#ffffff;">
                <div style="font-size:12px;line-height:18px;font-weight:800;letter-spacing:1.3px;">CONFIGURATION VERIFIED</div>
                <h1 style="margin:7px 0 6px;font-size:28px;line-height:35px;font-weight:750;">{{.Headline}}</h1>
                <p style="margin:0;font-size:14px;line-height:22px;opacity:.94;">{{.Summary}}</p>
              </td>
            </tr>
            <tr>
              <td style="padding:26px 28px 8px;">
                <div style="padding:16px 18px;background:#ecfdf3;border:1px solid #abefc6;border-left:4px solid #17b26a;border-radius:10px;">
                  <div style="font-size:14px;line-height:21px;font-weight:700;color:#067647;">SMTP delivery completed successfully</div>
                  <div style="margin-top:4px;font-size:12px;line-height:18px;color:#067647;">Receiving this message confirms the full outbound delivery path.</div>
                </div>
              </td>
            </tr>
            <tr>
              <td style="padding:20px 28px 8px;">
                <h2 style="margin:0 0 10px;font-size:16px;line-height:24px;">Verified configuration</h2>
                <table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="font-size:13px;line-height:20px;border:1px solid #eaecf0;border-radius:10px;">
                  <tr><td style="padding:11px 14px;color:#667085;border-bottom:1px solid #eaecf0;">SMTP server</td><td style="padding:11px 14px;border-bottom:1px solid #eaecf0;font-family:SFMono-Regular,Consolas,monospace;">{{.SMTPServer}}</td></tr>
                  <tr><td style="padding:11px 14px;color:#667085;border-bottom:1px solid #eaecf0;">Security</td><td style="padding:11px 14px;border-bottom:1px solid #eaecf0;">{{.Security}}</td></tr>
                  <tr><td style="padding:11px 14px;color:#667085;border-bottom:1px solid #eaecf0;">Authentication</td><td style="padding:11px 14px;border-bottom:1px solid #eaecf0;">{{.Authentication}}</td></tr>
                  <tr><td style="padding:11px 14px;color:#667085;border-bottom:1px solid #eaecf0;">Sender</td><td style="padding:11px 14px;border-bottom:1px solid #eaecf0;word-break:break-word;">{{.Sender}}</td></tr>
                  <tr><td style="padding:11px 14px;color:#667085;border-bottom:1px solid #eaecf0;">Recipient</td><td style="padding:11px 14px;border-bottom:1px solid #eaecf0;word-break:break-word;">{{.Recipient}}</td></tr>
                  <tr><td style="padding:11px 14px;color:#667085;border-bottom:1px solid #eaecf0;">Environment</td><td style="padding:11px 14px;border-bottom:1px solid #eaecf0;">{{.Environment}}</td></tr>
                  <tr><td style="padding:11px 14px;color:#667085;">Generated</td><td style="padding:11px 14px;">{{.GeneratedAt}}</td></tr>
                </table>
              </td>
            </tr>
            {{if .Note}}
            <tr>
              <td style="padding:18px 28px 4px;">
                <h2 style="margin:0 0 8px;font-size:16px;line-height:24px;">Test note</h2>
                <div style="padding:14px 16px;background:#f9fafb;border:1px solid #eaecf0;border-radius:10px;font-size:13px;line-height:21px;color:#344054;white-space:pre-wrap;">{{.Note}}</div>
              </td>
            </tr>
            {{end}}
            {{if .OpenURL}}
            <tr>
              <td style="padding:22px 28px 28px;">
                <a href="{{.OpenURL}}" style="display:inline-block;padding:11px 17px;border-radius:8px;background:#175cd3;color:#ffffff;text-decoration:none;font-size:13px;font-weight:700;">Open NopsAI</a>
              </td>
            </tr>
            {{end}}
            <tr>
              <td align="center" style="padding:24px 28px;background:#f9fafb;border-top:1px solid #eaecf0;color:#667085;font-size:12px;line-height:19px;">
                {{if .Brand.LogoURL}}<img src="{{.Brand.LogoURL}}" width="76" alt="{{.Brand.Name}}" style="display:block;margin:0 auto 10px;width:76px;height:auto;border:0;">{{else}}<div style="margin-bottom:8px;font-size:17px;font-weight:800;color:#344054;">{{.Brand.Name}}</div>{{end}}
                <div>This was a configuration test, not a pipeline notification.</div>
                {{if .Brand.Address}}<div>{{.Brand.Address}}</div>{{end}}
                {{if or .Brand.WebsiteURL .Brand.SupportURL}}<div style="margin-top:6px;">{{if .Brand.WebsiteURL}}<a href="{{.Brand.WebsiteURL}}" style="color:#175cd3;text-decoration:none;">{{.WebsiteDisplay}}</a>{{end}}{{if and .Brand.WebsiteURL .Brand.SupportURL}} &nbsp;|&nbsp; {{end}}{{if .Brand.SupportURL}}<a href="{{.Brand.SupportURL}}" style="color:#175cd3;text-decoration:none;">Support</a>{{end}}</div>{{end}}
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`))
