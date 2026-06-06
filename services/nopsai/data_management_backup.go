package nopsai

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) createDataBackup(ctx context.Context, backupType, requestedBy string) (dataBackupRecord, error) {
	backupType, err := normalizeDataBackupType(backupType)
	if err != nil {
		return dataBackupRecord{}, err
	}
	backupID := uuid.NewString()
	now := time.Now().UTC()
	fileName := fmt.Sprintf("nopsai-%s-%s-%s.jsonl.gz", backupType, now.Format("20060102T150405Z"), backupID[:8])
	dir := a.dataBackupDirectory()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return dataBackupRecord{}, err
	}
	filePath := filepath.Join(dir, fileName)

	if _, err := a.db.Exec(ctx, `
		INSERT INTO data_backups (id, backup_type, status, file_path, file_name, requested_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, backupID, backupType, dataBackupStatusRunning, filePath, fileName, strings.TrimSpace(requestedBy), now); err != nil {
		return dataBackupRecord{}, err
	}

	sizeBytes, checksum, err := a.writeDataBackupFile(ctx, backupID, backupType, filePath)
	if err != nil {
		_ = os.Remove(filePath)
		_, _ = a.db.Exec(ctx, `
			UPDATE data_backups
			SET status = $2, error = $3, completed_at = NOW()
			WHERE id::text = $1
		`, backupID, dataBackupStatusFailure, err.Error())
		record, _ := a.getDataBackup(ctx, backupID)
		return record, err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE data_backups
		SET status = $2,
			size_bytes = $3,
			checksum_sha256 = $4,
			completed_at = NOW()
		WHERE id::text = $1
	`, backupID, dataBackupStatusSuccess, sizeBytes, checksum); err != nil {
		return dataBackupRecord{}, err
	}
	return a.getDataBackup(ctx, backupID)
}

func (a *App) writeDataBackupFile(ctx context.Context, backupID, backupType, filePath string) (int64, string, error) {
	tables, err := a.dataBackupTables(ctx, backupType)
	if err != nil {
		return 0, "", err
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()

	hasher := sha256.New()
	counter := &countingWriter{}
	gzipWriter := gzip.NewWriter(io.MultiWriter(file, hasher, counter))
	gzipWriter.Name = filepath.Base(filePath)
	gzipWriter.ModTime = time.Now().UTC()
	encoder := json.NewEncoder(gzipWriter)

	if err := encoder.Encode(backupJSONLine{
		Kind:        "manifest",
		BackupID:    backupID,
		BackupType:  backupType,
		GeneratedAt: time.Now().UTC(),
		Tables:      tables,
	}); err != nil {
		gzipWriter.Close()
		return 0, "", err
	}
	for _, table := range tables {
		if err := encoder.Encode(backupJSONLine{Kind: "table", Table: table}); err != nil {
			gzipWriter.Close()
			return 0, "", err
		}
		if err := a.writeDataBackupTable(ctx, encoder, table); err != nil {
			gzipWriter.Close()
			return 0, "", err
		}
	}
	if err := gzipWriter.Close(); err != nil {
		return 0, "", err
	}
	return counter.count, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (a *App) writeDataBackupTable(ctx context.Context, encoder *json.Encoder, table string) error {
	query := fmt.Sprintf(`SELECT row_to_json(t)::text FROM (SELECT * FROM %s) t`, quoteSQLIdentifier(table))
	rows, err := a.db.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rowJSON string
		if err := rows.Scan(&rowJSON); err != nil {
			return err
		}
		if err := encoder.Encode(backupJSONLine{Kind: "row", Table: table, Row: json.RawMessage(rowJSON)}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (a *App) dataBackupTables(ctx context.Context, backupType string) ([]string, error) {
	switch backupType {
	case dataBackupTypeRuns:
		return runBackupTables, nil
	case dataBackupTypeLogs:
		return []string{"pipeline_run_logs"}, nil
	case dataBackupTypeFull:
		rows, err := a.db.Query(ctx, `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_type = 'BASE TABLE'
			ORDER BY table_name ASC
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var tables []string
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				return nil, err
			}
			tables = append(tables, table)
		}
		return tables, rows.Err()
	default:
		return nil, fmt.Errorf("unsupported backup type %q", backupType)
	}
}
