package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
)

const teamKnowledgeDefaultSettingPrefix = "knowledge_default:"

type teamDefaultsResponse struct {
	TeamID           int               `json:"team_id"`
	TeamPath         string            `json:"team_path"`
	LLMProfile       string            `json:"model,omitempty"`
	AgentProfile     string            `json:"agent_role,omitempty"`
	KnowledgeContext map[string]string `json:"knowledge_context"`
}

type updateTeamDefaultsRequest struct {
	LLMProfile       *string           `json:"model,omitempty"`
	AgentProfile     *string           `json:"agent_role,omitempty"`
	KnowledgeContext map[string]string `json:"knowledge_context,omitempty"`
}

func (a *App) handleTeamDefaults(w http.ResponseWriter, r *http.Request) {
	write := r.Method != http.MethodGet
	record, ok := a.resolveAuthorizedTeamProfile(w, r, write)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		response, err := a.buildTeamDefaultsResponse(r.Context(), record)
		if err != nil {
			http.Error(w, "failed to load team defaults", http.StatusInternalServerError)
			return
		}
		_ = httpapi.WriteJSON(w, http.StatusOK, response)
	case http.MethodPut:
		var payload updateTeamDefaultsRequest
		if err := httpapi.DecodeJSON(r, &payload); err != nil {
			http.Error(w, "invalid team defaults payload", http.StatusBadRequest)
			return
		}
		if err := a.updateTeamDefaults(r.Context(), record, payload); err != nil {
			http.Error(w, err.Error(), httpStatusForTeamDefaultError(err))
			return
		}
		response, err := a.buildTeamDefaultsResponse(r.Context(), record)
		if err != nil {
			http.Error(w, "failed to load team defaults", http.StatusInternalServerError)
			return
		}
		_ = httpapi.WriteJSON(w, http.StatusOK, response)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) buildTeamDefaultsResponse(ctx context.Context, record teamPathRecord) (teamDefaultsResponse, error) {
	llmProfiles, err := a.buildTeamLLMProfilesResponse(ctx, record)
	if err != nil {
		return teamDefaultsResponse{}, err
	}
	agentProfiles, err := a.buildTeamAgentProfilesResponse(ctx, record)
	if err != nil {
		return teamDefaultsResponse{}, err
	}
	knowledgeDefaults, err := a.loadTeamKnowledgeDefaults(ctx, record)
	if err != nil {
		return teamDefaultsResponse{}, err
	}
	return teamDefaultsResponse{
		TeamID:           record.ID,
		TeamPath:         record.Path,
		LLMProfile:       llmProfiles.DefaultProfile,
		AgentProfile:     agentProfiles.DefaultProfile,
		KnowledgeContext: knowledgeDefaults,
	}, nil
}

func (a *App) updateTeamDefaults(ctx context.Context, record teamPathRecord, payload updateTeamDefaultsRequest) error {
	if payload.LLMProfile != nil {
		defaultProfile := config.NormalizeLLMProfileName(*payload.LLMProfile)
		if defaultProfile != "" {
			_, profiles, err := a.loadTeamLLMProfilesFromDB(ctx, record.ID)
			if err != nil {
				return fmt.Errorf("failed to load team LLM profiles")
			}
			canonical, ok, err := a.canonicalTeamLLMDefaultProfile(ctx, record, defaultProfile, profiles)
			if err != nil {
				return fmt.Errorf("failed to validate team LLM default profile")
			}
			if !ok {
				return errBadTeamDefault("default LLM profile must be a team profile or a scoped profile in this team")
			}
			defaultProfile = canonical
		}
		if err := a.persistTeamProfileSetting(ctx, record.ID, teamLLMDefaultProfileSetting, defaultProfile); err != nil {
			return fmt.Errorf("failed to save team LLM default profile")
		}
	}
	if payload.AgentProfile != nil {
		defaultProfile := normalizeAgentProfileDefault(*payload.AgentProfile)
		profiles, err := a.loadTeamAgentProfilesFromDB(ctx, record.ID)
		if err != nil {
			return fmt.Errorf("failed to load team agent profiles")
		}
		if defaultProfile != "" {
			canonical, ok, err := a.canonicalTeamAgentDefaultProfile(ctx, record, defaultProfile, profiles)
			if err != nil {
				return fmt.Errorf("failed to validate team agent default profile")
			}
			if !ok {
				return errBadTeamDefault("default agent profile must be an enabled team profile or scoped profile in this team")
			}
			defaultProfile = canonical
		}
		if err := a.persistTeamProfileSetting(ctx, record.ID, teamAgentDefaultProfileSetting, defaultProfile); err != nil {
			return fmt.Errorf("failed to save team agent default profile")
		}
	}
	for rawKind, rawRef := range payload.KnowledgeContext {
		kind, err := normalizeKnowledgeContextKind(rawKind)
		if err != nil {
			return errBadTeamDefault(fmt.Sprintf("invalid knowledge default kind %q: %v", rawKind, err))
		}
		canonical, ok, err := a.canonicalTeamKnowledgeDefault(ctx, record, kind, rawRef)
		if err != nil {
			return fmt.Errorf("failed to validate %s knowledge default", kind)
		}
		if !ok {
			return errBadTeamDefault(fmt.Sprintf("%s knowledge default must be a %s document owned by %s", kind, kind, record.Path))
		}
		if err := a.persistTeamProfileSetting(ctx, record.ID, teamKnowledgeDefaultSettingKey(kind), canonical); err != nil {
			return fmt.Errorf("failed to save %s knowledge default", kind)
		}
	}
	return nil
}

func (a *App) loadTeamKnowledgeDefaults(ctx context.Context, record teamPathRecord) (map[string]string, error) {
	defaults := map[string]string{}
	for _, kind := range sortedKnowledgeContextKinds() {
		value, err := a.loadTeamProfileSetting(ctx, record.ID, teamKnowledgeDefaultSettingKey(kind))
		if err != nil {
			return nil, err
		}
		canonical, ok, err := a.canonicalTeamKnowledgeDefault(ctx, record, kind, value)
		if err != nil {
			return nil, err
		}
		if ok && canonical != "" {
			defaults[kind] = canonical
		}
	}
	return defaults, nil
}

func (a *App) canonicalTeamKnowledgeDefault(ctx context.Context, record teamPathRecord, kind, rawRef string) (string, bool, error) {
	return canonicalTeamKnowledgeDefaultWithRunner(ctx, a.db, record, kind, rawRef)
}

func canonicalTeamKnowledgeDefaultWithRunner(ctx context.Context, runner queryRunner, record teamPathRecord, kind, rawRef string) (string, bool, error) {
	kind, err := normalizeKnowledgeContextKind(kind)
	if err != nil {
		return "", false, err
	}
	rawRef = strings.TrimSpace(rawRef)
	if rawRef == "" {
		return "", true, nil
	}
	_, teamPath, name, err := teamKnowledgeDefaultRefParts(kind, record.Path, rawRef)
	if err != nil {
		return "", false, nil
	}
	if !strings.EqualFold(teamPath, record.Path) {
		return "", false, nil
	}
	teamPath = record.Path
	if runner == nil {
		return "", false, fmt.Errorf("knowledge default lookup requires a database")
	}
	var exists bool
	err = runner.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM knowledge_contexts
			WHERE kind = $1 AND team_path = $2 AND name = $3
		)
	`, kind, teamPath, name).Scan(&exists)
	if err != nil {
		return "", false, err
	}
	return buildKnowledgeDocumentIdentifier(teamPath, name), exists, nil
}

func teamKnowledgeDefaultRefParts(kind, teamPath, rawRef string) (string, string, string, error) {
	kind, err := normalizeKnowledgeContextKind(kind)
	if err != nil {
		return "", "", "", err
	}
	teamPath, err = normalizeKnowledgeContextTeam(teamPath)
	if err != nil {
		return "", "", "", err
	}
	value := strings.Trim(strings.TrimSpace(strings.ReplaceAll(rawRef, "\\", "/")), "/")
	if value == "" {
		return kind, "", "", fmt.Errorf("knowledge default ref is required")
	}
	parts := strings.Split(value, "/")
	if len(parts) >= 2 {
		if prefixedKind, prefixedErr := normalizeKnowledgeContextKind(parts[0]); prefixedErr == nil {
			if prefixedKind != kind {
				return "", "", "", fmt.Errorf("knowledge default kind %q does not match %q", prefixedKind, kind)
			}
			name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
			if err != nil {
				return "", "", "", err
			}
			refTeam, err := normalizeKnowledgeContextTeam(strings.Join(parts[1:len(parts)-1], "/"))
			if err != nil {
				return "", "", "", err
			}
			if refTeam == "" {
				refTeam = teamPath
			}
			return kind, refTeam, name, nil
		}
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	if len(parts) == 1 {
		return kind, teamPath, name, nil
	}
	refTeam, err := normalizeKnowledgeContextTeam(strings.Join(parts[:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	if refTeam == "" {
		return "", "", "", fmt.Errorf("team knowledge default must reference a team-owned document")
	}
	return kind, refTeam, name, nil
}

func (a *App) effectiveKnowledgeDefaultRefsForTeam(ctx context.Context, teamID *int) ([]models.KnowledgeContextRef, error) {
	if teamID == nil || a == nil || a.db == nil {
		return nil, nil
	}
	records, err := a.teamPathRecords(ctx)
	if err != nil {
		return nil, err
	}
	record, ok := records[*teamID]
	if !ok {
		return nil, nil
	}
	defaults, err := a.loadTeamKnowledgeDefaults(ctx, record)
	if err != nil {
		return nil, err
	}
	refs := make([]models.KnowledgeContextRef, 0, len(defaults))
	for _, kind := range sortedKnowledgeContextKinds() {
		ref := defaults[kind]
		if ref == "" {
			continue
		}
		refs = append(refs, models.KnowledgeContextRef{Kind: kind, Ref: ref, Required: true})
	}
	return refs, nil
}

func teamKnowledgeDefaultSettingKey(kind string) string {
	return teamKnowledgeDefaultSettingPrefix + strings.TrimSpace(kind)
}

func sortedKnowledgeContextKinds() []string {
	kinds := make([]string, 0, len(supportedKnowledgeContextKinds))
	for kind := range supportedKnowledgeContextKinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

type errBadTeamDefault string

func (e errBadTeamDefault) Error() string {
	return string(e)
}

func httpStatusForTeamDefaultError(err error) int {
	if _, ok := err.(errBadTeamDefault); ok {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
