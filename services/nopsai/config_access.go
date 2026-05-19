package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

type stringList []string

func (l *stringList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		item := strings.TrimSpace(value.Value)
		if item != "" {
			*l = append(*l, item)
		}
	case yaml.SequenceNode:
		for _, child := range value.Content {
			if child == nil {
				continue
			}
			if child.Kind != yaml.ScalarNode {
				return fmt.Errorf("list entries must be strings")
			}
			item := strings.TrimSpace(child.Value)
			if item != "" {
				*l = append(*l, item)
			}
		}
	default:
		return fmt.Errorf("must be a string or list of strings")
	}
	return nil
}

func (l stringList) values() []string {
	return append([]string{}, l...)
}

type accessConfigDocument struct {
	accessConfigPayload `yaml:",inline"`
	Access              *accessConfigPayload `yaml:"access" json:"access"`
}

type accessConfigPayload struct {
	Users                []accessUserFile        `yaml:"users" json:"users"`
	Groups               *yaml.Node              `yaml:"groups" json:"groups"`
	AuthGroups           *yaml.Node              `yaml:"auth_groups" json:"auth_groups"`
	Roles                *yaml.Node              `yaml:"roles" json:"roles"`
	AdvancedRoles        []accessRoleFile        `yaml:"advanced_roles" json:"advanced_roles"`
	Policies             []accessPolicyFile      `yaml:"policies" json:"policies"`
	RoleBindings         *yaml.Node              `yaml:"role_bindings" json:"role_bindings"`
	Bindings             *yaml.Node              `yaml:"bindings" json:"bindings"`
	AdvancedRoleBindings []accessRoleBindingFile `yaml:"advanced_role_bindings" json:"advanced_role_bindings"`
	Grants               *yaml.Node              `yaml:"grants" json:"grants"`
	AccessGrants         *yaml.Node              `yaml:"access_grants" json:"access_grants"`
	BasicRoles           []accessGrantFile       `yaml:"basic_roles" json:"basic_roles"`
}

func (d accessConfigDocument) effectivePayload() accessConfigPayload {
	payload := d.accessConfigPayload
	if d.Access == nil {
		return payload
	}
	payload.Users = append(payload.Users, d.Access.Users...)
	if payload.Groups == nil {
		payload.Groups = d.Access.Groups
	}
	if payload.AuthGroups == nil {
		payload.AuthGroups = d.Access.AuthGroups
	}
	if payload.Roles == nil {
		payload.Roles = d.Access.Roles
	}
	payload.AdvancedRoles = append(payload.AdvancedRoles, d.Access.AdvancedRoles...)
	payload.Policies = append(payload.Policies, d.Access.Policies...)
	if payload.RoleBindings == nil {
		payload.RoleBindings = d.Access.RoleBindings
	}
	if payload.Bindings == nil {
		payload.Bindings = d.Access.Bindings
	}
	payload.AdvancedRoleBindings = append(payload.AdvancedRoleBindings, d.Access.AdvancedRoleBindings...)
	if payload.Grants == nil {
		payload.Grants = d.Access.Grants
	}
	if payload.AccessGrants == nil {
		payload.AccessGrants = d.Access.AccessGrants
	}
	payload.BasicRoles = append(payload.BasicRoles, d.Access.BasicRoles...)
	return payload
}

type accessUserFile struct {
	ID            string     `yaml:"id" json:"id"`
	Sub           string     `yaml:"sub" json:"sub"`
	Email         string     `yaml:"email" json:"email"`
	Provider      string     `yaml:"provider" json:"provider"`
	Status        string     `yaml:"status" json:"status"`
	Password      string     `yaml:"password" json:"password"`
	PasswordHash  string     `yaml:"password_hash" json:"password_hash"`
	Role          *yaml.Node `yaml:"role" json:"role"`
	Roles         *yaml.Node `yaml:"roles" json:"roles"`
	AdvancedRole  string     `yaml:"advanced_role" json:"advanced_role"`
	AdvancedRoles stringList `yaml:"advanced_roles" json:"advanced_roles"`
}

type accessRoleFile struct {
	Name        string                  `yaml:"name" json:"name"`
	Role        string                  `yaml:"role" json:"role"`
	Description string                  `yaml:"description" json:"description"`
	Policies    []accessPolicyFile      `yaml:"policies" json:"policies"`
	Bindings    []accessRoleBindingFile `yaml:"bindings" json:"bindings"`
}

type accessPolicyFile struct {
	Role         string `yaml:"role" json:"role"`
	Name         string `yaml:"name" json:"name"`
	Object       string `yaml:"obj" json:"obj"`
	ObjectName   string `yaml:"object" json:"object"`
	Resource     string `yaml:"resource" json:"resource"`
	Action       string `yaml:"act" json:"act"`
	ActionName   string `yaml:"action" json:"action"`
	Effect       string `yaml:"effect" json:"effect"`
	ResourceType string `yaml:"resource_type" json:"resource_type"`
	ResourceID   string `yaml:"resource_id" json:"resource_id"`
}

type accessSubjectFile struct {
	Type    string `yaml:"type" json:"type"`
	ID      string `yaml:"id" json:"id"`
	User    string `yaml:"user" json:"user"`
	Group   string `yaml:"group" json:"group"`
	Service string `yaml:"service" json:"service"`
}

type accessRoleBindingFile struct {
	Role        string `yaml:"role" json:"role"`
	SubjectType string `yaml:"subject_type" json:"subject_type"`
	SubjectID   string `yaml:"subject_id" json:"subject_id"`
	User        string `yaml:"user" json:"user"`
	Group       string `yaml:"group" json:"group"`
	Service     string `yaml:"service" json:"service"`
}

type accessGrantFile struct {
	SubjectType  string `yaml:"subject_type" json:"subject_type"`
	SubjectID    string `yaml:"subject_id" json:"subject_id"`
	User         string `yaml:"user" json:"user"`
	Group        string `yaml:"group" json:"group"`
	Service      string `yaml:"service" json:"service"`
	Role         string `yaml:"role" json:"role"`
	Resource     string `yaml:"resource" json:"resource"`
	ResourceType string `yaml:"resource_type" json:"resource_type"`
	ResourceID   string `yaml:"resource_id" json:"resource_id"`
	Inherit      *bool  `yaml:"inherit" json:"inherit"`
}

type embeddedResourceAccessDocument struct {
	Access *embeddedResourceAccessFile `yaml:"access" json:"access"`
}

type embeddedResourceAccessFile struct {
	Visibility   string                         `yaml:"visibility" json:"visibility"`
	UseAccess    *embeddedResourceUseAccessFile `yaml:"use_access" json:"use_access"`
	Grants       []embeddedResourceUseGrantFile `yaml:"grants" json:"grants"`
	Groups       stringList                     `yaml:"groups" json:"groups"`
	Repositories stringList                     `yaml:"repositories" json:"repositories"`
}

type embeddedResourceUseAccessFile struct {
	Mode         string                         `yaml:"mode" json:"mode"`
	Grants       []embeddedResourceUseGrantFile `yaml:"grants" json:"grants"`
	Groups       stringList                     `yaml:"groups" json:"groups"`
	Repositories stringList                     `yaml:"repositories" json:"repositories"`
}

type embeddedResourceUseGrantFile struct {
	SubjectType    string         `yaml:"subject_type" json:"subject_type"`
	SubjectID      string         `yaml:"subject_id" json:"subject_id"`
	Group          string         `yaml:"group" json:"group"`
	Repository     string         `yaml:"repository" json:"repository"`
	Repo           string         `yaml:"repo" json:"repo"`
	User           string         `yaml:"user" json:"user"`
	Trigger        string         `yaml:"trigger" json:"trigger"`
	Service        string         `yaml:"service" json:"service"`
	ServiceAccount string         `yaml:"service_account" json:"service_account"`
	Actions        stringList     `yaml:"actions" json:"actions"`
	Conditions     map[string]any `yaml:"conditions" json:"conditions"`
}

type accessSyncPlan struct {
	users          map[string]storedAccessUser
	roles          map[string]storedAccessRole
	policies       map[accessRolePolicyKey]storedAccessPolicy
	roleBindings   map[accessRoleBindingKey]storedAccessRoleBinding
	grants         map[accessGrantPlanKey]storedAccessGrant
	resourceAccess map[resourceAccessPlanKey]storedResourceAccess
}

type storedAccessUser struct {
	id           string
	sub          string
	email        string
	provider     string
	status       string
	password     string
	passwordHash string
	sourcePath   string
}

type storedAccessRole struct {
	name        string
	description string
	sourcePath  string
}

type storedAccessPolicy struct {
	role         string
	name         string
	resourceType string
	resourceID   string
	action       string
	effect       string
	sourcePath   string
}

type storedAccessRoleBinding struct {
	role        string
	subjectType string
	subjectID   string
	sourcePath  string
}

type storedAccessGrant struct {
	subjectType  string
	subjectID    string
	role         string
	resourceType string
	resourceID   string
	inherit      bool
	actions      []string
	sourcePath   string
}

type storedResourceAccess struct {
	resourceType  string
	resourceID    string
	visibility    string
	visibilitySet bool
	sourcePath    string
}

type accessRolePolicyKey struct {
	role         string
	resourceType string
	resourceID   string
	action       string
	effect       string
}

type accessRoleBindingKey struct {
	role        string
	subjectType string
	subjectID   string
}

type accessGrantPlanKey struct {
	subjectType  string
	subjectID    string
	resourceType string
	resourceID   string
}

type resourceAccessPlanKey struct {
	resourceType string
	resourceID   string
}

type resolvedAccessGrantKey struct {
	subjectType  string
	subjectID    string
	resourceType string
	resourceID   string
}

func newAccessSyncPlan() accessSyncPlan {
	return accessSyncPlan{
		users:          map[string]storedAccessUser{},
		roles:          map[string]storedAccessRole{},
		policies:       map[accessRolePolicyKey]storedAccessPolicy{},
		roleBindings:   map[accessRoleBindingKey]storedAccessRoleBinding{},
		grants:         map[accessGrantPlanKey]storedAccessGrant{},
		resourceAccess: map[resourceAccessPlanKey]storedResourceAccess{},
	}
}

func parseAccessSyncPlan(files map[string]string, accessDir string, binding models.ConfigRepository, boundFolder string) (accessSyncPlan, error) {
	plan := newAccessSyncPlan()
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, accessDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}
		var doc accessConfigDocument
		if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
			return plan, fmt.Errorf("failed to parse access manifest '%s': %w", normalized, err)
		}
		if err := plan.addPayload(doc.effectivePayload(), binding, boundFolder, normalized); err != nil {
			return plan, fmt.Errorf("invalid access manifest '%s': %w", normalized, err)
		}
	}
	if err := validateAccessPlanForBinding(plan, binding); err != nil {
		return plan, err
	}
	return plan, nil
}

func (p accessSyncPlan) addPayload(payload accessConfigPayload, binding models.ConfigRepository, boundFolder, sourcePath string) error {
	if payload.Groups != nil || payload.AuthGroups != nil {
		return fmt.Errorf("access manifests do not support auth groups; grant users or services to folder resources instead")
	}
	if payload.Roles != nil {
		return fmt.Errorf("access manifests use advanced_roles for advanced role definitions")
	}
	if payload.RoleBindings != nil || payload.Bindings != nil {
		return fmt.Errorf("access manifests use advanced_role_bindings for advanced role assignments")
	}
	if payload.Grants != nil || payload.AccessGrants != nil {
		return fmt.Errorf("access manifests use basic_roles for basic product role grants")
	}

	for _, raw := range payload.Users {
		if raw.Role != nil || raw.Roles != nil {
			return fmt.Errorf("user %q uses ambiguous roles; use advanced_roles for advanced role assignments", strings.TrimSpace(raw.Sub))
		}
		user, err := normalizeAccessUser(raw, sourcePath)
		if err != nil {
			return err
		}
		if _, exists := p.users[user.sub]; exists {
			return fmt.Errorf("duplicate user %q", user.sub)
		}
		p.users[user.sub] = user
		roles := raw.AdvancedRoles.values()
		if role := strings.TrimSpace(raw.AdvancedRole); role != "" {
			roles = append(roles, role)
		}
		for _, role := range roles {
			binding := storedAccessRoleBinding{
				role:        strings.TrimSpace(role),
				subjectType: model.SubjectTypeUser,
				subjectID:   user.sub,
				sourcePath:  sourcePath,
			}
			if err := p.addRoleBinding(binding); err != nil {
				return err
			}
		}
	}

	for _, raw := range payload.AdvancedRoles {
		roleName := firstNonEmptyString(raw.Name, raw.Role)
		role, err := normalizeAccessRole(roleName, raw.Description, sourcePath)
		if err != nil {
			return err
		}
		if _, exists := p.roles[role.name]; exists {
			return fmt.Errorf("duplicate role %q", role.name)
		}
		p.roles[role.name] = role
		for _, rawPolicy := range raw.Policies {
			if strings.TrimSpace(rawPolicy.Role) == "" {
				rawPolicy.Role = role.name
			}
			policy, err := normalizeAccessPolicy(rawPolicy, sourcePath)
			if err != nil {
				return err
			}
			if err := p.addPolicy(policy); err != nil {
				return err
			}
		}
		for _, rawBinding := range raw.Bindings {
			if strings.TrimSpace(rawBinding.Role) == "" {
				rawBinding.Role = role.name
			}
			binding, err := normalizeAccessRoleBinding(rawBinding, sourcePath)
			if err != nil {
				return err
			}
			if err := p.addRoleBinding(binding); err != nil {
				return err
			}
		}
	}

	for _, raw := range payload.Policies {
		policy, err := normalizeAccessPolicy(raw, sourcePath)
		if err != nil {
			return err
		}
		if err := p.addPolicy(policy); err != nil {
			return err
		}
	}

	for _, raw := range payload.AdvancedRoleBindings {
		binding, err := normalizeAccessRoleBinding(raw, sourcePath)
		if err != nil {
			return err
		}
		if err := p.addRoleBinding(binding); err != nil {
			return err
		}
	}

	for _, raw := range payload.BasicRoles {
		grant, err := normalizeAccessGrant(raw, binding, boundFolder, sourcePath)
		if err != nil {
			return err
		}
		if err := p.addGrant(grant); err != nil {
			return err
		}
	}

	return nil
}

func (p accessSyncPlan) addGrant(grant storedAccessGrant) error {
	key := accessGrantPlanKey{
		subjectType:  grant.subjectType,
		subjectID:    grant.subjectID,
		resourceType: grant.resourceType,
		resourceID:   grant.resourceID,
	}
	if _, exists := p.grants[key]; exists {
		return fmt.Errorf("duplicate access grant for %s:%s on %s:%s", grant.subjectType, grant.subjectID, grant.resourceType, grant.resourceID)
	}
	p.grants[key] = grant
	return nil
}

func (p accessSyncPlan) addEmbeddedResourceAccess(content, sourcePath, resourceType, resourceID string, binding models.ConfigRepository, boundFolder string) error {
	resourceAccess, grants, ok, err := parseEmbeddedResourceAccess(content, sourcePath, resourceType, resourceID, binding, boundFolder)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if resourceAccess.visibilitySet {
		key := resourceAccessPlanKey{resourceType: resourceAccess.resourceType, resourceID: resourceAccess.resourceID}
		if existing, exists := p.resourceAccess[key]; exists && existing.visibility != resourceAccess.visibility {
			return fmt.Errorf("duplicate resource access visibility for %s:%s", resourceAccess.resourceType, resourceAccess.resourceID)
		}
		p.resourceAccess[key] = resourceAccess
	}
	for _, grant := range grants {
		if err := p.addGrant(grant); err != nil {
			return err
		}
	}
	return nil
}

func (p accessSyncPlan) addPolicy(policy storedAccessPolicy) error {
	if isProtectedAdminRoleName(policy.role) {
		return fmt.Errorf("default roles cannot be modified")
	}
	key := accessRolePolicyKey{
		role:         policy.role,
		resourceType: policy.resourceType,
		resourceID:   policy.resourceID,
		action:       policy.action,
		effect:       policy.effect,
	}
	if _, exists := p.policies[key]; exists {
		return fmt.Errorf("duplicate policy for role %q on %s:%s %s %s", policy.role, policy.resourceType, policy.resourceID, policy.effect, policy.action)
	}
	p.policies[key] = policy
	return nil
}

func (p accessSyncPlan) addRoleBinding(binding storedAccessRoleBinding) error {
	if strings.TrimSpace(binding.role) == "" {
		return fmt.Errorf("role binding is missing role")
	}
	key := accessRoleBindingKey{
		role:        binding.role,
		subjectType: binding.subjectType,
		subjectID:   binding.subjectID,
	}
	if _, exists := p.roleBindings[key]; exists {
		return fmt.Errorf("duplicate role binding for role %q and subject %s:%s", binding.role, binding.subjectType, binding.subjectID)
	}
	p.roleBindings[key] = binding
	return nil
}

func normalizeAccessUser(raw accessUserFile, sourcePath string) (storedAccessUser, error) {
	sub := strings.TrimSpace(raw.Sub)
	if sub == "" {
		return storedAccessUser{}, fmt.Errorf("user is missing sub")
	}
	if strings.EqualFold(sub, defaultAdminSub) {
		return storedAccessUser{}, fmt.Errorf("default admin user cannot be managed by GitOps")
	}
	email, err := normalizeOptionalEmail(raw.Email)
	if err != nil {
		return storedAccessUser{}, err
	}
	provider := strings.TrimSpace(raw.Provider)
	if provider == "" {
		provider = "local"
	}
	status := strings.ToLower(strings.TrimSpace(raw.Status))
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "disabled":
	default:
		return storedAccessUser{}, fmt.Errorf("user %q has invalid status %q", sub, raw.Status)
	}
	if raw.ID != "" {
		if _, err := uuid.Parse(strings.TrimSpace(raw.ID)); err != nil {
			return storedAccessUser{}, fmt.Errorf("user %q has invalid id", sub)
		}
	}
	return storedAccessUser{
		id:           strings.TrimSpace(raw.ID),
		sub:          sub,
		email:        email,
		provider:     provider,
		status:       status,
		password:     strings.TrimSpace(raw.Password),
		passwordHash: strings.TrimSpace(raw.PasswordHash),
		sourcePath:   sourcePath,
	}, nil
}

func normalizeAccessRole(name, description, sourcePath string) (storedAccessRole, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return storedAccessRole{}, fmt.Errorf("role is missing name")
	}
	if isProtectedAdminRoleName(name) {
		return storedAccessRole{}, fmt.Errorf("default roles cannot be modified")
	}
	return storedAccessRole{
		name:        name,
		description: strings.TrimSpace(description),
		sourcePath:  sourcePath,
	}, nil
}

func normalizeAccessPolicy(raw accessPolicyFile, sourcePath string) (storedAccessPolicy, error) {
	req := createRoleRequest{
		Role:         strings.TrimSpace(raw.Role),
		Name:         strings.TrimSpace(raw.Name),
		Object:       firstNonEmptyString(raw.Object, raw.ObjectName, raw.Resource),
		Action:       firstNonEmptyString(raw.Action, raw.ActionName),
		Effect:       strings.TrimSpace(raw.Effect),
		ResourceType: strings.TrimSpace(raw.ResourceType),
		ResourceID:   strings.TrimSpace(raw.ResourceID),
	}
	permission, err := parseAdminRolePermission(req)
	if err != nil {
		return storedAccessPolicy{}, err
	}
	if permission.Role == "" {
		return storedAccessPolicy{}, fmt.Errorf("policy is missing role")
	}
	return storedAccessPolicy{
		role:         permission.Role,
		name:         permission.Name,
		resourceType: permission.ResourceType,
		resourceID:   permission.ResourceID,
		action:       permission.Action,
		effect:       permission.Effect,
		sourcePath:   sourcePath,
	}, nil
}

func normalizeAccessRoleBinding(raw accessRoleBindingFile, sourcePath string) (storedAccessRoleBinding, error) {
	subjectType, subjectID, err := normalizeAccessSubject(raw.SubjectType, raw.SubjectID, raw.User, raw.Group, raw.Service)
	if err != nil {
		return storedAccessRoleBinding{}, err
	}
	return storedAccessRoleBinding{
		role:        strings.TrimSpace(raw.Role),
		subjectType: subjectType,
		subjectID:   subjectID,
		sourcePath:  sourcePath,
	}, nil
}

func normalizeAccessSubject(subjectType, subjectID, userID, groupID, serviceID string) (string, string, error) {
	subjectType = strings.TrimSpace(subjectType)
	subjectID = strings.TrimSpace(subjectID)
	switch {
	case strings.TrimSpace(userID) != "":
		subjectType = model.SubjectTypeUser
		subjectID = strings.TrimSpace(userID)
	case strings.TrimSpace(groupID) != "":
		return "", "", fmt.Errorf("auth group subjects are not supported in access manifests; use user or service subjects and target folders with resource_type: folder")
	case strings.TrimSpace(serviceID) != "":
		subjectType = model.SubjectTypeInternalService
		subjectID = strings.TrimSpace(serviceID)
	}
	normalizedType, err := normalizeAccessGrantSubjectType(subjectType)
	if err != nil {
		return "", "", err
	}
	if normalizedType == model.SubjectTypeAuthGroup {
		return "", "", fmt.Errorf("auth group subjects are not supported in access manifests; use user or service subjects and target folders with resource_type: folder")
	}
	if subjectID == "" {
		return "", "", fmt.Errorf("subject_id is required")
	}
	return normalizedType, subjectID, nil
}

func normalizeAccessGrant(raw accessGrantFile, binding models.ConfigRepository, boundFolder, sourcePath string) (storedAccessGrant, error) {
	subjectType, subjectID, err := normalizeAccessSubject(raw.SubjectType, raw.SubjectID, raw.User, raw.Group, raw.Service)
	if err != nil {
		return storedAccessGrant{}, err
	}
	roleName, err := normalizeProductRoleName(raw.Role)
	if err != nil {
		return storedAccessGrant{}, err
	}

	resourceType := strings.TrimSpace(raw.ResourceType)
	resourceID := strings.TrimSpace(raw.ResourceID)
	if raw.Resource != "" {
		parsedType, parsedID, ok := strings.Cut(strings.TrimSpace(raw.Resource), ":")
		if !ok {
			return storedAccessGrant{}, fmt.Errorf("resource must use resource_type:resource_id syntax")
		}
		if resourceType == "" {
			resourceType = parsedType
		}
		if resourceID == "" {
			resourceID = parsedID
		}
	}
	resourceType, err = normalizeAccessGrantResourceType(resourceType)
	if err != nil {
		return storedAccessGrant{}, err
	}
	resourceID, err = normalizeAccessGrantResourceIDForBinding(resourceType, resourceID, binding, boundFolder)
	if err != nil {
		return storedAccessGrant{}, err
	}

	inherit := resourceType == grantResourceFolder
	if raw.Inherit != nil {
		inherit = *raw.Inherit
	}
	if binding.ScopeType == models.ConfigRepositoryScopeFolder {
		if roleName == productRoleAdmin || resourceType == grantResourcePlatform {
			return storedAccessGrant{}, fmt.Errorf("group-scoped config repositories cannot grant platform admin access")
		}
		if !accessGrantResourceUnderBindingScope(resourceType, resourceID, boundFolder) {
			return storedAccessGrant{}, fmt.Errorf("access grant target %s:%s is outside group scope %q", resourceType, resourceID, boundFolder)
		}
	}

	return storedAccessGrant{
		subjectType:  subjectType,
		subjectID:    subjectID,
		role:         roleName,
		resourceType: resourceType,
		resourceID:   resourceID,
		inherit:      inherit,
		sourcePath:   sourcePath,
	}, nil
}

func parseEmbeddedResourceAccess(content, sourcePath, resourceType, resourceID string, binding models.ConfigRepository, boundFolder string) (storedResourceAccess, []storedAccessGrant, bool, error) {
	var doc embeddedResourceAccessDocument
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return storedResourceAccess{}, nil, false, fmt.Errorf("failed to parse embedded access: %w", err)
	}
	if doc.Access == nil {
		return storedResourceAccess{}, nil, false, nil
	}

	resourceType, err := normalizeAccessGrantResourceType(resourceType)
	if err != nil {
		return storedResourceAccess{}, nil, true, err
	}
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	resourceAccess := storedResourceAccess{
		resourceType: resourceType,
		resourceID:   resourceID,
		sourcePath:   sourcePath,
	}

	rawGrants := embeddedResourceAccessGrants(*doc.Access)
	visibility := firstNonEmptyString(doc.Access.Visibility, embeddedResourceUseAccessMode(doc.Access.UseAccess))
	if visibility == "" && len(rawGrants) > 0 {
		visibility = resourceVisibilityRestricted
	}
	if visibility != "" {
		normalizedVisibility, err := normalizeResourceVisibilityUpdate(visibility)
		if err != nil {
			return storedResourceAccess{}, nil, true, err
		}
		if err := validateResourceVisibilityPolicy(resourceType, normalizedVisibility); err != nil {
			return storedResourceAccess{}, nil, true, err
		}
		resourceAccess.visibility = normalizedVisibility
		resourceAccess.visibilitySet = true
	}

	grants := make([]storedAccessGrant, 0, len(rawGrants))
	for _, rawGrant := range rawGrants {
		grant, err := normalizeEmbeddedResourceUseGrant(rawGrant, resourceType, resourceID, sourcePath)
		if err != nil {
			return storedResourceAccess{}, nil, true, err
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			if !accessGrantResourceUnderBindingScope(grant.resourceType, grant.resourceID, boundFolder) {
				return storedResourceAccess{}, nil, true, fmt.Errorf("resource access target %s:%s is outside group scope %q", grant.resourceType, grant.resourceID, boundFolder)
			}
		}
		grants = append(grants, grant)
	}

	return resourceAccess, grants, true, nil
}

func embeddedResourceUseAccessMode(useAccess *embeddedResourceUseAccessFile) string {
	if useAccess == nil {
		return ""
	}
	return useAccess.Mode
}

func embeddedResourceAccessGrants(access embeddedResourceAccessFile) []embeddedResourceUseGrantFile {
	var grants []embeddedResourceUseGrantFile
	grants = append(grants, access.Grants...)
	for _, group := range access.Groups.values() {
		grants = append(grants, embeddedResourceUseGrantFile{Group: group})
	}
	for _, repo := range access.Repositories.values() {
		grants = append(grants, embeddedResourceUseGrantFile{Repository: repo})
	}
	if access.UseAccess != nil {
		grants = append(grants, access.UseAccess.Grants...)
		for _, group := range access.UseAccess.Groups.values() {
			grants = append(grants, embeddedResourceUseGrantFile{Group: group})
		}
		for _, repo := range access.UseAccess.Repositories.values() {
			grants = append(grants, embeddedResourceUseGrantFile{Repository: repo})
		}
	}
	return grants
}

func normalizeEmbeddedResourceUseGrant(raw embeddedResourceUseGrantFile, resourceType, resourceID, sourcePath string) (storedAccessGrant, error) {
	subjectType, subjectID, err := normalizeEmbeddedResourceUseGrantSubject(raw)
	if err != nil {
		return storedAccessGrant{}, err
	}
	if err := validateResourceGrantConditions(raw.Conditions); err != nil {
		return storedAccessGrant{}, err
	}
	actions, err := normalizeUseGrantActions(resourceType, raw.Actions.values())
	if err != nil {
		return storedAccessGrant{}, err
	}
	return storedAccessGrant{
		subjectType:  subjectType,
		subjectID:    subjectID,
		role:         customUseGrantRole,
		resourceType: resourceType,
		resourceID:   resourceID,
		inherit:      false,
		actions:      actions,
		sourcePath:   sourcePath,
	}, nil
}

func normalizeEmbeddedResourceUseGrantSubject(raw embeddedResourceUseGrantFile) (string, string, error) {
	subjectType := strings.TrimSpace(raw.SubjectType)
	subjectID := strings.TrimSpace(raw.SubjectID)
	setSubject := func(nextType, nextID string) error {
		nextID = strings.TrimSpace(nextID)
		if nextID == "" {
			return nil
		}
		if subjectType != "" || subjectID != "" {
			return fmt.Errorf("resource access grant has multiple subjects")
		}
		subjectType = nextType
		subjectID = nextID
		return nil
	}

	for _, candidate := range []struct {
		subjectType string
		subjectID   string
	}{
		{grantSubjectGroup, raw.Group},
		{model.SubjectTypeRepository, firstNonEmptyString(raw.Repository, raw.Repo)},
		{model.SubjectTypeUser, raw.User},
		{model.SubjectTypeTrigger, raw.Trigger},
		{model.SubjectTypeServiceAccount, raw.ServiceAccount},
		{model.SubjectTypeInternalService, raw.Service},
	} {
		if err := setSubject(candidate.subjectType, candidate.subjectID); err != nil {
			return "", "", err
		}
	}

	if subjectID == "" {
		return "", "", fmt.Errorf("resource access grant is missing subject_id")
	}
	subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
	if subjectID == "" {
		return "", "", fmt.Errorf("resource access grant is missing subject_id")
	}
	switch strings.ToLower(strings.TrimSpace(subjectType)) {
	case grantSubjectGroup, grantResourceFolder, grantResourceTeam, "resource_group":
		return grantSubjectGroup, subjectID, nil
	default:
		normalizedType, err := normalizeAccessGrantSubjectType(subjectType)
		if err != nil {
			return "", "", err
		}
		return normalizedType, subjectID, nil
	}
}

func normalizeAccessGrantResourceIDForBinding(resourceType, resourceID string, binding models.ConfigRepository, boundFolder string) (string, error) {
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	if resourceType == grantResourcePlatform {
		return platformGrantID, nil
	}
	if resourceType == grantResourceKnowledgeContext {
		kind, group, name, err := splitKnowledgeContextIdentifier(resourceID)
		if err != nil {
			return "", err
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			group, err = normalizeConfigPathForFolder(boundFolder, group)
			if err != nil {
				return "", err
			}
		}
		return buildKnowledgeContextIdentifier(kind, group, name), nil
	}
	if resourceType == grantResourceSecret || resourceType == grantResourceVariable {
		if !strings.Contains(resourceID, "=") {
			resourceID = model.BuildNamedResourceID("", "", resourceID)
		}
		resourceID = runtimeNamedResourceIDForResource(resourceID)
		if binding.ScopeType != models.ConfigRepositoryScopeFolder {
			return resourceID, nil
		}
		repoName, scope, name := model.ParseNamedResourceID(resourceID)
		if name == "" {
			return "", fmt.Errorf("%s grant resource_id must include a name", resourceType)
		}
		var err error
		if repoName != "" {
			repoName, err = normalizeConfigPathForFolder(boundFolder, repoName)
			if err != nil {
				return "", err
			}
		}
		if scope != "" {
			scope, err = normalizeConfigPathForFolder(boundFolder, scope)
			if err != nil {
				return "", err
			}
		}
		return model.BuildNamedResourceID(repoName, scope, name), nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeFolder {
		return resourceID, nil
	}
	if isGeneralGrantResourceID(resourceID) {
		return generalGrantID, nil
	}
	return normalizeConfigPathForFolder(boundFolder, resourceID)
}

func validateAccessPlanForBinding(plan accessSyncPlan, binding models.ConfigRepository) error {
	if binding.ScopeType != models.ConfigRepositoryScopeFolder {
		return nil
	}
	if len(plan.users) > 0 {
		return fmt.Errorf("group-scoped config repositories cannot manage users")
	}
	if len(plan.roles) > 0 || len(plan.policies) > 0 || len(plan.roleBindings) > 0 {
		return fmt.Errorf("group-scoped config repositories cannot manage global roles, policies, or role bindings")
	}
	return nil
}

func accessGrantResourceUnderBindingScope(resourceType, resourceID, boundFolder string) bool {
	boundFolder = strings.Trim(strings.TrimSpace(boundFolder), "/")
	if boundFolder == "" {
		return false
	}
	switch resourceType {
	case grantResourceSecret, grantResourceVariable:
		repoName, scope, name := model.ParseNamedResourceID(resourceID)
		if name == "" {
			return false
		}
		checked := false
		if repoName != "" {
			checked = true
			if !configResourceUnderScope(repoName, boundFolder) {
				return false
			}
		}
		if scope != "" {
			checked = true
			if !configResourceUnderScope(scope, boundFolder) {
				return false
			}
		}
		return checked
	case grantResourceKnowledgeContext:
		_, group, _, err := splitKnowledgeContextIdentifier(resourceID)
		return err == nil && configResourceUnderScope(group, boundFolder)
	case grantResourcePlatform:
		return false
	default:
		return configResourceUnderScope(resourceID, boundFolder)
	}
}

func accessGrantResourceIntersectsAnyScope(resourceType, resourceID string, scopes []string) bool {
	for _, scope := range scopes {
		switch resourceType {
		case grantResourceSecret, grantResourceVariable:
			repoName, secretScope, name := model.ParseNamedResourceID(resourceID)
			if name == "" {
				continue
			}
			if repoName != "" && configResourceUnderScope(repoName, scope) {
				return true
			}
			if secretScope != "" && configResourceUnderScope(secretScope, scope) {
				return true
			}
		case grantResourceKnowledgeContext:
			_, group, _, err := splitKnowledgeContextIdentifier(resourceID)
			if err == nil && configResourceUnderScope(group, scope) {
				return true
			}
		default:
			if configResourceUnderScope(resourceID, scope) {
				return true
			}
		}
	}
	return false
}

func filterDelegatedAccessResources(plan accessSyncPlan, binding models.ConfigRepository, overrideScopes []string) {
	if len(overrideScopes) == 0 {
		return
	}
	for key, grant := range plan.grants {
		if (binding.ScopeType == models.ConfigRepositoryScopeFolder || grant.role == customUseGrantRole) &&
			accessGrantResourceIntersectsAnyScope(grant.resourceType, grant.resourceID, overrideScopes) {
			delete(plan.grants, key)
		}
	}
	for key, access := range plan.resourceAccess {
		if accessGrantResourceIntersectsAnyScope(access.resourceType, access.resourceID, overrideScopes) {
			delete(plan.resourceAccess, key)
		}
	}
}

func (a *App) syncAccessConfiguration(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan accessSyncPlan, commitSHA string, details map[string]int) error {
	if err := resetManagedAccessLinks(ctx, tx, binding.ID); err != nil {
		return err
	}

	for _, user := range plan.users {
		if err := upsertAccessUser(ctx, tx, binding, user, commitSHA); err != nil {
			return err
		}
		details["access_users_synced"]++
	}
	if err := pruneStaleAccessGrantsForManagedUsers(ctx, tx, plan.users); err != nil {
		return err
	}
	for _, role := range plan.roles {
		if err := upsertAccessRole(ctx, tx, binding, role, commitSHA); err != nil {
			return err
		}
		details["access_roles_synced"]++
	}
	for _, policy := range plan.policies {
		if err := upsertAccessPolicy(ctx, tx, binding, policy, commitSHA); err != nil {
			return err
		}
		details["access_policies_synced"]++
	}
	for _, roleBinding := range plan.roleBindings {
		if err := upsertAccessRoleBinding(ctx, tx, binding, roleBinding, commitSHA); err != nil {
			return err
		}
		details["access_role_bindings_synced"]++
	}

	if err := syncResourceVisibilities(ctx, tx, plan, details); err != nil {
		return err
	}

	grantKeys, err := a.syncAccessGrants(ctx, tx, binding, plan, commitSHA, details)
	if err != nil {
		return err
	}
	if err := pruneManagedAccessConfiguration(ctx, tx, binding, plan, grantKeys); err != nil {
		return err
	}
	if err := clearResourceAccessOverridesForConfigSync(ctx, tx, binding); err != nil {
		return err
	}
	return nil
}

func resetManagedAccessLinks(ctx context.Context, tx pgx.Tx, configRepoID int64) error {
	statements := []string{
		`DELETE FROM user_roles WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`,
		`DELETE FROM auth_role_bindings WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`,
		`DELETE FROM auth_role_permissions WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`,
		`DELETE FROM role_permissions WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, configRepoID); err != nil {
			return fmt.Errorf("failed to reset managed access links: %w", err)
		}
	}
	return nil
}

func upsertAccessUser(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, user storedAccessUser, commitSHA string) error {
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "users", "user", user.sub, "sub = $1", user.sub); err != nil {
		return err
	}
	passwordHash := strings.TrimSpace(user.passwordHash)
	if passwordHash == "" && strings.TrimSpace(user.password) != "" {
		hashed, err := auth.HashPassword(user.password)
		if err != nil {
			return fmt.Errorf("failed to hash password for user %q: %w", user.sub, err)
		}
		passwordHash = hashed
	}
	userID := uuid.New()
	if user.id != "" {
		parsedID, err := uuid.Parse(user.id)
		if err != nil {
			return fmt.Errorf("user %q has invalid id: %w", user.sub, err)
		}
		userID = parsedID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO users (
			id, sub, email, provider, password_hash, status,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, TRUE)
		ON CONFLICT (sub) DO UPDATE SET
			email = EXCLUDED.email,
			provider = EXCLUDED.provider,
			password_hash = COALESCE(EXCLUDED.password_hash, users.password_hash),
			status = EXCLUDED.status,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, userID, user.sub, user.email, user.provider, passwordHash, user.status, binding.ID, user.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to upsert user %q: %w", user.sub, err)
	}
	return nil
}

func upsertAccessRole(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, role storedAccessRole, commitSHA string) error {
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "auth_roles", "role", role.name, "name = $1", role.name); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO auth_roles (
			name, description,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		ON CONFLICT (name) DO UPDATE SET
			description = EXCLUDED.description,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()
	`, role.name, role.description, binding.ID, role.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to upsert role %q: %w", role.name, err)
	}
	return nil
}

func upsertAccessPolicy(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, policy storedAccessPolicy, commitSHA string) error {
	if err := ensureAccessRolePrepared(ctx, tx, binding, policy.role, policy.sourcePath, commitSHA); err != nil {
		return err
	}
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "auth_role_permissions", "role policy", policy.role, "role_name = $1 AND resource_type = $2 AND resource_id = $3 AND action = $4 AND effect = $5", policy.role, policy.resourceType, policy.resourceID, policy.action, policy.effect); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO auth_role_permissions (
			role_name, resource_type, resource_id, action, effect,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE)
		ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO UPDATE SET
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, policy.role, policy.resourceType, policy.resourceID, policy.action, policy.effect, binding.ID, policy.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to upsert role policy for %q: %w", policy.role, err)
	}

	objectValue := formatAdminPermissionObject(policy.resourceType, policy.resourceID)
	actionValue := formatAdminPermissionAction(policy.effect, policy.action)
	displayName := adminPermissionDisplayName(policy.name, objectValue, actionValue)
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "role_permissions", "role policy metadata", policy.role, "role = $1 AND obj = $2 AND act = $3", policy.role, objectValue, actionValue); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role = $1 AND obj = $2 AND act = $3`, policy.role, objectValue, actionValue); err != nil {
		return fmt.Errorf("failed to refresh role policy metadata: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO role_permissions (
			role, name, obj, act,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)
	`, policy.role, displayName, objectValue, actionValue, binding.ID, policy.sourcePath, commitSHA); err != nil {
		return fmt.Errorf("failed to upsert role policy metadata for %q: %w", policy.role, err)
	}
	return nil
}

func upsertAccessRoleBinding(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, roleBinding storedAccessRoleBinding, commitSHA string) error {
	if err := ensureAccessRolePrepared(ctx, tx, binding, roleBinding.role, roleBinding.sourcePath, commitSHA); err != nil {
		return err
	}
	subject, err := resolveAccessGrantSubject(ctx, tx, roleBinding.subjectType, roleBinding.subjectID)
	if err != nil {
		return fmt.Errorf("failed to resolve role binding subject %s:%s: %w", roleBinding.subjectType, roleBinding.subjectID, err)
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return err
	} else if locked {
		return fmt.Errorf("cannot modify default admin role assignments")
	}
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "auth_role_bindings", "role binding", roleBinding.role, "role_name = $1 AND subject_type = $2 AND subject_id = $3", roleBinding.role, subject.Type, subject.ID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO auth_role_bindings (
			role_name, subject_type, subject_id,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		ON CONFLICT (role_name, subject_type, subject_id) DO UPDATE SET
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, roleBinding.role, subject.Type, subject.ID, binding.ID, roleBinding.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to upsert role binding for %q: %w", roleBinding.role, err)
	}
	if subject.Type == model.SubjectTypeUser {
		if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "user_roles", "user role", roleBinding.role, "user_id = $1 AND role = $2", subject.ID, roleBinding.role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (
				user_id, role,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, TRUE)
			ON CONFLICT (user_id, role) DO UPDATE SET
				config_repo_id = EXCLUDED.config_repo_id,
				config_source_path = EXCLUDED.config_source_path,
				config_source_commit_sha = EXCLUDED.config_source_commit_sha,
				managed_by_config_repo = TRUE
		`, subject.ID, roleBinding.role, binding.ID, roleBinding.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert legacy user role for %q: %w", roleBinding.role, err)
		}
	}
	return nil
}

func ensureAccessRolePrepared(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, roleName, sourcePath, commitSHA string) error {
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return fmt.Errorf("role is required")
	}
	var exists int
	err := tx.QueryRow(ctx, `SELECT 1 FROM auth_roles WHERE name = $1 LIMIT 1`, roleName).Scan(&exists)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		if isProtectedAdminRoleName(roleName) {
			return fmt.Errorf("default role %q is not available", roleName)
		}
		return upsertAccessRole(ctx, tx, binding, storedAccessRole{name: roleName, sourcePath: sourcePath}, commitSHA)
	case err != nil:
		return err
	default:
		return nil
	}
}

func (a *App) syncAccessGrants(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan accessSyncPlan, commitSHA string, details map[string]int) (map[resolvedAccessGrantKey]struct{}, error) {
	keep := map[resolvedAccessGrantKey]struct{}{}
	for _, grant := range plan.grants {
		if grant.role != productRoleOwner {
			continue
		}
		key, err := a.upsertManagedProductRoleGrant(ctx, tx, binding, grant, commitSHA)
		if err != nil {
			return nil, err
		}
		keep[key] = struct{}{}
		details["access_grants_synced"]++
	}
	for _, grant := range plan.grants {
		if grant.role == productRoleOwner || grant.role == customUseGrantRole {
			continue
		}
		key, err := a.upsertManagedProductRoleGrant(ctx, tx, binding, grant, commitSHA)
		if err != nil {
			return nil, err
		}
		keep[key] = struct{}{}
		details["access_grants_synced"]++
	}
	for _, grant := range plan.grants {
		if grant.role != customUseGrantRole {
			continue
		}
		key, err := a.upsertManagedResourceUseGrant(ctx, tx, binding, grant, commitSHA)
		if err != nil {
			return nil, err
		}
		keep[key] = struct{}{}
		details["access_grants_synced"]++
	}
	return keep, nil
}

func syncResourceVisibilities(ctx context.Context, tx pgx.Tx, plan accessSyncPlan, details map[string]int) error {
	for _, access := range plan.resourceAccess {
		if !access.visibilitySet {
			continue
		}
		resource, err := resolveAccessGrantResource(ctx, tx, access.resourceType, access.resourceID, true)
		if err != nil {
			return fmt.Errorf("failed to resolve resource access target %s:%s: %w", access.resourceType, access.resourceID, err)
		}
		if err := validateResourceVisibilityPolicy(resource.Type, access.visibility); err != nil {
			return err
		}
		if err := setResourceVisibilityWithRunner(ctx, tx, resource, access.visibility); err != nil {
			return fmt.Errorf("failed to sync resource visibility for %s:%s: %w", resource.Type, resource.ID, err)
		}
		details["resource_access_synced"]++
	}
	return nil
}

func (a *App) upsertManagedProductRoleGrant(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, grant storedAccessGrant, commitSHA string) (resolvedAccessGrantKey, error) {
	subject, err := resolveAccessGrantSubject(ctx, tx, grant.subjectType, grant.subjectID)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve grant subject %s:%s: %w", grant.subjectType, grant.subjectID, err)
	}
	roleName, err := normalizeProductRoleName(grant.role)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return resolvedAccessGrantKey{}, err
	} else if locked {
		return resolvedAccessGrantKey{}, fmt.Errorf("cannot modify default admin role assignments")
	}
	resource, err := resolveAccessGrantResource(ctx, tx, grant.resourceType, grant.resourceID, true)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve grant resource %s:%s: %w", grant.resourceType, grant.resourceID, err)
	}
	if err := validateGrantShape(roleName, resource, grant.inherit); err != nil {
		return resolvedAccessGrantKey{}, err
	}

	resourceScope := resource.ID
	if resource.Type == grantResourceSecret || resource.Type == grantResourceVariable {
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		resourceScope = firstNonEmptyString(repoName, scope)
	}
	writable, err := ensureAccessGrantConfigWritable(ctx, tx, binding, resourceScope, subject.Type, subject.ID, resource.Type, resource.ID)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}
	if !writable {
		return resolvedAccessGrantKey{
			subjectType:  subject.Type,
			subjectID:    subject.ID,
			resourceType: resource.Type,
			resourceID:   resource.ID,
		}, nil
	}

	var existingID int64
	var previousRole string
	err = tx.QueryRow(ctx, `
		SELECT id, role_name
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
	`, subject.Type, subject.ID, resource.Type, resource.ID).Scan(&existingID, &previousRole)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return resolvedAccessGrantKey{}, err
	}

	grantedBy := "config-repo"
	if existingID == 0 {
		if err := tx.QueryRow(ctx, `
			INSERT INTO access_grants (
				subject_type, subject_id, subject_display, role_name,
				resource_type, resource_id, resource_display, inherit, granted_by,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, TRUE)
			RETURNING id
		`, subject.Type, subject.ID, subject.Display, roleName, resource.Type, resource.ID, resource.Display, grant.inherit, grantedBy, binding.ID, grant.sourcePath, commitSHA).Scan(&existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to insert access grant: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM resource_acl WHERE access_grant_id = $1`, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM resource_ownership WHERE access_grant_id = $1`, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if previousRole == productRoleAdmin {
			if _, err := tx.Exec(ctx, `
				DELETE FROM auth_role_bindings
				WHERE role_name = $1
				  AND subject_type = $2
				  AND subject_id = $3
				  AND (managed_by_config_repo = FALSE OR config_repo_id = $4)
			`, productRoleAdmin, subject.Type, subject.ID, binding.ID); err != nil {
				return resolvedAccessGrantKey{}, err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE access_grants
			SET subject_display = $1,
				role_name = $2,
				resource_display = $3,
				inherit = $4,
				granted_by = $5,
				config_repo_id = $6,
				config_source_path = $7,
				config_source_commit_sha = $8,
				managed_by_config_repo = TRUE
			WHERE id = $9
		`, subject.Display, roleName, resource.Display, grant.inherit, grantedBy, binding.ID, grant.sourcePath, commitSHA, existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to update access grant: %w", err)
		}
	}

	if roleName == productRoleAdmin {
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_role_bindings (
				role_name, subject_type, subject_id,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE)
			ON CONFLICT (role_name, subject_type, subject_id) DO UPDATE SET
				config_repo_id = EXCLUDED.config_repo_id,
				config_source_path = EXCLUDED.config_source_path,
				config_source_commit_sha = EXCLUDED.config_source_commit_sha,
				managed_by_config_repo = TRUE
		`, productRoleAdmin, subject.Type, subject.ID, binding.ID, grant.sourcePath, commitSHA); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		return resolvedAccessGrantKey{subjectType: subject.Type, subjectID: subject.ID, resourceType: resource.Type, resourceID: resource.ID}, nil
	}

	for _, action := range applicableProductRoleActions(roleName, resource.Type) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO resource_acl (
				resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'allow')
			ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
			DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
		`, resource.Type, resource.ID, subject.Type, subject.ID, existingID, action); err != nil {
			return resolvedAccessGrantKey{}, err
		}
	}
	if roleName == productRoleOwner {
		if _, err := tx.Exec(ctx, `
			INSERT INTO resource_ownership (
				resource_type, resource_id, owner_subject_type, owner_subject_id, access_grant_id
			)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (resource_type, resource_id, owner_subject_type, owner_subject_id)
			DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
		`, resource.Type, resource.ID, subject.Type, subject.ID, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
	}
	return resolvedAccessGrantKey{subjectType: subject.Type, subjectID: subject.ID, resourceType: resource.Type, resourceID: resource.ID}, nil
}

func (a *App) upsertManagedResourceUseGrant(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, grant storedAccessGrant, commitSHA string) (resolvedAccessGrantKey, error) {
	subject, err := resolveResourceUseGrantSubject(ctx, tx, grant.subjectType, grant.subjectID)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve resource access subject %s:%s: %w", grant.subjectType, grant.subjectID, err)
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return resolvedAccessGrantKey{}, err
	} else if locked {
		return resolvedAccessGrantKey{}, fmt.Errorf("cannot modify default admin role assignments")
	}
	resource, err := resolveAccessGrantResource(ctx, tx, grant.resourceType, grant.resourceID, true)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve resource access target %s:%s: %w", grant.resourceType, grant.resourceID, err)
	}
	actions, err := normalizeUseGrantActions(resource.Type, grant.actions)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}

	resourceScope := resource.ID
	if resource.Type == grantResourceSecret || resource.Type == grantResourceVariable {
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		resourceScope = firstNonEmptyString(repoName, scope)
	}
	writable, err := ensureAccessGrantConfigWritable(ctx, tx, binding, resourceScope, subject.Type, subject.ID, resource.Type, resource.ID)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}
	resolvedKey := resolvedAccessGrantKey{
		subjectType:  subject.Type,
		subjectID:    subject.ID,
		resourceType: resource.Type,
		resourceID:   resource.ID,
	}
	if !writable {
		return resolvedKey, nil
	}

	var existingID int64
	var previousRole string
	err = tx.QueryRow(ctx, `
		SELECT id, role_name
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
	`, subject.Type, subject.ID, resource.Type, resource.ID).Scan(&existingID, &previousRole)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return resolvedAccessGrantKey{}, err
	}

	grantedBy := "config-repo"
	if existingID == 0 {
		if err := tx.QueryRow(ctx, `
			INSERT INTO access_grants (
				subject_type, subject_id, subject_display, role_name,
				resource_type, resource_id, resource_display, inherit, granted_by,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, $8, $9, $10, $11, TRUE)
			RETURNING id
		`, subject.Type, subject.ID, subject.Display, customUseGrantRole, resource.Type, resource.ID, resource.Display, grantedBy, binding.ID, grant.sourcePath, commitSHA).Scan(&existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to insert resource access grant: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM resource_acl WHERE access_grant_id = $1`, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM resource_ownership WHERE access_grant_id = $1`, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if previousRole == productRoleAdmin {
			if _, err := tx.Exec(ctx, `
				DELETE FROM auth_role_bindings
				WHERE role_name = $1
				  AND subject_type = $2
				  AND subject_id = $3
				  AND (managed_by_config_repo = FALSE OR config_repo_id = $4)
			`, productRoleAdmin, subject.Type, subject.ID, binding.ID); err != nil {
				return resolvedAccessGrantKey{}, err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE access_grants
			SET subject_display = $1,
				role_name = $2,
				resource_display = $3,
				inherit = FALSE,
				granted_by = $4,
				config_repo_id = $5,
				config_source_path = $6,
				config_source_commit_sha = $7,
				managed_by_config_repo = TRUE
			WHERE id = $8
		`, subject.Display, customUseGrantRole, resource.Display, grantedBy, binding.ID, grant.sourcePath, commitSHA, existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to update resource access grant: %w", err)
		}
	}

	if subject.Type != grantSubjectGroup {
		for _, action := range actions {
			if _, err := tx.Exec(ctx, `
				INSERT INTO resource_acl (
					resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
				)
				VALUES ($1, $2, $3, $4, $5, $6, 'allow')
				ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
				DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
			`, resource.Type, resource.ID, subject.Type, subject.ID, existingID, action); err != nil {
				return resolvedAccessGrantKey{}, err
			}
		}
	}

	return resolvedKey, nil
}

func ensureAccessGrantConfigWritable(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, resourceScope, subjectType, subjectID, resourceType, resourceID string) (bool, error) {
	displayID := subjectType + ":" + subjectID + " " + resourceType + ":" + resourceID
	var existingRepoID sql.NullInt64
	var managed bool
	err := tx.QueryRow(ctx, `
		SELECT config_repo_id, managed_by_config_repo
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
		LIMIT 1
	`, subjectType, subjectID, resourceType, resourceID).Scan(&existingRepoID, &managed)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return true, nil
	case err != nil:
		return false, err
	}
	if !managed {
		return true, nil
	}
	if !existingRepoID.Valid {
		return false, fmt.Errorf("access grant %s is already managed by an unknown config repository", displayID)
	}
	if existingRepoID.Int64 == binding.ID {
		return true, nil
	}

	existing, err := loadConfigRepositoryByID(ctx, tx, existingRepoID.Int64)
	if err != nil {
		return false, err
	}
	if canConfigRepositoryWriteOver(binding, existing, resourceScope) {
		return true, nil
	}
	if configRepositoryShadowsCurrent(existing, binding, resourceScope) {
		return false, nil
	}

	return false, fmt.Errorf("access grant %s is already managed by config repository %d", displayID, existingRepoID.Int64)
}

func pruneManagedAccessConfiguration(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan accessSyncPlan, grantKeys map[resolvedAccessGrantKey]struct{}) error {
	if err := pruneManagedAccessGrants(ctx, tx, binding, grantKeys); err != nil {
		return err
	}
	if err := pruneManagedUsers(ctx, tx, binding.ID, plan.users); err != nil {
		return err
	}
	if err := pruneManagedRoles(ctx, tx, binding.ID, plan); err != nil {
		return err
	}
	return nil
}

func clearResourceAccessOverridesForConfigSync(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository) error {
	rows, err := tx.Query(ctx, `
		SELECT resource_type, resource_id
		FROM resource_access_overrides
	`)
	if err != nil {
		return fmt.Errorf("failed to load resource access overrides: %w", err)
	}
	defer rows.Close()

	type overrideKey struct {
		resourceType string
		resourceID   string
	}
	var keys []overrideKey
	for rows.Next() {
		var key overrideKey
		if err := rows.Scan(&key.resourceType, &key.resourceID); err != nil {
			return err
		}
		if accessGrantResourceInConfigBindingScope(key.resourceType, key.resourceID, binding) {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, key := range keys {
		if _, err := tx.Exec(ctx, `
			DELETE FROM resource_access_overrides
			WHERE resource_type = $1 AND resource_id = $2
		`, key.resourceType, key.resourceID); err != nil {
			return fmt.Errorf("failed to clear resource access override for %s:%s: %w", key.resourceType, key.resourceID, err)
		}
	}
	return nil
}

func pruneManagedAccessGrants(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, keep map[resolvedAccessGrantKey]struct{}) error {
	rows, err := tx.Query(ctx, `
		SELECT id, subject_type, subject_id, role_name, resource_type, resource_id, resource_display, managed_by_config_repo
		FROM access_grants
		WHERE (managed_by_config_repo = TRUE AND config_repo_id = $1)
		   OR managed_by_config_repo = FALSE
	`, binding.ID)
	if err != nil {
		return fmt.Errorf("failed to load managed access grants for pruning: %w", err)
	}
	defer rows.Close()

	type grantRow struct {
		id              int64
		subjectType     string
		subjectID       string
		roleName        string
		resourceType    string
		resourceID      string
		resourceDisplay string
		managed         bool
	}
	var prune []grantRow
	for rows.Next() {
		var row grantRow
		if err := rows.Scan(&row.id, &row.subjectType, &row.subjectID, &row.roleName, &row.resourceType, &row.resourceID, &row.resourceDisplay, &row.managed); err != nil {
			return err
		}
		if !row.managed && !accessGrantResourceInConfigBindingScope(row.resourceType, row.resourceID, binding) {
			continue
		}
		key := resolvedAccessGrantKey{
			subjectType:  row.subjectType,
			subjectID:    row.subjectID,
			resourceType: row.resourceType,
			resourceID:   row.resourceID,
		}
		if _, ok := keep[key]; !ok {
			prune = append(prune, row)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range prune {
		if row.roleName == productRoleAdmin {
			if _, err := tx.Exec(ctx, `
				DELETE FROM auth_role_bindings
				WHERE role_name = $1
				  AND subject_type = $2
				  AND subject_id = $3
				  AND (managed_by_config_repo = FALSE OR config_repo_id = $4)
			`, productRoleAdmin, row.subjectType, row.subjectID, binding.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE id = $1`, row.id); err != nil {
			return fmt.Errorf("failed to prune access grant: %w", err)
		}
	}
	return nil
}

func accessGrantResourceInConfigBindingScope(resourceType, resourceID string, binding models.ConfigRepository) bool {
	switch binding.ScopeType {
	case models.ConfigRepositoryScopeSystem:
		return true
	case models.ConfigRepositoryScopeFolder:
		return accessGrantResourceUnderBindingScope(resourceType, resourceID, binding.ScopeID)
	default:
		return false
	}
}

func pruneStaleAccessGrantsForManagedUsers(ctx context.Context, tx pgx.Tx, users map[string]storedAccessUser) error {
	for _, user := range users {
		labels := []string{strings.TrimSpace(user.sub), strings.TrimSpace(user.email)}
		filtered := labels[:0]
		for _, label := range labels {
			if label != "" {
				filtered = append(filtered, label)
			}
		}
		if len(filtered) == 0 {
			continue
		}

		var currentID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE sub = $1 LIMIT 1`, user.sub).Scan(&currentID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id
			FROM access_grants
			WHERE subject_type = 'user'
			  AND subject_id != $1
			  AND subject_display = ANY($2)
		`, currentID, filtered)
		if err != nil {
			return err
		}
		var grantIDs []int64
		for rows.Next() {
			var grantID int64
			if err := rows.Scan(&grantID); err != nil {
				rows.Close()
				return err
			}
			grantIDs = append(grantIDs, grantID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		for _, grantID := range grantIDs {
			if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE id = $1`, grantID); err != nil {
				return err
			}
		}
	}
	return nil
}

func pruneManagedUsers(ctx context.Context, tx pgx.Tx, configRepoID int64, users map[string]storedAccessUser) error {
	query := `
		SELECT id::text
		FROM users
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
	`
	args := []any{configRepoID}
	if len(users) > 0 {
		subs := make([]string, 0, len(users))
		for sub := range users {
			subs = append(subs, sub)
		}
		query += ` AND sub != ALL($2)`
		args = append(args, subs)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	var prunedUserIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return err
		}
		prunedUserIDs = append(prunedUserIDs, userID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, userID := range prunedUserIDs {
		if err := deleteUserAccessArtifacts(ctx, tx, userID); err != nil {
			return err
		}
	}

	if len(users) == 0 {
		_, err := tx.Exec(ctx, `DELETE FROM users WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`, configRepoID)
		return err
	}
	subs := make([]string, 0, len(users))
	for sub := range users {
		subs = append(subs, sub)
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM users
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND sub != ALL($2)
	`, configRepoID, subs)
	return err
}

func pruneManagedRoles(ctx context.Context, tx pgx.Tx, configRepoID int64, plan accessSyncPlan) error {
	keep := map[string]struct{}{}
	for name := range plan.roles {
		keep[name] = struct{}{}
	}
	for _, policy := range plan.policies {
		keep[policy.role] = struct{}{}
	}
	for _, roleBinding := range plan.roleBindings {
		keep[roleBinding.role] = struct{}{}
	}

	if len(keep) == 0 {
		_, err := tx.Exec(ctx, `
			DELETE FROM auth_roles
			WHERE managed_by_config_repo = TRUE
			  AND config_repo_id = $1
		`, configRepoID)
		return err
	}
	names := make([]string, 0, len(keep))
	for name := range keep {
		names = append(names, name)
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM auth_roles
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND name != ALL($2)
	`, configRepoID, names)
	return err
}

func ensureGlobalConfigObjectWritable(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, tableName, resourceKind, resourceID, whereClause string, args ...any) error {
	query := fmt.Sprintf("SELECT config_repo_id, managed_by_config_repo FROM %s WHERE %s", tableName, whereClause)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var existingRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&existingRepoID, &managed); err != nil {
			return err
		}
		if !managed {
			continue
		}
		if !existingRepoID.Valid {
			return fmt.Errorf("%s %s is already managed by an unknown config repository", resourceKind, resourceID)
		}
		if existingRepoID.Int64 != binding.ID {
			return fmt.Errorf("%s %s is already managed by config repository %d", resourceKind, resourceID, existingRepoID.Int64)
		}
	}
	return rows.Err()
}
