package nopsai

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Turn progress.
//
// While a turn runs the user sees a line saying what the assistant is doing. It
// used to be guessed in the browser from keywords in the question and advanced
// by a timer, so it announced "Read run metadata and bounded logs" whether or not
// anything read logs. This publishes what the turn is actually doing, as it does
// it, and the label is derived from the tool the model chose rather than from a
// table that would need an entry for every tool ever added.

func (a *App) publishAssistantTurnProgress(ctx context.Context, conversationID uuid.UUID, userID, label string) {
	label = strings.TrimSpace(label)
	if a == nil || a.db == nil || label == "" {
		return
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE assistant_conversations
		SET turn_progress = $1
		WHERE id = $2 AND user_id = $3
	`, label, conversationID, userID); err != nil {
		log.Debug().Err(err).Str("conversation_id", conversationID.String()).Msg("Failed to record assistant turn progress")
	}
}

// assistantToolProgressLabel says what running this tool means, in the user's
// terms. The verb comes from the tool's prefix and the subject from the rest of
// its name, so a tool added tomorrow reads correctly without being registered
// anywhere.
func assistantToolProgressLabel(toolName string) string {
	name := strings.TrimPrefix(strings.TrimSpace(toolName), "nopsai.")
	if name == "" {
		return "Working"
	}
	verbs := []struct {
		prefix string
		verb   string
	}{
		{"get_", "Reading"},
		{"list_", "Listing"},
		{"search_", "Searching"},
		{"find_", "Looking for"},
		{"analyze_", "Analysing"},
		{"explain_", "Explaining"},
		{"validate_", "Validating"},
		{"propose_", "Preparing a proposal for"},
		{"write_", "Writing"},
		{"delete_", "Deleting"},
		{"encrypt_", "Encrypting"},
		{"run_", "Running"},
	}
	for _, candidate := range verbs {
		if strings.HasPrefix(name, candidate.prefix) {
			subject := strings.ReplaceAll(strings.TrimPrefix(name, candidate.prefix), "_", " ")
			return candidate.verb + " " + subject
		}
	}
	return "Running " + strings.ReplaceAll(name, "_", " ")
}
