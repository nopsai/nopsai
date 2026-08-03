package nopsai

import (
	"context"
	"database/sql"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

type configRepositoryAccessExportDocument struct {
	Users                []configRepositoryUserExport                `yaml:"users,omitempty"`
	ServiceAccounts      []configRepositoryServiceAccountExport      `yaml:"service_accounts,omitempty"`
	AdvancedRoles        []configRepositoryAdvancedRoleExport        `yaml:"advanced_roles,omitempty"`
	Policies             []configRepositoryAccessPolicyExport        `yaml:"policies,omitempty"`
	AdvancedRoleBindings []configRepositoryAdvancedRoleBindingExport `yaml:"advanced_role_bindings,omitempty"`
	BasicRoles           []configRepositoryBasicRoleExport           `yaml:"basic_roles,omitempty"`
}

type configRepositoryUserExport struct {
	Sub           string   `yaml:"sub"`
	Email         string   `yaml:"email,omitempty"`
	Provider      string   `yaml:"provider,omitempty"`
	Status        string   `yaml:"status"`
	AdvancedRoles []string `yaml:"advanced_roles,omitempty"`
}

type configRepositoryServiceAccountExport struct {
	Sub           string   `yaml:"sub"`
	Email         string   `yaml:"email,omitempty"`
	Status        string   `yaml:"status"`
	AdvancedRoles []string `yaml:"advanced_roles,omitempty"`
}

type configRepositoryBasicRoleExport struct {
	User           string `yaml:"user,omitempty"`
	ServiceAccount string `yaml:"service_account,omitempty"`
	SubjectType    string `yaml:"subject_type,omitempty"`
	SubjectID      string `yaml:"subject_id,omitempty"`
	Role           string `yaml:"role"`
	Resource       string `yaml:"resource"`
	Inherit        *bool  `yaml:"inherit,omitempty"`
}

type configRepositoryAdvancedRoleExport struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type configRepositoryAccessPolicyExport struct {
	Role     string `yaml:"role"`
	Name     string `yaml:"name,omitempty"`
	Resource string `yaml:"resource"`
	Action   string `yaml:"action"`
	Effect   string `yaml:"effect,omitempty"`
}

type configRepositoryAdvancedRoleBindingExport struct {
	Role        string `yaml:"role"`
	SubjectType string `yaml:"subject_type"`
	SubjectID   string `yaml:"subject_id"`
}

func (a *App) exportConfigRepositoryAccess(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		allDoc := configRepositoryAccessExportDocument{}
		users, err := a.configRepositoryUserExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.Users = users
		roles, err := a.configRepositoryAdvancedRoleExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.AdvancedRoles = roles
		policies, err := a.configRepositoryAccessPolicyExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.Policies = policies
		roleBindings, err := a.configRepositoryAdvancedRoleBindingExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.AdvancedRoleBindings = roleBindings
		grants, err := a.configRepositoryBasicRoleGrantExports(ctx, repo, delegatedScopes, func(subjectType string) bool {
			return subjectType != grantSubjectServiceAccount
		})
		if err != nil {
			return err
		}
		allDoc.BasicRoles = grants
		if !allDoc.empty() {
			content, err := marshalConfigRepositoryYAML(allDoc)
			if err != nil {
				return err
			}
			files[configRepositoryAccessAllPath] = string(content)
		}

		serviceDoc := configRepositoryAccessExportDocument{}
		serviceAccounts, err := a.configRepositoryServiceAccountExports(ctx, repo)
		if err != nil {
			return err
		}
		serviceDoc.ServiceAccounts = serviceAccounts
		serviceGrants, err := a.configRepositoryBasicRoleGrantExports(ctx, repo, delegatedScopes, func(subjectType string) bool {
			return subjectType == grantSubjectServiceAccount
		})
		if err != nil {
			return err
		}
		serviceDoc.BasicRoles = serviceGrants
		if !serviceDoc.empty() {
			content, err := marshalConfigRepositoryYAML(serviceDoc)
			if err != nil {
				return err
			}
			files[configRepositoryServiceAccountsAccessPath] = string(content)
		}
		return nil
	}

	grants, err := a.configRepositoryBasicRoleGrantExports(ctx, repo, delegatedScopes, func(subjectType string) bool {
		return true
	})
	if err != nil {
		return err
	}
	if len(grants) == 0 {
		return nil
	}
	doc := configRepositoryAccessExportDocument{BasicRoles: grants}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files[configRepositoryAccessGrantsPath] = string(content)
	return nil
}

func (doc configRepositoryAccessExportDocument) empty() bool {
	return len(doc.Users) == 0 &&
		len(doc.ServiceAccounts) == 0 &&
		len(doc.AdvancedRoles) == 0 &&
		len(doc.Policies) == 0 &&
		len(doc.AdvancedRoleBindings) == 0 &&
		len(doc.BasicRoles) == 0
}

func (a *App) configRepositoryUserExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryUserExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT u.sub, COALESCE(u.email, ''), u.provider, u.status
		FROM users u
		WHERE u.provider <> $1
		  AND LOWER(COALESCE(u.provider, '')) NOT LIKE 'oidc:%'
		  AND u.sub <> $2
		  AND (u.managed_by_config_repo = FALSE OR u.config_repo_id = $3)
		  AND NOT EXISTS (
			SELECT 1
			FROM auth_external_identities ei
			WHERE ei.user_id = u.id
		  )
		ORDER BY sub ASC
	`, auth.ProviderServiceAccount, defaultAdminSub, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []configRepositoryUserExport{}
	userIndex := map[string]int{}
	for rows.Next() {
		var user configRepositoryUserExport
		if err := rows.Scan(&user.Sub, &user.Email, &user.Provider, &user.Status); err != nil {
			return nil, err
		}
		user.Sub = strings.TrimSpace(user.Sub)
		user.Email = strings.TrimSpace(user.Email)
		user.Provider = strings.TrimSpace(user.Provider)
		user.Status = strings.TrimSpace(user.Status)
		if user.Sub == "" {
			continue
		}
		if user.Status == "" {
			user.Status = "active"
		}
		if user.Provider == "" || user.Provider == "local" {
			user.Provider = ""
		}
		userIndex[user.Sub] = len(users)
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	roleRows, err := a.db.Query(ctx, `
		SELECT subject_id, role_name
		FROM auth_role_bindings
		WHERE subject_type = $1
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		ORDER BY subject_id ASC, role_name ASC
	`, grantSubjectUser, repo.ID)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var subjectID, roleName string
		if err := roleRows.Scan(&subjectID, &roleName); err != nil {
			return nil, err
		}
		idx, ok := userIndex[strings.TrimSpace(subjectID)]
		if !ok {
			continue
		}
		roleName = strings.TrimSpace(roleName)
		if roleName == "" {
			continue
		}
		users[idx].AdvancedRoles = append(users[idx].AdvancedRoles, roleName)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	for idx := range users {
		users[idx].AdvancedRoles = uniqueSortedStrings(users[idx].AdvancedRoles)
	}
	return users, nil
}

func (a *App) configRepositoryServiceAccountExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryServiceAccountExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT sub, COALESCE(email, ''), status
		FROM users
		WHERE provider = $1
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		ORDER BY sub ASC
	`, auth.ProviderServiceAccount, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []configRepositoryServiceAccountExport{}
	accountIndex := map[string]int{}
	for rows.Next() {
		var account configRepositoryServiceAccountExport
		if err := rows.Scan(&account.Sub, &account.Email, &account.Status); err != nil {
			return nil, err
		}
		account.Sub = strings.TrimSpace(account.Sub)
		account.Email = strings.TrimSpace(account.Email)
		account.Status = strings.TrimSpace(account.Status)
		if account.Sub == "" {
			continue
		}
		if account.Status == "" {
			account.Status = "active"
		}
		accountIndex[account.Sub] = len(accounts)
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	roleRows, err := a.db.Query(ctx, `
		SELECT subject_id, role_name
		FROM auth_role_bindings arb
		WHERE subject_type = $1
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		  AND NOT EXISTS (
			SELECT 1
			FROM access_grants ag
			WHERE ag.subject_type = arb.subject_type
			  AND ag.subject_id = arb.subject_id
			  AND ag.role_name = arb.role_name
			  AND ag.role_name = $3
		  )
		ORDER BY subject_id ASC, role_name ASC
	`, grantSubjectServiceAccount, repo.ID, productRoleAdmin)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()

	for roleRows.Next() {
		var subjectID, roleName string
		if err := roleRows.Scan(&subjectID, &roleName); err != nil {
			return nil, err
		}
		idx, ok := accountIndex[strings.TrimSpace(subjectID)]
		if !ok {
			continue
		}
		roleName = strings.TrimSpace(roleName)
		if roleName == "" {
			continue
		}
		accounts[idx].AdvancedRoles = append(accounts[idx].AdvancedRoles, roleName)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	for idx := range accounts {
		accounts[idx].AdvancedRoles = uniqueSortedStrings(accounts[idx].AdvancedRoles)
	}
	return accounts, nil
}

func (a *App) configRepositoryAdvancedRoleExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryAdvancedRoleExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT name, COALESCE(description, '')
		FROM auth_roles
		WHERE managed_by_config_repo = FALSE OR config_repo_id = $1
		ORDER BY name ASC
	`, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := []configRepositoryAdvancedRoleExport{}
	for rows.Next() {
		var role configRepositoryAdvancedRoleExport
		if err := rows.Scan(&role.Name, &role.Description); err != nil {
			return nil, err
		}
		role.Name = strings.TrimSpace(role.Name)
		role.Description = strings.TrimSpace(role.Description)
		if role.Name == "" || isProtectedAdminRoleName(role.Name) {
			continue
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func (a *App) configRepositoryAccessPolicyExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryAccessPolicyExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT role_name, resource_type, resource_id, action, effect
		FROM auth_role_permissions
		WHERE managed_by_config_repo = FALSE OR config_repo_id = $1
		ORDER BY role_name ASC, resource_type ASC, resource_id ASC, action ASC, effect ASC
	`, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := []configRepositoryAccessPolicyExport{}
	for rows.Next() {
		var policy configRepositoryAccessPolicyExport
		var resourceType, resourceID, effect string
		if err := rows.Scan(&policy.Role, &resourceType, &resourceID, &policy.Action, &effect); err != nil {
			return nil, err
		}
		policy.Role = strings.TrimSpace(policy.Role)
		resourceType = strings.TrimSpace(resourceType)
		resourceID = strings.TrimSpace(resourceID)
		policy.Action = strings.TrimSpace(policy.Action)
		effect = strings.TrimSpace(effect)
		if policy.Role == "" || isProtectedAdminRoleName(policy.Role) || resourceType == "" || policy.Action == "" {
			continue
		}
		policy.Resource = resourceType + ":" + resourceID
		if effect != "" && effect != "allow" {
			policy.Effect = effect
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return policies, nil
}

func (a *App) configRepositoryAdvancedRoleBindingExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryAdvancedRoleBindingExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT role_name, subject_type, subject_id
		FROM auth_role_bindings
		WHERE subject_type = ANY($1)
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		ORDER BY role_name ASC, subject_type ASC, subject_id ASC
	`, []string{model.SubjectTypeRepository, model.SubjectTypeTrigger, model.SubjectTypeInternalService}, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bindings := []configRepositoryAdvancedRoleBindingExport{}
	for rows.Next() {
		var binding configRepositoryAdvancedRoleBindingExport
		if err := rows.Scan(&binding.Role, &binding.SubjectType, &binding.SubjectID); err != nil {
			return nil, err
		}
		binding.Role = strings.TrimSpace(binding.Role)
		binding.SubjectType = exportAccessSubjectType(binding.SubjectType)
		binding.SubjectID = strings.Trim(strings.TrimSpace(binding.SubjectID), "/")
		if binding.Role == "" || isProtectedAdminRoleName(binding.Role) || binding.SubjectType == "" || binding.SubjectID == "" {
			continue
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bindings, nil
}

func (a *App) configRepositoryBasicRoleGrantExports(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, includeSubject func(string) bool) ([]configRepositoryBasicRoleExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			ag.subject_type,
			CASE
				WHEN ag.subject_type = $2 THEN COALESCE(NULLIF(u.sub, ''), ag.subject_id)
				WHEN ag.subject_type = $4 THEN COALESCE(NULLIF(sa.sub, ''), ag.subject_id)
				ELSE ag.subject_id
			END AS subject_id,
			ag.role_name,
			ag.resource_type,
			ag.resource_id,
			ag.inherit,
			ag.config_repo_id,
			ag.managed_by_config_repo
		FROM access_grants ag
		LEFT JOIN users u
		  ON ag.subject_type = $2
		 AND (ag.subject_id = u.id::text OR ag.subject_id = u.sub)
		 AND u.provider <> $3
		 AND LOWER(COALESCE(u.provider, '')) NOT LIKE 'oidc:%'
		 AND NOT EXISTS (
			SELECT 1
			FROM auth_external_identities ei
			WHERE ei.user_id = u.id
		 )
		LEFT JOIN users sa
		  ON ag.subject_type = $4
		 AND (ag.subject_id = sa.sub OR ag.subject_id = sa.id::text)
		 AND sa.provider = $3
		WHERE ag.role_name <> $1
		  AND COALESCE(ag.managed_by_identity_provider, FALSE) = FALSE
		  AND NOT (
			ag.subject_type = $2
			AND LOWER(BTRIM(ag.subject_id)) LIKE 'oidc:%'
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM users sso_user
			WHERE ag.subject_type = $2
			  AND (ag.subject_id = sso_user.id::text OR ag.subject_id = sso_user.sub)
			  AND (
				LOWER(COALESCE(sso_user.provider, '')) LIKE 'oidc:%'
				OR EXISTS (
					SELECT 1
					FROM auth_external_identities ei
					WHERE ei.user_id = sso_user.id
				)
			  )
		  )
		ORDER BY ag.subject_type ASC, subject_id ASC, ag.resource_type ASC, ag.resource_id ASC, ag.role_name ASC
	`, customUseGrantRole, model.SubjectTypeUser, auth.ProviderServiceAccount, model.SubjectTypeServiceAccount)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grants := []configRepositoryBasicRoleExport{}
	for rows.Next() {
		var subjectType, subjectID, roleName, resourceType, resourceID string
		var inherit, managed bool
		var configRepoID sql.NullInt64
		if err := rows.Scan(&subjectType, &subjectID, &roleName, &resourceType, &resourceID, &inherit, &configRepoID, &managed); err != nil {
			return nil, err
		}
		subjectType = exportAccessSubjectType(subjectType)
		subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
		roleName = strings.TrimSpace(roleName)
		resourceType = strings.TrimSpace(resourceType)
		resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
		if subjectType == "" || subjectID == "" || roleName == "" || resourceType == "" {
			continue
		}
		if includeSubject != nil && !includeSubject(subjectType) {
			continue
		}
		if _, ok := productRoleDefinitions[roleName]; !ok {
			continue
		}
		if managed {
			if !configRepoID.Valid || configRepoID.Int64 != repo.ID {
				continue
			}
		}
		if !configRepositoryIncludesBasicRoleGrant(repo, resourceType, resourceID, delegatedScopes) {
			continue
		}
		grant := configRepositoryBasicRoleExport{
			Role:     roleName,
			Resource: configRepositoryBasicRoleResourceExport(resourceType, resourceID),
		}
		if !setConfigRepositoryBasicRoleSubjectExport(&grant, subjectType, subjectID) {
			continue
		}
		defaultInherit := resourceType == grantResourceTeam
		if inherit != defaultInherit {
			next := inherit
			grant.Inherit = &next
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

func setConfigRepositoryBasicRoleSubjectExport(grant *configRepositoryBasicRoleExport, subjectType, subjectID string) bool {
	if grant == nil {
		return false
	}
	subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
	if subjectID == "" {
		return false
	}
	switch strings.TrimSpace(subjectType) {
	case grantSubjectUser:
		grant.User = subjectID
	case grantSubjectServiceAccount:
		grant.ServiceAccount = subjectID
	default:
		grant.SubjectType = strings.TrimSpace(subjectType)
		grant.SubjectID = subjectID
	}
	return grant.User != "" || grant.ServiceAccount != "" || (grant.SubjectType != "" && grant.SubjectID != "")
}

func configRepositoryBasicRoleResourceExport(resourceType, resourceID string) string {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	if resourceType == grantResourceTeam && resourceID == globalGrantID {
		resourceID = globalGrantID
	}
	if resourceType == grantResourceTeam {
		resourceType = grantResourceTeam
	}
	return resourceType + ":" + resourceID
}

func exportAccessSubjectType(subjectType string) string {
	switch strings.TrimSpace(subjectType) {
	case model.SubjectTypeUser:
		return grantSubjectUser
	case model.SubjectTypeRepository:
		return grantSubjectRepository
	case model.SubjectTypeTrigger:
		return grantSubjectTrigger
	case model.SubjectTypeServiceAccount:
		return grantSubjectServiceAccount
	case model.SubjectTypeInternalService, grantSubjectService:
		return model.SubjectTypeInternalService
	default:
		return ""
	}
}
