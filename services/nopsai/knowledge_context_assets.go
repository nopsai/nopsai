package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type knowledgeContextAssetSummary struct {
	ID              string         `json:"id"`
	Provider        string         `json:"provider"`
	ExternalPageID  string         `json:"external_page_id,omitempty"`
	SourceBlockID   string         `json:"source_block_id"`
	SourceBlockType string         `json:"source_block_type"`
	Kind            string         `json:"kind"`
	Title           string         `json:"title,omitempty"`
	URL             string         `json:"url,omitempty"`
	MediaType       string         `json:"media_type,omitempty"`
	ContentHash     string         `json:"content_hash,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (a *App) replaceKnowledgeContextAssets(ctx context.Context, tx pgx.Tx, knowledgeID, provider string, page ExternalPage) error {
	if strings.TrimSpace(knowledgeID) == "" {
		return fmt.Errorf("knowledge context id is required")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_context_assets WHERE knowledge_context_id = $1::uuid`, knowledgeID); err != nil {
		return fmt.Errorf("delete previous knowledge context assets: %w", err)
	}
	for _, asset := range dedupeExternalPageAssets(page.Assets) {
		asset = normalizeExternalPageAsset(asset)
		metadata, err := json.Marshal(asset.Metadata)
		if err != nil {
			return fmt.Errorf("marshal knowledge context asset metadata: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO knowledge_context_assets (
				knowledge_context_id, provider, external_page_id, source_block_id, source_block_type,
				asset_kind, title, url, media_type, content_hash, metadata, updated_at
			) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, NOW())
			ON CONFLICT (knowledge_context_id, source_block_id, asset_kind, url)
			DO UPDATE SET
				provider = EXCLUDED.provider,
				external_page_id = EXCLUDED.external_page_id,
				source_block_type = EXCLUDED.source_block_type,
				title = EXCLUDED.title,
				media_type = EXCLUDED.media_type,
				content_hash = EXCLUDED.content_hash,
				metadata = EXCLUDED.metadata,
				updated_at = NOW()
		`, knowledgeID, provider, page.ID, asset.SourceBlockID, asset.SourceBlockType,
			asset.Kind, asset.Title, asset.URL, asset.MediaType, asset.ContentHash, string(metadata))
		if err != nil {
			return fmt.Errorf("insert knowledge context asset %s: %w", asset.SourceBlockID, err)
		}
	}
	return nil
}

func (a *App) loadKnowledgeContextAssets(ctx context.Context, knowledgeID string) ([]knowledgeContextAssetSummary, error) {
	if a == nil || a.db == nil || strings.TrimSpace(knowledgeID) == "" {
		return nil, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT id::text, provider, external_page_id, source_block_id, source_block_type,
		       asset_kind, title, url, media_type, content_hash, metadata, updated_at
		FROM knowledge_context_assets
		WHERE knowledge_context_id = $1::uuid
		ORDER BY asset_kind ASC, source_block_type ASC, title ASC, source_block_id ASC
	`, strings.TrimSpace(knowledgeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assets []knowledgeContextAssetSummary
	for rows.Next() {
		var asset knowledgeContextAssetSummary
		var metadataRaw []byte
		if err := rows.Scan(&asset.ID, &asset.Provider, &asset.ExternalPageID, &asset.SourceBlockID,
			&asset.SourceBlockType, &asset.Kind, &asset.Title, &asset.URL, &asset.MediaType,
			&asset.ContentHash, &metadataRaw, &asset.UpdatedAt); err != nil {
			return nil, err
		}
		if len(metadataRaw) > 0 {
			_ = json.Unmarshal(metadataRaw, &asset.Metadata)
		}
		if asset.Metadata == nil {
			asset.Metadata = map[string]any{}
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}
