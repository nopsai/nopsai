package nopsai

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
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
	Users                []accessUserFile           `yaml:"users" json:"users"`
	ServiceAccounts      []accessServiceAccountFile `yaml:"service_accounts" json:"service_accounts"`
	Groups               *yaml.Node                 `yaml:"groups" json:"groups"`
	AuthGroups           *yaml.Node                 `yaml:"auth_groups" json:"auth_groups"`
	Roles                *yaml.Node                 `yaml:"roles" json:"roles"`
	AdvancedRoles        []accessRoleFile           `yaml:"advanced_roles" json:"advanced_roles"`
	Policies             []accessPolicyFile         `yaml:"policies" json:"policies"`
	RoleBindings         *yaml.Node                 `yaml:"role_bindings" json:"role_bindings"`
	Bindings             *yaml.Node                 `yaml:"bindings" json:"bindings"`
	AdvancedRoleBindings []accessRoleBindingFile    `yaml:"advanced_role_bindings" json:"advanced_role_bindings"`
	Grants               *yaml.Node                 `yaml:"grants" json:"grants"`
	AccessGrants         *yaml.Node                 `yaml:"access_grants" json:"access_grants"`
	BasicRoles           []accessGrantFile          `yaml:"basic_roles" json:"basic_roles"`
}

func (d accessConfigDocument) effectivePayload() accessConfigPayload {
	payload := d.accessConfigPayload
	if d.Access == nil {
		return payload
	}
	payload.Users = append(payload.Users, d.Access.Users...)
	payload.ServiceAccounts = append(payload.ServiceAccounts, d.Access.ServiceAccounts...)
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

type accessServiceAccountFile struct {
	ID            string     `yaml:"id" json:"id"`
	Sub           string     `yaml:"sub" json:"sub"`
	Email         string     `yaml:"email" json:"email"`
	Status        string     `yaml:"status" json:"status"`
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
	Type           string `yaml:"type" json:"type"`
	ID             string `yaml:"id" json:"id"`
	User           string `yaml:"user" json:"user"`
	Group          string `yaml:"group" json:"group"`
	Service        string `yaml:"service" json:"service"`
	ServiceAccount string `yaml:"service_account" json:"service_account"`
}

type accessRoleBindingFile struct {
	Role           string `yaml:"role" json:"role"`
	SubjectType    string `yaml:"subject_type" json:"subject_type"`
	SubjectID      string `yaml:"subject_id" json:"subject_id"`
	User           string `yaml:"user" json:"user"`
	Group          string `yaml:"group" json:"group"`
	Service        string `yaml:"service" json:"service"`
	ServiceAccount string `yaml:"service_account" json:"service_account"`
}

type accessGrantFile struct {
	SubjectType    string `yaml:"subject_type" json:"subject_type"`
	SubjectID      string `yaml:"subject_id" json:"subject_id"`
	User           string `yaml:"user" json:"user"`
	Group          string `yaml:"group" json:"group"`
	Service        string `yaml:"service" json:"service"`
	ServiceAccount string `yaml:"service_account" json:"service_account"`
	Role           string `yaml:"role" json:"role"`
	Resource       string `yaml:"resource" json:"resource"`
	ResourceType   string `yaml:"resource_type" json:"resource_type"`
	ResourceID     string `yaml:"resource_id" json:"resource_id"`
	Inherit        *bool  `yaml:"inherit" json:"inherit"`
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
	users           map[string]storedAccessUser
	serviceAccounts map[string]storedAccessServiceAccount
	roles           map[string]storedAccessRole
	policies        map[accessRolePolicyKey]storedAccessPolicy
	roleBindings    map[accessRoleBindingKey]storedAccessRoleBinding
	grants          map[accessGrantPlanKey]storedAccessGrant
	resourceAccess  map[resourceAccessPlanKey]storedResourceAccess
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

type storedAccessServiceAccount struct {
	id         string
	sub        string
	email      string
	status     string
	sourcePath string
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
