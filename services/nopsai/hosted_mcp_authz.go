package nopsai

import (
	"context"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

type hostedMCPPermission struct {
	Action   string
	Resource model.ResourceRef
}

func (p hostedMCPPermission) valid() bool {
	return strings.TrimSpace(p.Action) != "" &&
		strings.TrimSpace(p.Resource.Type) != "" &&
		strings.TrimSpace(p.Resource.ID) != ""
}

func (a *App) hostedMCPAllowed(ctx context.Context, subject model.Subject, permission hostedMCPPermission) bool {
	if !permission.valid() {
		return false
	}
	return a.checkCapabilityOrScopedGrant(ctx, subject, permission.Action, permission.Resource)
}

func hostedMCPToolPermission(tool hostedMCPTool) hostedMCPPermission {
	return hostedMCPPermission{
		Action:   tool.Action,
		Resource: tool.Resource,
	}
}

func hostedMCPReadPermission(action, resourceType, resourceID string) hostedMCPPermission {
	return hostedMCPPermission{
		Action: strings.TrimSpace(action),
		Resource: model.ResourceRef{
			Type: strings.TrimSpace(resourceType),
			ID:   strings.TrimSpace(resourceID),
		},
	}
}
