package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (a *App) resolveDashboardTeam(ctx context.Context, req dashboardRequest) (int, string, error) {
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return 0, "", err
	}
	if req.TeamID > 0 {
		record, ok := records[req.TeamID]
		if !ok {
			return 0, "", fmt.Errorf("team not found")
		}
		teamPath := strings.Trim(record.Path, "/")
		if requested := strings.Trim(strings.TrimSpace(req.TeamPath), "/"); requested != "" && requested != teamPath {
			return 0, "", fmt.Errorf("team_id and team_path refer to different teams")
		}
		return req.TeamID, teamPath, nil
	}
	teamPath := strings.Trim(strings.TrimSpace(req.TeamPath), "/")
	if teamPath == "" {
		return 0, "", fmt.Errorf("team_path is required")
	}
	for _, record := range records {
		if record.Path == teamPath {
			return record.ID, teamPath, nil
		}
	}
	return 0, "", fmt.Errorf("team not found")
}

func (a *App) resolveDashboardRefTeam(ctx context.Context, ref string) (teamID int, teamPath, slug string, err error) {
	teamPath, slug, err = splitDashboardRef(ref)
	if err != nil {
		return 0, "", "", err
	}
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return 0, "", "", err
	}
	for _, record := range records {
		if record.Path == teamPath {
			return record.ID, teamPath, slug, nil
		}
	}
	return 0, "", "", fmt.Errorf("team not found")
}

func (a *App) listDashboardRecords(ctx context.Context, teamFilter, queryFilter string) ([]dashboardRecord, error) {
	rows, err := a.db.Query(ctx, baseDashboardSelect()+`
		GROUP BY d.id
		ORDER BY d.updated_at DESC, d.slug ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teamPaths, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return nil, err
	}
	teamFilter = strings.Trim(strings.TrimSpace(teamFilter), "/")
	queryFilter = strings.ToLower(strings.TrimSpace(queryFilter))
	records := []dashboardRecord{}
	for rows.Next() {
		record, err := scanDashboardRecord(rows, teamPaths)
		if err != nil {
			return nil, err
		}
		if teamFilter != "" && record.TeamPath != teamFilter && !strings.HasPrefix(record.TeamPath, teamFilter+"/") {
			continue
		}
		if queryFilter != "" {
			haystack := strings.ToLower(strings.Join([]string{record.ref(), record.Title, record.Description}, " "))
			if !strings.Contains(haystack, queryFilter) {
				continue
			}
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getDashboardRecord(ctx context.Context, dashboardID string) (dashboardRecord, error) {
	dashboardID = strings.Trim(strings.TrimSpace(dashboardID), "/")
	if dashboardID == "" {
		return dashboardRecord{}, pgx.ErrNoRows
	}
	teamPaths, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return dashboardRecord{}, err
	}
	if looksLikeUUID(dashboardID) {
		return scanDashboardRecord(a.db.QueryRow(ctx, baseDashboardSelect()+` WHERE d.id::text = $1 GROUP BY d.id LIMIT 1`, dashboardID), teamPaths)
	}
	teamID, _, slug, err := a.resolveDashboardRefTeam(ctx, dashboardID)
	if err != nil {
		return dashboardRecord{}, err
	}
	return scanDashboardRecord(a.db.QueryRow(ctx, baseDashboardSelect()+` WHERE d.team_id = $1 AND d.slug = $2 GROUP BY d.id LIMIT 1`, teamID, slug), teamPaths)
}

func (a *App) createDashboard(ctx context.Context, input dashboardInput, actor string) (dashboardRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return dashboardRecord{}, err
	}
	defer tx.Rollback(ctx)

	refreshPolicyJSON, err := json.Marshal(input.RefreshPolicy)
	if err != nil {
		return dashboardRecord{}, err
	}
	var dashboardID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboards (team_id, slug, title, description, visibility, refresh_policy, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $7)
		RETURNING id::text
	`, input.TeamID, input.Slug, input.Title, input.Description, input.Visibility, string(refreshPolicyJSON), actor).Scan(&dashboardID)
	if err != nil {
		return dashboardRecord{}, err
	}
	sections := input.Sections
	if len(sections) == 0 {
		sections = []dashboardSectionInput{{
			SectionKey:   "overview",
			Title:        "Overview",
			Layout:       map[string]any{},
			DisplayOrder: 0,
		}}
	}
	for _, section := range sections {
		if err := upsertDashboardSection(ctx, tx, dashboardID, section); err != nil {
			return dashboardRecord{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return dashboardRecord{}, err
	}
	return a.getDashboardRecord(ctx, dashboardID)
}

func (a *App) updateDashboard(ctx context.Context, dashboardID string, input dashboardInput, actor string) (dashboardRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return dashboardRecord{}, err
	}
	defer tx.Rollback(ctx)

	refreshPolicyJSON, err := json.Marshal(input.RefreshPolicy)
	if err != nil {
		return dashboardRecord{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE dashboards
		SET team_id = $2,
			slug = $3,
			title = $4,
			description = $5,
			visibility = $6,
			refresh_policy = $7::jsonb,
			source = 'database',
			config_repo_id = NULL,
			config_source_path = '',
			config_source_commit_sha = '',
			managed_by_config_repo = FALSE,
			updated_by = $8,
			updated_at = NOW()
		WHERE id::text = $1
	`, dashboardID, input.TeamID, input.Slug, input.Title, input.Description, input.Visibility, string(refreshPolicyJSON), actor)
	if err != nil {
		return dashboardRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return dashboardRecord{}, pgx.ErrNoRows
	}
	for _, section := range input.Sections {
		if err := upsertDashboardSection(ctx, tx, dashboardID, section); err != nil {
			return dashboardRecord{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return dashboardRecord{}, err
	}
	return a.getDashboardRecord(ctx, dashboardID)
}

func (a *App) deleteDashboard(ctx context.Context, dashboardID string) error {
	tag, err := a.db.Exec(ctx, `DELETE FROM dashboards WHERE id::text = $1`, strings.TrimSpace(dashboardID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (a *App) listDashboardSections(ctx context.Context, dashboardID string) ([]dashboardSectionRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, dashboard_id::text, section_key, title, description, layout::text,
		       display_order, created_at, updated_at
		FROM dashboard_sections
		WHERE dashboard_id::text = $1
		ORDER BY display_order ASC, section_key ASC
	`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sections []dashboardSectionRecord
	for rows.Next() {
		section, err := scanDashboardSectionRecord(rows)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}
	return sections, rows.Err()
}

func (a *App) getDashboardSection(ctx context.Context, dashboardID, sectionID string) (dashboardSectionRecord, error) {
	return scanDashboardSectionRecord(a.db.QueryRow(ctx, `
		SELECT id::text, dashboard_id::text, section_key, title, description, layout::text,
		       display_order, created_at, updated_at
		FROM dashboard_sections
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboardID, strings.TrimSpace(sectionID)))
}

func (a *App) createDashboardSection(ctx context.Context, dashboardID string, input dashboardSectionInput) (dashboardSectionRecord, error) {
	layoutJSON, err := json.Marshal(input.Layout)
	if err != nil {
		return dashboardSectionRecord{}, err
	}
	var id string
	err = a.db.QueryRow(ctx, `
		INSERT INTO dashboard_sections (dashboard_id, section_key, title, description, layout, display_order, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, NOW())
		RETURNING id::text
	`, dashboardID, input.SectionKey, input.Title, input.Description, string(layoutJSON), input.DisplayOrder).Scan(&id)
	if err != nil {
		return dashboardSectionRecord{}, err
	}
	return a.getDashboardSection(ctx, dashboardID, id)
}

func (a *App) updateDashboardSection(ctx context.Context, dashboardID, sectionID string, input dashboardSectionInput) (dashboardSectionRecord, error) {
	layoutJSON, err := json.Marshal(input.Layout)
	if err != nil {
		return dashboardSectionRecord{}, err
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE dashboard_sections
		SET title = $3,
			description = $4,
			layout = $5::jsonb,
			display_order = $6,
			updated_at = NOW()
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboardID, strings.TrimSpace(sectionID), input.Title, input.Description, string(layoutJSON), input.DisplayOrder)
	if err != nil {
		return dashboardSectionRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return dashboardSectionRecord{}, pgx.ErrNoRows
	}
	return a.getDashboardSection(ctx, dashboardID, sectionID)
}

func (a *App) deleteDashboardSection(ctx context.Context, dashboardID, sectionID string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	section, err := scanDashboardSectionRecord(tx.QueryRow(ctx, `
		SELECT id::text, dashboard_id::text, section_key, title, description, layout::text,
		       display_order, created_at, updated_at
		FROM dashboard_sections
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboardID, strings.TrimSpace(sectionID)))
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM dashboard_refresh_schedules
		WHERE dashboard_id::text = $1
		  AND scope_type = 'section'
		  AND (
			scope->>'section_key' = $2
			OR COALESCE(scope->'section_keys', '[]'::jsonb) ? $2
		  )
	`, dashboardID, section.SectionKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		WITH section_sources AS (
			SELECT id::text AS source_id
			FROM dashboard_source_bindings
			WHERE dashboard_id::text = $1 AND section_key = $2
		)
		DELETE FROM dashboard_refresh_schedules schedule
		USING section_sources source
		WHERE schedule.dashboard_id::text = $1
		  AND schedule.scope_type = 'source'
		  AND (
			schedule.scope->>'source_id' = source.source_id
			OR COALESCE(schedule.scope->'source_ids', '[]'::jsonb) ? source.source_id
		  )
	`, dashboardID, section.SectionKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dashboard_source_bindings WHERE dashboard_id::text = $1 AND section_key = $2`, dashboardID, section.SectionKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dashboard_publication_events WHERE dashboard_id::text = $1 AND section_key = $2`, dashboardID, section.SectionKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dashboard_publications WHERE dashboard_id::text = $1 AND section_key = $2`, dashboardID, section.SectionKey); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM dashboard_sections WHERE dashboard_id::text = $1 AND id::text = $2`, dashboardID, section.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func upsertDashboardSection(ctx context.Context, runner queryRunner, dashboardID string, input dashboardSectionInput) error {
	layoutJSON, err := json.Marshal(input.Layout)
	if err != nil {
		return err
	}
	_, err = runner.Exec(ctx, `
		INSERT INTO dashboard_sections (dashboard_id, section_key, title, description, layout, display_order, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, NOW())
		ON CONFLICT (dashboard_id, section_key) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			layout = EXCLUDED.layout,
			display_order = EXCLUDED.display_order,
			updated_at = NOW()
	`, dashboardID, input.SectionKey, input.Title, input.Description, string(layoutJSON), input.DisplayOrder)
	return err
}

func (a *App) listDashboardSources(ctx context.Context, dashboardID string) ([]dashboardSourceRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, dashboard_id::text, section_key, pipeline_id, output_name, entry_key,
		       enabled, required_for_refresh, refresh_order, created_at, updated_at
		FROM dashboard_source_bindings
		WHERE dashboard_id::text = $1
		ORDER BY section_key ASC, refresh_order ASC, pipeline_id ASC, output_name ASC, entry_key ASC
	`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []dashboardSourceRecord
	for rows.Next() {
		source, err := scanDashboardSourceRecord(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (a *App) getDashboardSource(ctx context.Context, dashboardID, sourceID string) (dashboardSourceRecord, error) {
	return scanDashboardSourceRecord(a.db.QueryRow(ctx, `
		SELECT id::text, dashboard_id::text, section_key, pipeline_id, output_name, entry_key,
		       enabled, required_for_refresh, refresh_order, created_at, updated_at
		FROM dashboard_source_bindings
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboardID, sourceID))
}

func (a *App) saveDashboardSource(ctx context.Context, dashboardID, sourceID string, input dashboardSourceInput) (dashboardSourceRecord, error) {
	if sourceID == "" {
		var id string
		err := a.db.QueryRow(ctx, `
			INSERT INTO dashboard_source_bindings (
				dashboard_id, section_key, pipeline_id, output_name, entry_key,
				enabled, required_for_refresh, refresh_order, updated_at
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, NOW())
			ON CONFLICT (dashboard_id, section_key, pipeline_id, output_name, entry_key) DO UPDATE SET
				enabled = EXCLUDED.enabled,
				required_for_refresh = EXCLUDED.required_for_refresh,
				refresh_order = EXCLUDED.refresh_order,
				updated_at = NOW()
			RETURNING id::text
		`, dashboardID, input.SectionKey, input.PipelineID, input.OutputName, input.EntryKey, input.Enabled, input.RequiredForRefresh, input.RefreshOrder).Scan(&id)
		if err != nil {
			return dashboardSourceRecord{}, err
		}
		return a.getDashboardSource(ctx, dashboardID, id)
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE dashboard_source_bindings
		SET section_key = $3,
			pipeline_id = $4,
			output_name = $5,
			entry_key = $6,
			enabled = $7,
			required_for_refresh = $8,
			refresh_order = $9,
			updated_at = NOW()
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboardID, sourceID, input.SectionKey, input.PipelineID, input.OutputName, input.EntryKey, input.Enabled, input.RequiredForRefresh, input.RefreshOrder)
	if err != nil {
		return dashboardSourceRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return dashboardSourceRecord{}, pgx.ErrNoRows
	}
	return a.getDashboardSource(ctx, dashboardID, sourceID)
}

func (a *App) deleteDashboardSource(ctx context.Context, dashboardID, sourceID string) error {
	tag, err := a.db.Exec(ctx, `DELETE FROM dashboard_source_bindings WHERE dashboard_id::text = $1 AND id::text = $2`, dashboardID, sourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (a *App) listDashboardPublications(ctx context.Context, dashboardID string, currentOnly bool) ([]dashboardPublicationRecord, error) {
	query := `
		SELECT id::text, dashboard_id::text, section_key, entry_key, mode, content::text,
		       revision, COALESCE(run_id::text, ''), COALESCE(run_output_id::text, ''),
		       pipeline_id, output_name, COALESCE(refresh_id::text, ''), source_finished_at,
		       published_at, expires_at, status, (expires_at IS NOT NULL AND expires_at <= NOW()) AS stale,
		       created_at, updated_at
		FROM dashboard_publications
		WHERE dashboard_id::text = $1
	`
	if currentOnly {
		query += ` AND status = 'current'`
	}
	query += ` ORDER BY section_key ASC, published_at DESC`
	rows, err := a.db.Query(ctx, query, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var publications []dashboardPublicationRecord
	for rows.Next() {
		publication, err := scanDashboardPublicationRecord(rows)
		if err != nil {
			return nil, err
		}
		publications = append(publications, publication)
	}
	return publications, rows.Err()
}

func (a *App) listDashboardEvents(ctx context.Context, dashboardID string, limit int) ([]dashboardEventRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := a.db.Query(ctx, `
		SELECT id::text, dashboard_id::text, section_key, entry_key, COALESCE(publication_id::text, ''),
		       revision, event_type, content::text, COALESCE(run_id::text, ''), COALESCE(refresh_id::text, ''),
		       created_at
		FROM dashboard_publication_events
		WHERE dashboard_id::text = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, dashboardID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []dashboardEventRecord
	for rows.Next() {
		event, err := scanDashboardEventRecord(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func baseDashboardSelect() string {
	return `
		SELECT d.id::text, d.team_id, d.slug, d.title, d.description, d.visibility,
		       d.refresh_policy::text, COUNT(dp.id)::int, MAX(dp.published_at),
		       COALESCE(d.source, 'database'), d.config_repo_id, d.managed_by_config_repo,
		       COALESCE(d.config_source_path, ''), COALESCE(d.config_source_commit_sha, ''),
		       d.created_by, d.updated_by, d.created_at, d.updated_at
		FROM dashboards d
		LEFT JOIN dashboard_publications dp ON dp.dashboard_id = d.id AND dp.status = 'current'
	`
}

type dashboardScanner interface {
	Scan(dest ...any) error
}

func scanDashboardRecord(scanner dashboardScanner, teamPaths map[int]teamPathRecord) (dashboardRecord, error) {
	var record dashboardRecord
	var refreshPolicyRaw string
	var lastPublishedAt sql.NullTime
	if err := scanner.Scan(
		&record.ID,
		&record.TeamID,
		&record.Slug,
		&record.Title,
		&record.Description,
		&record.Visibility,
		&refreshPolicyRaw,
		&record.CurrentPublicationCount,
		&lastPublishedAt,
		&record.Source,
		&record.ConfigRepoID,
		&record.ManagedByConfigRepo,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
		&record.CreatedBy,
		&record.UpdatedBy,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.RefreshPolicy = scanJSONMap(refreshPolicyRaw)
	if path, ok := teamPaths[record.TeamID]; ok {
		record.TeamPath = path.Path
	}
	record.LastPublishedAt = nullTimePtr(lastPublishedAt)
	record.Visibility = normalizeResourceVisibility(record.Visibility)
	return record, nil
}

func scanDashboardSectionRecord(scanner dashboardScanner) (dashboardSectionRecord, error) {
	var record dashboardSectionRecord
	var layoutRaw string
	if err := scanner.Scan(
		&record.ID,
		&record.DashboardID,
		&record.SectionKey,
		&record.Title,
		&record.Description,
		&layoutRaw,
		&record.DisplayOrder,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.Layout = scanJSONMap(layoutRaw)
	return record, nil
}

func scanDashboardSourceRecord(scanner dashboardScanner) (dashboardSourceRecord, error) {
	var record dashboardSourceRecord
	if err := scanner.Scan(
		&record.ID,
		&record.DashboardID,
		&record.SectionKey,
		&record.PipelineID,
		&record.OutputName,
		&record.EntryKey,
		&record.Enabled,
		&record.RequiredForRefresh,
		&record.RefreshOrder,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	return record, nil
}

func scanDashboardPublicationRecord(scanner dashboardScanner) (dashboardPublicationRecord, error) {
	var record dashboardPublicationRecord
	var contentRaw string
	var sourceFinishedAt, expiresAt sql.NullTime
	if err := scanner.Scan(
		&record.ID,
		&record.DashboardID,
		&record.SectionKey,
		&record.EntryKey,
		&record.Mode,
		&contentRaw,
		&record.Revision,
		&record.RunID,
		&record.RunOutputID,
		&record.PipelineID,
		&record.OutputName,
		&record.RefreshID,
		&sourceFinishedAt,
		&record.PublishedAt,
		&expiresAt,
		&record.Status,
		&record.Stale,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.Content = json.RawMessage(contentRaw)
	record.SourceFinishedAt = nullTimePtr(sourceFinishedAt)
	record.ExpiresAt = nullTimePtr(expiresAt)
	return record, nil
}

func scanDashboardEventRecord(scanner dashboardScanner) (dashboardEventRecord, error) {
	var record dashboardEventRecord
	var contentRaw string
	if err := scanner.Scan(
		&record.ID,
		&record.DashboardID,
		&record.SectionKey,
		&record.EntryKey,
		&record.PublicationID,
		&record.Revision,
		&record.EventType,
		&contentRaw,
		&record.RunID,
		&record.RefreshID,
		&record.CreatedAt,
	); err != nil {
		return record, err
	}
	record.Content = json.RawMessage(contentRaw)
	return record, nil
}

func dashboardNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

func resolveDashboardGrantResource(ctx context.Context, runner queryRunner, rawID string, requireExists bool) (accessGrantResource, error) {
	resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
	if resourceID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if resourceID == "*" {
		return accessGrantResource{Type: grantResourceDashboard, ID: resourceID, Display: resourceID}, nil
	}
	if !requireExists {
		if looksLikeUUID(resourceID) {
			return accessGrantResource{Type: grantResourceDashboard, ID: resourceID, Display: resourceID}, nil
		}
		teamPath, slug, err := splitDashboardRef(resourceID)
		if err != nil {
			return accessGrantResource{}, err
		}
		ref := dashboardResourceID(teamPath, slug)
		return accessGrantResource{Type: grantResourceDashboard, ID: ref, Display: ref}, nil
	}
	teamPaths, err := loadTeamPathRecords(ctx, runner)
	if err != nil {
		return accessGrantResource{}, err
	}
	if looksLikeUUID(resourceID) {
		var teamID int
		var slug string
		err := runner.QueryRow(ctx, `
			SELECT team_id, slug
			FROM dashboards
			WHERE id::text = $1
			LIMIT 1
		`, resourceID).Scan(&teamID, &slug)
		if err != nil {
			if dashboardNotFound(err) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
		teamPath := ""
		if record, ok := teamPaths[teamID]; ok {
			teamPath = record.Path
		}
		ref := dashboardResourceID(teamPath, slug)
		return accessGrantResource{Type: grantResourceDashboard, ID: ref, Display: ref}, nil
	}
	teamPath, slug, err := splitDashboardRef(resourceID)
	if err != nil {
		return accessGrantResource{}, err
	}
	var teamID int
	for _, record := range teamPaths {
		if record.Path == teamPath {
			teamID = record.ID
			break
		}
	}
	if teamID == 0 {
		return accessGrantResource{}, fmt.Errorf("resource not found")
	}
	ref := dashboardResourceID(teamPath, slug)
	if requireExists {
		var exists int
		err := runner.QueryRow(ctx, `
			SELECT 1
			FROM dashboards
			WHERE team_id = $1 AND slug = $2
			LIMIT 1
		`, teamID, slug).Scan(&exists)
		if err != nil {
			if dashboardNotFound(err) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
	}
	return accessGrantResource{Type: grantResourceDashboard, ID: ref, Display: ref}, nil
}
