package nopsai

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

func newAccessSyncPlan() accessSyncPlan {
	return accessSyncPlan{
		users:           map[string]storedAccessUser{},
		serviceAccounts: map[string]storedAccessServiceAccount{},
		roles:           map[string]storedAccessRole{},
		policies:        map[accessRolePolicyKey]storedAccessPolicy{},
		roleBindings:    map[accessRoleBindingKey]storedAccessRoleBinding{},
		grants:          map[accessGrantPlanKey]storedAccessGrant{},
		resourceAccess:  map[resourceAccessPlanKey]storedResourceAccess{},
		teamSubjects:    map[string]struct{}{},
	}
}

func parseAccessSyncPlan(files map[string]string, accessDir string, binding models.ConfigRepository, boundTeam string) (accessSyncPlan, error) {
	plan := newAccessSyncPlan()
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, accessDir)
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
		if err := plan.addPayload(doc.effectivePayload(), binding, boundTeam, normalized); err != nil {
			return plan, fmt.Errorf("invalid access manifest '%s': %w", normalized, err)
		}
	}
	if err := validateAccessPlanForBinding(plan, binding); err != nil {
		return plan, err
	}
	return plan, nil
}

func (p accessSyncPlan) addPayload(payload accessConfigPayload, binding models.ConfigRepository, boundTeam, sourcePath string) error {
	if payload.Teams != nil || payload.AuthTeams != nil {
		return fmt.Errorf("access manifests do not support auth teams; grant users or services to team resources instead")
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

	for _, raw := range payload.ServiceAccounts {
		if raw.Role != nil || raw.Roles != nil {
			return fmt.Errorf("service account %q uses ambiguous roles; use advanced_roles for advanced role assignments", strings.TrimSpace(raw.Sub))
		}
		serviceAccount, err := normalizeAccessServiceAccount(raw, sourcePath)
		if err != nil {
			return err
		}
		if _, exists := p.serviceAccounts[serviceAccount.sub]; exists {
			return fmt.Errorf("duplicate service account %q", serviceAccount.sub)
		}
		if _, exists := p.users[serviceAccount.sub]; exists {
			return fmt.Errorf("service account %q conflicts with a managed user", serviceAccount.sub)
		}
		p.serviceAccounts[serviceAccount.sub] = serviceAccount
		roles := raw.AdvancedRoles.values()
		if role := strings.TrimSpace(raw.AdvancedRole); role != "" {
			roles = append(roles, role)
		}
		for _, role := range roles {
			binding := storedAccessRoleBinding{
				role:        strings.TrimSpace(role),
				subjectType: model.SubjectTypeServiceAccount,
				subjectID:   serviceAccount.sub,
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
		grant, err := normalizeAccessGrant(raw, binding, boundTeam, sourcePath)
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

func (p accessSyncPlan) addEmbeddedResourceAccess(content, sourcePath, resourceType, resourceID string, binding models.ConfigRepository, boundTeam string) error {
	resourceAccess, grants, ok, err := parseEmbeddedResourceAccess(content, sourcePath, resourceType, resourceID, binding, boundTeam)
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
	if isSSOManagedUserIdentifier(sub) {
		return storedAccessUser{}, fmt.Errorf("SSO-managed user %q cannot be managed by GitOps", sub)
	}
	email, err := normalizeOptionalEmail(raw.Email)
	if err != nil {
		return storedAccessUser{}, err
	}
	provider := strings.TrimSpace(raw.Provider)
	if provider == "" {
		provider = "local"
	}
	if isSSOManagedUserIdentifier(provider) {
		return storedAccessUser{}, fmt.Errorf("SSO-managed user %q cannot be managed by GitOps", sub)
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

func isSSOManagedUserIdentifier(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "oidc:")
}

func normalizeAccessServiceAccount(raw accessServiceAccountFile, sourcePath string) (storedAccessServiceAccount, error) {
	sub, err := validateServiceAccountSub(raw.Sub)
	if err != nil {
		return storedAccessServiceAccount{}, fmt.Errorf("service account %q has invalid sub: %w", strings.TrimSpace(raw.Sub), err)
	}
	email, err := normalizeOptionalEmail(raw.Email)
	if err != nil {
		return storedAccessServiceAccount{}, err
	}
	status := strings.ToLower(strings.TrimSpace(raw.Status))
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "disabled":
	default:
		return storedAccessServiceAccount{}, fmt.Errorf("service account %q has invalid status %q", sub, raw.Status)
	}
	if raw.ID != "" {
		if _, err := uuid.Parse(strings.TrimSpace(raw.ID)); err != nil {
			return storedAccessServiceAccount{}, fmt.Errorf("service account %q has invalid id", sub)
		}
	}
	return storedAccessServiceAccount{
		id:         strings.TrimSpace(raw.ID),
		sub:        sub,
		email:      email,
		status:     status,
		sourcePath: sourcePath,
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
	subjectType, subjectID, err := normalizeAccessSubject(raw.SubjectType, raw.SubjectID, raw.User, raw.Team, raw.Service, raw.ServiceAccount)
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

func normalizeAccessSubject(subjectType, subjectID, userID, teamID, serviceID, serviceAccountID string) (string, string, error) {
	subjectType = strings.TrimSpace(subjectType)
	subjectID = strings.TrimSpace(subjectID)
	switch {
	case strings.TrimSpace(userID) != "":
		subjectType = model.SubjectTypeUser
		subjectID = strings.TrimSpace(userID)
	case strings.TrimSpace(teamID) != "":
		subjectType = model.SubjectTypeAuthTeam
		subjectID = strings.TrimSpace(teamID)
	case strings.TrimSpace(serviceAccountID) != "":
		subjectType = model.SubjectTypeServiceAccount
		subjectID = strings.TrimSpace(serviceAccountID)
	case strings.TrimSpace(serviceID) != "":
		subjectType = model.SubjectTypeInternalService
		subjectID = strings.TrimSpace(serviceID)
	}
	normalizedType, err := normalizeAccessGrantSubjectType(subjectType)
	if err != nil {
		return "", "", err
	}
	if subjectID == "" {
		return "", "", fmt.Errorf("subject_id is required")
	}
	if err := rejectSSOManagedGitOpsSubject(normalizedType, subjectID); err != nil {
		return "", "", err
	}
	return normalizedType, subjectID, nil
}

func normalizeAccessGrant(raw accessGrantFile, binding models.ConfigRepository, boundTeam, sourcePath string) (storedAccessGrant, error) {
	subjectType, subjectID, err := normalizeAccessSubject(raw.SubjectType, raw.SubjectID, raw.User, raw.Team, raw.Service, raw.ServiceAccount)
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
	resourceID, err = normalizeAccessGrantResourceIDForBinding(resourceType, resourceID, binding, boundTeam)
	if err != nil {
		return storedAccessGrant{}, err
	}

	inherit := resourceType == grantResourceTeam
	if raw.Inherit != nil {
		inherit = *raw.Inherit
	}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		if roleName == productRoleAdmin || resourceType == grantResourcePlatform {
			return storedAccessGrant{}, fmt.Errorf("team-scoped config repositories cannot grant platform admin access")
		}
		if !accessGrantResourceUnderBindingScope(resourceType, resourceID, boundTeam) {
			return storedAccessGrant{}, fmt.Errorf("access grant target %s:%s is outside team scope %q", resourceType, resourceID, boundTeam)
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

func parseEmbeddedResourceAccess(content, sourcePath, resourceType, resourceID string, binding models.ConfigRepository, boundTeam string) (storedResourceAccess, []storedAccessGrant, bool, error) {
	if err := validateEmbeddedResourceAccessKeys(content); err != nil {
		return storedResourceAccess{}, nil, true, fmt.Errorf("invalid resource access '%s': %w", sourcePath, err)
	}

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
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			if !accessGrantResourceUnderBindingScope(grant.resourceType, grant.resourceID, boundTeam) {
				return storedResourceAccess{}, nil, true, fmt.Errorf("resource access target %s:%s is outside team scope %q", grant.resourceType, grant.resourceID, boundTeam)
			}
		}
		grants = append(grants, grant)
	}

	return resourceAccess, grants, true, nil
}

var embeddedResourceAccessKeys = map[string]struct{}{
	"visibility":   {},
	"use_access":   {},
	"grants":       {},
	"teams":        {},
	"repositories": {},
}

var embeddedResourceUseAccessKeys = map[string]struct{}{
	"mode":         {},
	"grants":       {},
	"teams":        {},
	"repositories": {},
}

var embeddedResourceUseGrantKeys = map[string]struct{}{
	"subject_type":    {},
	"subject_id":      {},
	"team":            {},
	"repository":      {},
	"repo":            {},
	"user":            {},
	"trigger":         {},
	"service":         {},
	"service_account": {},
	"actions":         {},
	"conditions":      {},
}

func validateEmbeddedResourceAccessKeys(content string) error {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return nil
	}
	access := yamlMappingValue(root.Content[0], "access")
	if access == nil {
		return nil
	}
	return validateEmbeddedResourceAccessNode(access)
}

func validateEmbeddedResourceAccessNode(node *yaml.Node) error {
	if err := validateYAMLMappingKeys(node, "resource access", embeddedResourceAccessKeys); err != nil {
		return err
	}
	if useAccess := yamlMappingValue(node, "use_access"); useAccess != nil {
		if err := validateYAMLMappingKeys(useAccess, "resource access use_access", embeddedResourceUseAccessKeys); err != nil {
			return err
		}
		if grants := yamlMappingValue(useAccess, "grants"); grants != nil {
			if err := validateEmbeddedResourceGrantNodes(grants); err != nil {
				return err
			}
		}
	}
	if grants := yamlMappingValue(node, "grants"); grants != nil {
		if err := validateEmbeddedResourceGrantNodes(grants); err != nil {
			return err
		}
	}
	return nil
}

func validateEmbeddedResourceGrantNodes(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range node.Content {
		if err := validateYAMLMappingKeys(item, "resource access grant", embeddedResourceUseGrantKeys); err != nil {
			return err
		}
	}
	return nil
}

func validateYAMLMappingKeys(node *yaml.Node, label string, allowed map[string]struct{}) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode == nil || keyNode.Kind != yaml.ScalarNode {
			continue
		}
		key := strings.TrimSpace(keyNode.Value)
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unsupported %s key %q", label, key)
		}
	}
	return nil
}

func yamlMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode != nil && keyNode.Kind == yaml.ScalarNode && keyNode.Value == key {
			return node.Content[i+1]
		}
	}
	return nil
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
	for _, team := range access.Teams.values() {
		grants = append(grants, embeddedResourceUseGrantFile{Team: team})
	}
	for _, repo := range access.Repositories.values() {
		grants = append(grants, embeddedResourceUseGrantFile{Repository: repo})
	}
	if access.UseAccess != nil {
		grants = append(grants, access.UseAccess.Grants...)
		for _, team := range access.UseAccess.Teams.values() {
			grants = append(grants, embeddedResourceUseGrantFile{Team: team})
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
		{grantSubjectTeam, raw.Team},
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
	case grantSubjectTeam:
		return grantSubjectTeam, subjectID, nil
	default:
		normalizedType, err := normalizeAccessGrantSubjectType(subjectType)
		if err != nil {
			return "", "", err
		}
		if err := rejectSSOManagedGitOpsSubject(normalizedType, subjectID); err != nil {
			return "", "", err
		}
		return normalizedType, subjectID, nil
	}
}

func rejectSSOManagedGitOpsSubject(subjectType, subjectID string) error {
	if subjectType == model.SubjectTypeUser && isSSOManagedUserIdentifier(subjectID) {
		return fmt.Errorf("SSO-managed user %q cannot be managed by GitOps", subjectID)
	}
	return nil
}

func normalizeAccessGrantResourceIDForBinding(resourceType, resourceID string, binding models.ConfigRepository, boundTeam string) (string, error) {
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	if resourceType == grantResourcePlatform {
		return platformGrantID, nil
	}
	if resourceType == grantResourceKnowledgeContext {
		kind, team, name, err := normalizeKnowledgeContextAccessResourceIDForBinding(resourceID, binding, boundTeam)
		if err != nil {
			return "", err
		}
		return buildKnowledgeContextIdentifier(kind, team, name), nil
	}
	if resourceType == grantResourceKnowledgeConnection {
		team, name, err := normalizeKnowledgeConnectionAccessResourceIDForBinding(resourceID, binding, boundTeam)
		if err != nil {
			return "", err
		}
		return buildKnowledgeConnectionIdentifier(team, name), nil
	}
	if resourceType == grantResourceSecret || resourceType == grantResourceVariable {
		if !strings.Contains(resourceID, "=") {
			resourceID = model.BuildNamedResourceID("", "", resourceID)
		}
		resourceID = runtimeNamedResourceIDForResource(resourceID)
		if binding.ScopeType != models.ConfigRepositoryScopeTeam {
			return resourceID, nil
		}
		repoName, scope, name := model.ParseNamedResourceID(resourceID)
		if name == "" {
			return "", fmt.Errorf("%s grant resource_id must include a name", resourceType)
		}
		var err error
		if repoName != "" {
			repoName, err = configsync.NormalizePathForTeam(boundTeam, repoName)
			if err != nil {
				return "", err
			}
		}
		if scope != "" {
			scope, err = configsync.NormalizePathForTeam(boundTeam, scope)
			if err != nil {
				return "", err
			}
		}
		return model.BuildNamedResourceID(repoName, scope, name), nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeTeam {
		return resourceID, nil
	}
	if isGlobalGrantResourceID(resourceID) {
		return globalGrantID, nil
	}
	return configsync.NormalizePathForTeam(boundTeam, resourceID)
}

func normalizeKnowledgeContextAccessResourceIDForBinding(resourceID string, binding models.ConfigRepository, boundTeam string) (string, string, string, error) {
	resourceID = strings.Trim(strings.TrimSpace(strings.ReplaceAll(resourceID, "\\", "/")), "/")
	parts := strings.Split(resourceID, "/")
	if binding.ScopeType == models.ConfigRepositoryScopeTeam && len(parts) == 2 {
		kind, err := normalizeKnowledgeContextKind(parts[0])
		if err != nil {
			return "", "", "", err
		}
		name, err := normalizeKnowledgeContextName(parts[1])
		if err != nil {
			return "", "", "", err
		}
		team, err := configsync.NormalizePathForTeam(boundTeam, "")
		if err != nil {
			return "", "", "", err
		}
		return kind, team, name, nil
	}
	kind, team, name, err := splitKnowledgeContextIdentifier(resourceID)
	if err != nil {
		return "", "", "", err
	}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		team, err = configsync.NormalizePathForTeam(boundTeam, team)
		if err != nil {
			return "", "", "", err
		}
	}
	return kind, team, name, nil
}

func normalizeKnowledgeConnectionAccessResourceIDForBinding(resourceID string, binding models.ConfigRepository, boundTeam string) (string, string, error) {
	resourceID = strings.Trim(strings.TrimSpace(strings.ReplaceAll(resourceID, "\\", "/")), "/")
	parts := strings.Split(resourceID, "/")
	if binding.ScopeType == models.ConfigRepositoryScopeTeam && len(parts) == 1 {
		name, err := normalizeKnowledgeConnectionName(parts[0])
		if err != nil {
			return "", "", err
		}
		team, err := configsync.NormalizePathForTeam(boundTeam, "")
		if err != nil {
			return "", "", err
		}
		return team, name, nil
	}
	team, name, err := splitKnowledgeConnectionIdentifier(resourceID)
	if err != nil {
		return "", "", err
	}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		team, err = configsync.NormalizePathForTeam(boundTeam, team)
		if err != nil {
			return "", "", err
		}
	}
	return team, name, nil
}

func validateAccessPlanForBinding(plan accessSyncPlan, binding models.ConfigRepository) error {
	if binding.ScopeType != models.ConfigRepositoryScopeTeam {
		return nil
	}
	if len(plan.users) > 0 {
		return fmt.Errorf("team-scoped config repositories cannot manage users")
	}
	if len(plan.serviceAccounts) > 0 {
		return fmt.Errorf("team-scoped config repositories cannot manage service accounts")
	}
	if len(plan.roles) > 0 || len(plan.policies) > 0 || len(plan.roleBindings) > 0 {
		return fmt.Errorf("team-scoped config repositories cannot manage global roles, policies, or role bindings")
	}
	return nil
}

func accessGrantResourceUnderBindingScope(resourceType, resourceID, boundTeam string) bool {
	boundTeam = strings.Trim(strings.TrimSpace(boundTeam), "/")
	if boundTeam == "" {
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
			if !configsync.ResourceUnderScope(repoName, boundTeam) {
				return false
			}
		}
		if scope != "" {
			checked = true
			if !configsync.ResourceUnderScope(scope, boundTeam) {
				return false
			}
		}
		return checked
	case grantResourceKnowledgeContext:
		_, team, _, err := splitKnowledgeContextIdentifier(resourceID)
		return err == nil && configsync.ResourceUnderScope(team, boundTeam)
	case grantResourceKnowledgeConnection:
		team, _, err := splitKnowledgeConnectionIdentifier(resourceID)
		return err == nil && configsync.ResourceUnderScope(team, boundTeam)
	case grantResourcePlatform:
		return false
	default:
		return configsync.ResourceUnderScope(resourceID, boundTeam)
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
			if repoName != "" && configsync.ResourceUnderScope(repoName, scope) {
				return true
			}
			if secretScope != "" && configsync.ResourceUnderScope(secretScope, scope) {
				return true
			}
		case grantResourceKnowledgeContext:
			_, team, _, err := splitKnowledgeContextIdentifier(resourceID)
			if err == nil && configsync.ResourceUnderScope(team, scope) {
				return true
			}
		case grantResourceKnowledgeConnection:
			team, _, err := splitKnowledgeConnectionIdentifier(resourceID)
			if err == nil && configsync.ResourceUnderScope(team, scope) {
				return true
			}
		default:
			if configsync.ResourceUnderScope(resourceID, scope) {
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
		if (binding.ScopeType == models.ConfigRepositoryScopeTeam || grant.role == customUseGrantRole) &&
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
