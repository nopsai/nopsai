package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
)

const notificationMailGitOpsPath = "system/mail.yaml"

type notificationMailSMTPSettings struct {
	Host              string `json:"host" yaml:"host,omitempty"`
	Port              int    `json:"port" yaml:"port,omitempty"`
	StartTLS          bool   `json:"start_tls" yaml:"start_tls"`
	Username          string `json:"username" yaml:"username,omitempty"`
	PasswordSecretRef string `json:"password_secret_ref" yaml:"password_secret_ref,omitempty"`
}

type notificationMailSettingsFile struct {
	Enabled bool                         `json:"enabled" yaml:"enabled"`
	From    string                       `json:"from" yaml:"from,omitempty"`
	SMTP    notificationMailSMTPSettings `json:"smtp" yaml:"smtp,omitempty"`
}

type notificationMailSettingsRecord struct {
	notificationMailSettingsFile `yaml:",inline"`
	Source                       string     `json:"source,omitempty" yaml:"-"`
	ConfigRepoID                 *int64     `json:"config_repo_id,omitempty" yaml:"-"`
	ConfigSourcePath             string     `json:"config_source_path,omitempty" yaml:"-"`
	ConfigSourceCommitSHA        string     `json:"config_source_commit_sha,omitempty" yaml:"-"`
	ManagedByConfigRepo          bool       `json:"managed_by_config_repo" yaml:"-"`
	UpdatedAt                    *time.Time `json:"updated_at,omitempty" yaml:"-"`
}

type gitOpsMailSettingsPlan struct {
	settings   notificationMailSettingsFile
	sourcePath string
}

type notificationMailTestRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func defaultNotificationMailSettings() notificationMailSettingsFile {
	return notificationMailSettingsFile{
		Enabled: false,
		SMTP: notificationMailSMTPSettings{
			Port:     587,
			StartTLS: true,
		},
	}
}

func normalizeNotificationMailSettings(input notificationMailSettingsFile) (notificationMailSettingsFile, error) {
	settings := input
	settings.From = strings.TrimSpace(settings.From)
	settings.SMTP.Host = strings.TrimSpace(settings.SMTP.Host)
	settings.SMTP.Username = strings.TrimSpace(settings.SMTP.Username)
	settings.SMTP.PasswordSecretRef = strings.TrimSpace(settings.SMTP.PasswordSecretRef)
	if settings.SMTP.Port == 0 {
		settings.SMTP.Port = 587
	}
	if settings.SMTP.Port < 1 || settings.SMTP.Port > 65535 {
		return notificationMailSettingsFile{}, fmt.Errorf("smtp.port must be between 1 and 65535")
	}
	if settings.Enabled {
		if settings.From == "" {
			return notificationMailSettingsFile{}, fmt.Errorf("from is required when mail notifications are enabled")
		}
		if _, err := mail.ParseAddress(settings.From); err != nil {
			return notificationMailSettingsFile{}, fmt.Errorf("from must be a valid email address")
		}
		if settings.SMTP.Host == "" {
			return notificationMailSettingsFile{}, fmt.Errorf("smtp.host is required when mail notifications are enabled")
		}
		if settings.SMTP.Username != "" && settings.SMTP.PasswordSecretRef == "" {
			return notificationMailSettingsFile{}, fmt.Errorf("smtp.password_secret_ref is required when smtp.username is set")
		}
	}
	return settings, nil
}

func parseGitOpsMailSettingsPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsMailSettingsPlan, error) {
	var candidates []gitOpsRuntimeSettingsFileCandidate
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := relativeConfigPath(normalized, root)
			if !ok || !isGitOpsMailSettingsRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsRuntimeSettingsFileCandidate{sourcePath: normalized, content: content})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("mail settings can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		return nil, fmt.Errorf("multiple mail settings GitOps files found: %s", strings.Join(paths, ", "))
	}
	return parseGitOpsMailSettingsFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsMailSettingsRelativePath(rel string) bool {
	return strings.Trim(filepath.ToSlash(rel), "/") == notificationMailGitOpsPath
}

func parseGitOpsMailSettingsFile(content, sourcePath string) (*gitOpsMailSettingsPlan, error) {
	var file notificationMailSettingsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse mail settings GitOps file '%s': %w", sourcePath, err)
	}
	settings, err := normalizeNotificationMailSettings(file)
	if err != nil {
		return nil, fmt.Errorf("mail settings GitOps file '%s' is invalid: %w", sourcePath, err)
	}
	return &gitOpsMailSettingsPlan{settings: settings, sourcePath: sourcePath}, nil
}

func (a *App) handleGetNotificationMailSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.loadNotificationMailSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load mail notification settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *App) handleUpdateNotificationMailSettings(w http.ResponseWriter, r *http.Request) {
	var req notificationMailSettingsFile
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid mail notification settings payload", http.StatusBadRequest)
		return
	}
	settings, err := normalizeNotificationMailSettings(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.upsertNotificationMailSettings(r.Context(), settings, "database", nil, "", "", false)
	if err != nil {
		http.Error(w, "failed to save mail notification settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (a *App) handleTestNotificationMailSettings(w http.ResponseWriter, r *http.Request) {
	var req notificationMailTestRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid mail notification test payload", http.StatusBadRequest)
		return
	}
	to, err := normalizeNotificationTestRecipient(req.To)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings, err := a.loadNotificationMailSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load mail notification settings", http.StatusInternalServerError)
		return
	}
	if !settings.Enabled {
		http.Error(w, "mail notifications are disabled", http.StatusBadRequest)
		return
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "NopsAI mail notification test"
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = "NopsAI mail notifications are configured."
	}
	if err := sendNotificationMail(r.Context(), settings.notificationMailSettingsFile, []string{to}, subject, body); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func normalizeNotificationTestRecipient(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("to is required")
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("to must be a valid email address")
	}
	return strings.TrimSpace(parsed.Address), nil
}

func (a *App) loadNotificationMailSettings(ctx context.Context) (notificationMailSettingsRecord, error) {
	if a == nil || a.db == nil {
		return notificationMailSettingsRecord{notificationMailSettingsFile: defaultNotificationMailSettings(), Source: "database"}, nil
	}
	record, err := scanNotificationMailSettings(a.db.QueryRow(ctx, `
		SELECT enabled, from_address, smtp_host, smtp_port, smtp_start_tls, smtp_username,
		       smtp_password_secret_ref, COALESCE(source, 'database'), config_repo_id,
		       COALESCE(config_source_path, ''), COALESCE(config_source_commit_sha, ''),
		       managed_by_config_repo, updated_at
		FROM notification_mail_settings
		WHERE id = TRUE
	`))
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return notificationMailSettingsRecord{notificationMailSettingsFile: defaultNotificationMailSettings(), Source: "database"}, nil
	}
	return record, err
}

func (a *App) upsertNotificationMailSettings(ctx context.Context, settings notificationMailSettingsFile, source string, configRepoID *int64, sourcePath, commitSHA string, managed bool) (notificationMailSettingsRecord, error) {
	if a == nil || a.db == nil {
		return notificationMailSettingsRecord{notificationMailSettingsFile: settings, Source: source}, nil
	}
	return scanNotificationMailSettings(a.db.QueryRow(ctx, notificationMailSettingsUpsertSQL,
		settings.Enabled,
		settings.From,
		settings.SMTP.Host,
		settings.SMTP.Port,
		settings.SMTP.StartTLS,
		settings.SMTP.Username,
		settings.SMTP.PasswordSecretRef,
		source,
		configRepoID,
		sourcePath,
		commitSHA,
		managed,
	))
}

const notificationMailSettingsUpsertSQL = `
	INSERT INTO notification_mail_settings (
		id, enabled, from_address, smtp_host, smtp_port, smtp_start_tls, smtp_username,
		smtp_password_secret_ref, source, config_repo_id, config_source_path,
		config_source_commit_sha, managed_by_config_repo, updated_at
	) VALUES (
		TRUE, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW()
	)
	ON CONFLICT (id) DO UPDATE SET
		enabled = EXCLUDED.enabled,
		from_address = EXCLUDED.from_address,
		smtp_host = EXCLUDED.smtp_host,
		smtp_port = EXCLUDED.smtp_port,
		smtp_start_tls = EXCLUDED.smtp_start_tls,
		smtp_username = EXCLUDED.smtp_username,
		smtp_password_secret_ref = EXCLUDED.smtp_password_secret_ref,
		source = EXCLUDED.source,
		config_repo_id = EXCLUDED.config_repo_id,
		config_source_path = EXCLUDED.config_source_path,
		config_source_commit_sha = EXCLUDED.config_source_commit_sha,
		managed_by_config_repo = EXCLUDED.managed_by_config_repo,
		updated_at = NOW()
	RETURNING enabled, from_address, smtp_host, smtp_port, smtp_start_tls, smtp_username,
	          smtp_password_secret_ref, COALESCE(source, 'database'), config_repo_id,
	          COALESCE(config_source_path, ''), COALESCE(config_source_commit_sha, ''),
	          managed_by_config_repo, updated_at`

func scanNotificationMailSettings(row interface{ Scan(dest ...any) error }) (notificationMailSettingsRecord, error) {
	var record notificationMailSettingsRecord
	var configRepoID sql.NullInt64
	var updatedAt sql.NullTime
	err := row.Scan(
		&record.Enabled,
		&record.From,
		&record.SMTP.Host,
		&record.SMTP.Port,
		&record.SMTP.StartTLS,
		&record.SMTP.Username,
		&record.SMTP.PasswordSecretRef,
		&record.Source,
		&configRepoID,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
		&record.ManagedByConfigRepo,
		&updatedAt,
	)
	if err != nil {
		return notificationMailSettingsRecord{}, err
	}
	if configRepoID.Valid {
		id := configRepoID.Int64
		record.ConfigRepoID = &id
	}
	if updatedAt.Valid {
		record.UpdatedAt = &updatedAt.Time
	}
	settings, err := normalizeNotificationMailSettings(record.notificationMailSettingsFile)
	if err != nil {
		return notificationMailSettingsRecord{}, err
	}
	record.notificationMailSettingsFile = settings
	return record, nil
}

func sendNotificationMail(ctx context.Context, settings notificationMailSettingsFile, recipients []string, subject, body string) error {
	settings, err := normalizeNotificationMailSettings(settings)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return fmt.Errorf("mail notifications are disabled")
	}
	password := ""
	if ref := strings.TrimSpace(settings.SMTP.PasswordSecretRef); ref != "" {
		password = os.Getenv(ref)
		if password == "" {
			return fmt.Errorf("smtp password secret %q is not available", ref)
		}
	}
	message := buildNotificationMailMessage(settings.From, recipients, subject, body)
	addr := net.JoinHostPort(settings.SMTP.Host, fmt.Sprintf("%d", settings.SMTP.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if ctx == nil {
		ctx = context.Background()
	}
	if settings.SMTP.Port == 465 && !settings.SMTP.StartTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: settings.SMTP.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		client, err := smtp.NewClient(conn, settings.SMTP.Host)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("failed to start SMTP session: %w", err)
		}
		return sendSMTPMessage(client, settings, password, recipients, message)
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	client, err := smtp.NewClient(conn, settings.SMTP.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to start SMTP session: %w", err)
	}
	return sendSMTPMessage(client, settings, password, recipients, message)
}

func sendSMTPMessage(client *smtp.Client, settings notificationMailSettingsFile, password string, recipients []string, message []byte) error {
	defer client.Close()
	if settings.SMTP.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: settings.SMTP.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("failed to enable SMTP STARTTLS: %w", err)
			}
		} else {
			return fmt.Errorf("SMTP server does not advertise STARTTLS")
		}
	}
	if settings.SMTP.Username != "" {
		auth := smtp.PlainAuth("", settings.SMTP.Username, password, settings.SMTP.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := client.Mail(settings.From); err != nil {
		return fmt.Errorf("failed to set SMTP sender: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set SMTP recipient %s: %w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open SMTP data stream: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("failed to write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to close SMTP session: %w", err)
	}
	return nil
}

func buildNotificationMailMessage(from string, to []string, subject, body string) []byte {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "NopsAI notification"
	}
	headers := []string{
		"From: " + from,
		"To: " + strings.Join(to, ", "),
		"Subject: " + encodeMailHeader(subject),
		"Content-Type: text/plain; charset=utf-8",
		"MIME-Version: 1.0",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n")
}

func encodeMailHeader(value string) string {
	if value == "" {
		return ""
	}
	if isASCII(value) {
		return value
	}
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}
