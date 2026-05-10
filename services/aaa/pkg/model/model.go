package model

import (
	"net/url"
	"sort"
	"strings"
)

const (
	SubjectTypeUser            = "user"
	SubjectTypeAuthGroup       = "auth_group"
	SubjectTypeRole            = "role"
	SubjectTypeInternalService = "internal_service"

	RoleNameAdmin = "nopsai-admin"
)

type Subject struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Sub   string `json:"sub,omitempty"`
	Email string `json:"email,omitempty"`
}

type SubjectRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type ResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Decision struct {
	Allowed       bool           `json:"allowed"`
	Reason        string         `json:"reason"`
	MatchedPolicy map[string]any `json:"matched_policy,omitempty"`
}

type CheckRequest struct {
	Subject  Subject        `json:"subject"`
	Action   string         `json:"action"`
	Resource ResourceRef    `json:"resource"`
	Context  map[string]any `json:"context,omitempty"`
}

type BatchCheckItem struct {
	Action   string      `json:"action"`
	Resource ResourceRef `json:"resource"`
}

type BatchCheckRequest struct {
	Subject Subject          `json:"subject"`
	Checks  []BatchCheckItem `json:"checks"`
	Context map[string]any   `json:"context,omitempty"`
}

type BatchCheckResponse struct {
	Decisions []Decision `json:"decisions"`
}

type FilterRequest struct {
	Subject   Subject        `json:"subject"`
	Action    string         `json:"action"`
	Resources []ResourceRef  `json:"resources"`
	Context   map[string]any `json:"context,omitempty"`
}

type FilterResponse struct {
	Resources []ResourceRef `json:"resources"`
}

type AuditRecordRequest struct {
	RequestID     string         `json:"request_id,omitempty"`
	Subject       Subject        `json:"subject"`
	Action        string         `json:"action"`
	Resource      ResourceRef    `json:"resource"`
	Allowed       bool           `json:"allowed"`
	Reason        string         `json:"reason"`
	MatchedPolicy map[string]any `json:"matched_policy,omitempty"`
	Sensitive     bool           `json:"sensitive"`
	Context       map[string]any `json:"context,omitempty"`
}

type IntrospectRequest struct {
	Subject Subject `json:"subject"`
}

type AuthGroupInfo struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles,omitempty"`
}

type IntrospectResponse struct {
	ID         string          `json:"id"`
	Sub        string          `json:"sub"`
	Email      string          `json:"email,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Status     string          `json:"status"`
	Roles      []string        `json:"roles,omitempty"`
	AuthGroups []AuthGroupInfo `json:"auth_groups,omitempty"`
}

type ResolvedSubject struct {
	Subject     Subject
	Provider    string
	Status      string
	DirectRoles []string
	AuthGroups  []AuthGroupInfo
}

func (s ResolvedSubject) EffectiveRoles() []string {
	seen := make(map[string]struct{})
	var roles []string
	for _, role := range s.DirectRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	for _, group := range s.AuthGroups {
		for _, role := range group.Roles {
			role = strings.TrimSpace(role)
			if role == "" {
				continue
			}
			if _, ok := seen[role]; ok {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	return roles
}

type MatchedPolicy struct {
	Source       string `json:"source"`
	RoleName     string `json:"role_name,omitempty"`
	SubjectType  string `json:"subject_type,omitempty"`
	SubjectID    string `json:"subject_id,omitempty"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Action       string `json:"action"`
	Effect       string `json:"effect"`
}

func (p *MatchedPolicy) AsMap() map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{
		"source":        p.Source,
		"role_name":     p.RoleName,
		"subject_type":  p.SubjectType,
		"subject_id":    p.SubjectID,
		"resource_type": p.ResourceType,
		"resource_id":   p.ResourceID,
		"action":        p.Action,
		"effect":        p.Effect,
	}
}

type InheritedResource struct {
	Resource ResourceRef `json:"resource"`
	Reason   string      `json:"reason"`
}

type DecisionLogEntry struct {
	RequestID     string
	SubjectType   string
	SubjectID     string
	Action        string
	ResourceType  string
	ResourceID    string
	Allowed       bool
	Reason        string
	MatchedPolicy map[string]any
	Sensitive     bool
	Context       map[string]any
}

func NormalizeType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func NormalizeAction(raw string) string {
	return strings.TrimSpace(raw)
}

func BuildPipelineID(path, name string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	name = strings.Trim(strings.TrimSpace(name), "/")
	if path == "" {
		return name
	}
	if name == "" {
		return path
	}
	return path + "/" + name
}

func SplitPipelineID(id string) (string, string) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	if id == "" {
		return "", ""
	}
	idx := strings.LastIndex(id, "/")
	if idx == -1 {
		return "", id
	}
	return id[:idx], id[idx+1:]
}

func BuildNamedResourceID(repoName, scope, name string) string {
	values := url.Values{}
	if repoName = strings.TrimSpace(repoName); repoName != "" {
		values.Set("repo", repoName)
	}
	if scope = strings.TrimSpace(scope); scope != "" {
		values.Set("scope", scope)
	}
	values.Set("name", strings.TrimSpace(name))
	return values.Encode()
}

func ParseNamedResourceID(id string) (repoName, scope, name string) {
	values, err := url.ParseQuery(strings.TrimSpace(id))
	if err != nil {
		return "", "", ""
	}
	return strings.TrimSpace(values.Get("repo")), strings.TrimSpace(values.Get("scope")), strings.TrimSpace(values.Get("name"))
}

func IsSensitiveAction(action string) bool {
	action = strings.TrimSpace(action)
	if action == "" {
		return false
	}
	if strings.HasSuffix(action, ".manage_acl") {
		return true
	}
	switch action {
	case "iam.admin",
		"system.update",
		"system.admin",
		"audit.read",
		"audit.export",
		"pipeline.execute",
		"pipeline.delete",
		"pipeline_run.rerun",
		"pipeline_run.cancel",
		"pipeline_run.delete",
		"trigger.update",
		"trigger.delete",
		"secret.read_value",
		"secret.write_value",
		"secret.delete",
		"variable.write_value",
		"variable.delete",
		"folder.move",
		"folder.delete",
		"step.manage":
		return true
	default:
		return false
	}
}
