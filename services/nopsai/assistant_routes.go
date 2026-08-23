package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
)

func (a *App) registerAssistantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/assistant/config", a.handleGetAssistantConfig)
	mux.HandleFunc("GET /v1/assistant/models", a.handleListAssistantLLMProfiles)
	mux.HandleFunc("POST /v1/assistant/conversations", a.handleCreateAssistantConversation)
	mux.HandleFunc("GET /v1/assistant/conversations", a.handleListAssistantConversations)
	mux.HandleFunc("GET /v1/assistant/conversations/{id}", a.handleGetAssistantConversation)
	mux.HandleFunc("DELETE /v1/assistant/conversations/{id}", a.handleDeleteAssistantConversation)
	mux.HandleFunc("POST /v1/assistant/conversations/{id}/messages", a.handleCreateAssistantMessage)
	mux.HandleFunc("POST /v1/assistant/conversations/{id}/summarize-memory", a.handleSummarizeAssistantMemory)
}

func (a *App) handleGetAssistantConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAssistantUserID(w, r); !ok {
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, buildAssistantConfigResponse(a.assistantConfig()))
}

func (a *App) requireAssistantEnabled(w http.ResponseWriter) bool {
	if a == nil || a.db == nil {
		http.Error(w, "assistant is unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !a.assistantConfig().Enabled {
		http.Error(w, "assistant is disabled", http.StatusNotFound)
		return false
	}
	return true
}

func (a *App) requireAssistantUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return "", false
	}
	userID := assistantUserID(subject)
	if userID == "" {
		http.Error(w, "missing assistant user", http.StatusUnauthorized)
		return "", false
	}
	return userID, true
}

func assistantUserID(subject model.Subject) string {
	subjectType := strings.TrimSpace(subject.Type)
	if subjectType == "" {
		subjectType = model.SubjectTypeUser
	}
	id := strings.TrimSpace(subject.ID)
	if id == "" {
		id = strings.TrimSpace(subject.Sub)
	}
	if id == "" {
		id = strings.TrimSpace(subject.Email)
	}
	if id == "" {
		return ""
	}
	return subjectType + ":" + id
}

func (a *App) handleCreateAssistantConversation(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	var req assistantCreateConversationRequest
	if err := httpapi.DecodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid assistant conversation payload", http.StatusBadRequest)
		return
	}
	cfg := a.assistantConfig()
	req = normalizeAssistantConversationRequest(req, cfg)
	if req.SelectedLLMProfile == "" {
		req.SelectedLLMProfile = a.assistantDefaultLLMProfile(r.Context())
	}
	if err := a.validateAssistantLLMProfile(r.Context(), req.SelectedLLMProfile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var conversation assistantConversation
	err := a.db.QueryRow(r.Context(), `
		INSERT INTO assistant_conversations (user_id, title, selected_llm_profile, docs_version, scope)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, title, selected_llm_profile, docs_version, scope, created_at, updated_at
	`, userID, req.Title, req.SelectedLLMProfile, req.DocsVersion, req.Scope).Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.Title,
		&conversation.SelectedLLMProfile,
		&conversation.DocsVersion,
		&conversation.Scope,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	if err != nil {
		http.Error(w, "failed to create assistant conversation", http.StatusInternalServerError)
		return
	}
	if cfg.Memory.Enabled {
		memory, err := a.upsertAssistantMemory(r.Context(), conversation.ID, assistantConversationMemory{
			SelectedRun:         assistantPageContextRunID(req.PageContext),
			SelectedPipeline:    assistantPageContextPipelineID(req.PageContext),
			SelectedScope:       req.Scope,
			SelectedDocsVersion: req.DocsVersion,
		})
		if err != nil {
			http.Error(w, "failed to initialize assistant memory", http.StatusInternalServerError)
			return
		}
		conversation.Memory = memory
	}
	conversation.Messages = []assistantMessage{}
	_ = httpapi.WriteJSON(w, http.StatusCreated, conversation)
}

func (a *App) handleListAssistantConversations(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, user_id, title, selected_llm_profile, docs_version, scope, created_at, updated_at, running_turn_started_at
		FROM assistant_conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC, created_at DESC
	`, userID)
	if err != nil {
		http.Error(w, "failed to load assistant conversations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	conversations := []assistantConversation{}
	for rows.Next() {
		var conversation assistantConversation
		if err := rows.Scan(
			&conversation.ID,
			&conversation.UserID,
			&conversation.Title,
			&conversation.SelectedLLMProfile,
			&conversation.DocsVersion,
			&conversation.Scope,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
			&conversation.RunningTurnStartedAt,
		); err != nil {
			http.Error(w, "failed to scan assistant conversation", http.StatusInternalServerError)
			return
		}
		conversation.TurnRunning = assistantTurnIsRunning(conversation.RunningTurnStartedAt)
		conversation.Messages = []assistantMessage{}
		conversations = append(conversations, conversation)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load assistant conversations", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, assistantConversationsResponse{Conversations: conversations})
}

func (a *App) handleGetAssistantConversation(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "assistant conversation id is invalid", http.StatusBadRequest)
		return
	}
	conversation, err := a.loadAssistantConversation(r.Context(), userID, conversationID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "assistant conversation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load assistant conversation", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, conversation)
}

func (a *App) handleDeleteAssistantConversation(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "assistant conversation id is invalid", http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `
		DELETE FROM assistant_conversations
		WHERE id = $1 AND user_id = $2
	`, conversationID, userID)
	if err != nil {
		http.Error(w, "failed to delete assistant conversation", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "assistant conversation not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleCreateAssistantMessage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "assistant conversation id is invalid", http.StatusBadRequest)
		return
	}
	var req assistantCreateMessageRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid assistant message payload", http.StatusBadRequest)
		return
	}
	req = normalizeAssistantMessageRequest(req)
	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}
	if err := a.validateAssistantLLMProfile(r.Context(), req.SelectedLLMProfile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conversation, err := a.loadAssistantConversation(r.Context(), userID, conversationID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "assistant conversation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load assistant conversation", http.StatusInternalServerError)
		return
	}
	selectedProfile := firstNonEmpty(req.SelectedLLMProfile, conversation.SelectedLLMProfile)
	if selectedProfile == "" {
		selectedProfile = a.assistantDefaultLLMProfile(r.Context())
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to append assistant message", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	userMessage, err := insertAssistantMessageTx(r.Context(), tx, conversationID, assistantRoleUser, req.Content, nil, assistantUsageForUserMessage(req.Content))
	if err != nil {
		http.Error(w, "failed to append assistant message", http.StatusInternalServerError)
		return
	}
	nextTitle := conversation.Title
	if nextTitle == "" {
		nextTitle = assistantTitleFromMessage(req.Content)
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE assistant_conversations
		SET title = $1,
			selected_llm_profile = CASE WHEN $2 <> '' THEN $2 ELSE selected_llm_profile END,
			updated_at = NOW()
		WHERE id = $3 AND user_id = $4
	`, nextTitle, selectedProfile, conversationID, userID)
	if err != nil {
		http.Error(w, "failed to update assistant conversation", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to append assistant message", http.StatusInternalServerError)
		return
	}

	conversation.Title = nextTitle
	if selectedProfile != "" {
		conversation.SelectedLLMProfile = selectedProfile
	}
	messages, err := a.loadAssistantMessages(r.Context(), conversationID)
	if err != nil {
		http.Error(w, "failed to load assistant conversation messages", http.StatusInternalServerError)
		return
	}
	conversation.Messages = messages
	conversation.Usage = assistantConversationUsageFromMessages(messages)
	// The turn outlives the request. A refresh or a navigation cancels the client
	// side of the connection, and losing a half-finished turn — already charged to
	// the user — because they changed page is the wrong trade.
	turnCtx := context.WithoutCancel(r.Context())
	turnStarted := time.Now()
	a.markAssistantTurnRunning(turnCtx, conversationID, userID, true)
	defer a.markAssistantTurnRunning(turnCtx, conversationID, userID, false)

	orchestration := a.runAssistantConversationTurnWithPageContext(turnCtx, subject, userID, conversation, req.Content, selectedProfile, req.PageContext)
	replyUsage := assistantUsageForAssistantReply(orchestration.Reply, orchestration.ToolCalls, time.Since(turnStarted), a.assistantLLMPricer())
	reply, err := insertAssistantMessageTx(turnCtx, a.db, conversationID, assistantRoleAssistant, orchestration.Reply, orchestration.ToolCalls, replyUsage)
	if err != nil {
		http.Error(w, "failed to append assistant reply", http.StatusInternalServerError)
		return
	}
	if a.assistantConfig().Memory.Enabled {
		if _, err := a.upsertAssistantMemory(turnCtx, conversationID, orchestration.Memory); err != nil {
			http.Error(w, "failed to update assistant memory", http.StatusInternalServerError)
			return
		}
	}

	conversation, err = a.loadAssistantConversation(turnCtx, userID, conversationID, true)
	if err != nil {
		http.Error(w, "failed to load assistant conversation", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, assistantMessageResponse{
		Conversation: conversation,
		UserMessage:  userMessage,
		Reply:        reply,
	})
}

// markAssistantTurnRunning records that a turn is in flight, or that it is not.
// A failure to record it is logged rather than surfaced: the flag is how the UI
// shows progress, and refusing to answer because a progress marker did not save
// would be worse than showing no spinner.
func (a *App) markAssistantTurnRunning(ctx context.Context, conversationID uuid.UUID, userID string, running bool) {
	var startedAt any
	if running {
		startedAt = time.Now().UTC()
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE assistant_conversations
		SET running_turn_started_at = $1
		WHERE id = $2 AND user_id = $3
	`, startedAt, conversationID, userID); err != nil {
		log.Warn().Err(err).Str("conversation_id", conversationID.String()).Bool("running", running).
			Msg("Failed to record assistant turn state")
	}
}

func (a *App) handleSummarizeAssistantMemory(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "assistant conversation id is invalid", http.StatusBadRequest)
		return
	}
	if _, err := a.loadAssistantConversation(r.Context(), userID, conversationID, false); errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "assistant conversation not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to load assistant conversation", http.StatusInternalServerError)
		return
	}
	var req assistantSummarizeMemoryRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid assistant memory payload", http.StatusBadRequest)
		return
	}
	memory, err := a.upsertAssistantMemory(r.Context(), conversationID, assistantConversationMemory{
		Summary:               req.Summary,
		Entities:              req.Entities,
		OpenTasks:             req.OpenTasks,
		PreviousProposedFixes: req.PreviousProposedFixes,
		SelectedRun:           req.SelectedRun,
		SelectedPipeline:      req.SelectedPipeline,
		SelectedScope:         req.SelectedScope,
		SelectedDocsVersion:   req.SelectedDocsVersion,
	})
	if err != nil {
		http.Error(w, "failed to summarize assistant memory", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, memory)
}

func (a *App) handleListAssistantLLMProfiles(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssistantEnabled(w) {
		return
	}
	if _, ok := a.requireAssistantUserID(w, r); !ok {
		return
	}
	scope := strings.Trim(strings.TrimSpace(r.URL.Query().Get("scope")), "/")
	defaultProfile, profiles := a.assistantLLMProfiles(r.Context())
	_ = httpapi.WriteJSON(w, http.StatusOK, buildAssistantLLMProfilesResponse(defaultProfile, profiles, scope))
}

func (a *App) validateAssistantLLMProfile(ctx context.Context, profileName string) error {
	profileName = config.NormalizeLLMProfileName(profileName)
	if profileName == "" {
		return nil
	}
	_, profiles := a.assistantLLMProfiles(ctx)
	if len(profiles) == 0 {
		return fmt.Errorf("selected LLM profile %q is not available", profileName)
	}
	if _, ok := profiles[profileName]; !ok {
		return fmt.Errorf("selected LLM profile %q was not found", profileName)
	}
	return nil
}

func (a *App) loadAssistantConversation(ctx context.Context, userID string, conversationID uuid.UUID, includeMessages bool) (assistantConversation, error) {
	var conversation assistantConversation
	err := a.db.QueryRow(ctx, `
		SELECT id, user_id, title, selected_llm_profile, docs_version, scope, created_at, updated_at, running_turn_started_at
		FROM assistant_conversations
		WHERE id = $1 AND user_id = $2
	`, conversationID, userID).Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.Title,
		&conversation.SelectedLLMProfile,
		&conversation.DocsVersion,
		&conversation.Scope,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
		&conversation.RunningTurnStartedAt,
	)
	if err != nil {
		return assistantConversation{}, err
	}
	conversation.TurnRunning = assistantTurnIsRunning(conversation.RunningTurnStartedAt)
	if includeMessages {
		messages, err := a.loadAssistantMessages(ctx, conversationID)
		if err != nil {
			return assistantConversation{}, err
		}
		conversation.Messages = messages
		conversation.Usage = assistantConversationUsageFromMessages(messages)
	}
	memory, err := a.loadAssistantMemory(ctx, conversationID)
	if err == nil {
		conversation.Memory = memory
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return assistantConversation{}, err
	}
	return conversation, nil
}

func (a *App) loadAssistantMessages(ctx context.Context, conversationID uuid.UUID) ([]assistantMessage, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, conversation_id, role, content, tool_calls,
		       content_tokens, prompt_tokens, completion_tokens, total_tokens,
		       usage_estimated, duration_ms, llm_calls, cost_usd, created_at
		FROM assistant_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := []assistantMessage{}
	for rows.Next() {
		var (
			message      assistantMessage
			toolCallsRaw []byte
		)
		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&toolCallsRaw,
			&message.Usage.ContentTokens,
			&message.Usage.PromptTokens,
			&message.Usage.CompletionTokens,
			&message.Usage.TotalTokens,
			&message.Usage.Estimated,
			&message.Usage.DurationMS,
			&message.Usage.LLMCalls,
			&message.Usage.CostUSD,
			&message.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(toolCallsRaw) > 0 {
			if err := json.Unmarshal(toolCallsRaw, &message.ToolCalls); err != nil {
				return nil, err
			}
		}
		if message.ToolCalls == nil {
			message.ToolCalls = []assistantToolActivity{}
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

type assistantMessageTx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func insertAssistantMessageTx(ctx context.Context, tx assistantMessageTx, conversationID uuid.UUID, role, content string, toolCalls []assistantToolActivity, usage assistantMessageUsage) (assistantMessage, error) {
	if toolCalls == nil {
		toolCalls = []assistantToolActivity{}
	}
	usage = normalizeAssistantMessageUsage(content, usage)
	raw, err := json.Marshal(toolCalls)
	if err != nil {
		return assistantMessage{}, err
	}
	var message assistantMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO assistant_messages (
			conversation_id, role, content, tool_calls,
			content_tokens, prompt_tokens, completion_tokens, total_tokens,
			usage_estimated, duration_ms, llm_calls, cost_usd
		)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, conversation_id, role, content, tool_calls,
		          content_tokens, prompt_tokens, completion_tokens, total_tokens,
		          usage_estimated, duration_ms, llm_calls, cost_usd, created_at
	`, conversationID, role, content, string(raw),
		usage.ContentTokens, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		usage.Estimated, usage.DurationMS, usage.LLMCalls, usage.CostUSD).Scan(
		&message.ID,
		&message.ConversationID,
		&message.Role,
		&message.Content,
		&raw,
		&message.Usage.ContentTokens,
		&message.Usage.PromptTokens,
		&message.Usage.CompletionTokens,
		&message.Usage.TotalTokens,
		&message.Usage.Estimated,
		&message.Usage.DurationMS,
		&message.Usage.LLMCalls,
		&message.Usage.CostUSD,
		&message.CreatedAt,
	)
	if err != nil {
		return assistantMessage{}, err
	}
	if err := json.Unmarshal(raw, &message.ToolCalls); err != nil {
		return assistantMessage{}, err
	}
	if message.ToolCalls == nil {
		message.ToolCalls = []assistantToolActivity{}
	}
	return message, nil
}

func buildAssistantFoundationReply(selectedProfile string) string {
	if selectedProfile == "" {
		return "I saved this message and am ready to use the hosted Nopsai MCP tools once a profile is selected. The first foundation slice keeps tool actions read-only and permission-bound."
	}
	return fmt.Sprintf("I saved this message for the conversation using LLM profile %q. The assistant foundation is ready to use permission-bound hosted MCP tools and conversation memory.", selectedProfile)
}

func assistantTitleFromMessage(content string) string {
	title := strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if len(title) <= 80 {
		return title
	}
	return strings.TrimSpace(title[:77]) + "..."
}
