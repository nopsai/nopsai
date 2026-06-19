package nopsai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (a *App) loadAssistantMemory(ctx context.Context, conversationID uuid.UUID) (assistantConversationMemory, error) {
	var (
		memory      assistantConversationMemory
		entitiesRaw []byte
		tasksRaw    []byte
		fixesRaw    []byte
	)
	err := a.db.QueryRow(ctx, `
		SELECT conversation_id, summary, entities, open_tasks, previous_proposed_fixes,
		       selected_run, selected_pipeline, selected_scope, selected_docs_version, updated_at
		FROM assistant_conversation_memory
		WHERE conversation_id = $1
	`, conversationID).Scan(
		&memory.ConversationID,
		&memory.Summary,
		&entitiesRaw,
		&tasksRaw,
		&fixesRaw,
		&memory.SelectedRun,
		&memory.SelectedPipeline,
		&memory.SelectedScope,
		&memory.SelectedDocsVersion,
		&memory.UpdatedAt,
	)
	if err != nil {
		return assistantConversationMemory{}, err
	}
	if len(entitiesRaw) > 0 {
		if err := json.Unmarshal(entitiesRaw, &memory.Entities); err != nil {
			return assistantConversationMemory{}, fmt.Errorf("parse assistant memory entities: %w", err)
		}
	}
	if len(tasksRaw) > 0 {
		if err := json.Unmarshal(tasksRaw, &memory.OpenTasks); err != nil {
			return assistantConversationMemory{}, fmt.Errorf("parse assistant memory open tasks: %w", err)
		}
	}
	if len(fixesRaw) > 0 {
		if err := json.Unmarshal(fixesRaw, &memory.PreviousProposedFixes); err != nil {
			return assistantConversationMemory{}, fmt.Errorf("parse assistant memory proposed fixes: %w", err)
		}
	}
	return normalizeAssistantMemory(memory), nil
}

func (a *App) upsertAssistantMemory(ctx context.Context, conversationID uuid.UUID, memory assistantConversationMemory) (assistantConversationMemory, error) {
	memory = normalizeAssistantMemory(memory)
	entitiesRaw, err := json.Marshal(memory.Entities)
	if err != nil {
		return assistantConversationMemory{}, fmt.Errorf("encode assistant memory entities: %w", err)
	}
	tasksRaw, err := json.Marshal(memory.OpenTasks)
	if err != nil {
		return assistantConversationMemory{}, fmt.Errorf("encode assistant memory open tasks: %w", err)
	}
	fixesRaw, err := json.Marshal(memory.PreviousProposedFixes)
	if err != nil {
		return assistantConversationMemory{}, fmt.Errorf("encode assistant memory proposed fixes: %w", err)
	}

	err = a.db.QueryRow(ctx, `
		INSERT INTO assistant_conversation_memory (
			conversation_id, summary, entities, open_tasks, previous_proposed_fixes,
			selected_run, selected_pipeline, selected_scope, selected_docs_version
		)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6, $7, $8, $9)
		ON CONFLICT (conversation_id) DO UPDATE SET
			summary = EXCLUDED.summary,
			entities = EXCLUDED.entities,
			open_tasks = EXCLUDED.open_tasks,
			previous_proposed_fixes = EXCLUDED.previous_proposed_fixes,
			selected_run = EXCLUDED.selected_run,
			selected_pipeline = EXCLUDED.selected_pipeline,
			selected_scope = EXCLUDED.selected_scope,
			selected_docs_version = EXCLUDED.selected_docs_version,
			updated_at = NOW()
		RETURNING conversation_id, updated_at
	`, conversationID, memory.Summary, string(entitiesRaw), string(tasksRaw), string(fixesRaw),
		memory.SelectedRun, memory.SelectedPipeline, memory.SelectedScope, memory.SelectedDocsVersion,
	).Scan(&memory.ConversationID, &memory.UpdatedAt)
	if err != nil {
		return assistantConversationMemory{}, fmt.Errorf("upsert assistant memory: %w", err)
	}
	return memory, nil
}
