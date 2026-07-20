package app

import "nopsai/pkg/proto"

func actionMayMutateWorkspace(action *proto.Action) bool {
	if action == nil {
		return false
	}
	return action.GetCommandAction() != nil || action.GetFileAction() != nil
}
