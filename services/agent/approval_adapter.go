package agent

import "nopsai/services/agent/internal/approval"

func newAgentApprovalPauser() approval.Pauser {
	return approval.NewPauser(approval.Config{
		ArchiveWorkspace:    archiveWorkspace,
		RequestPause:        requestApprovalPause,
		CheckpointMaxBytes:  approvalCheckpointMaxBytesFromEnv,
		DefaultWorkspaceDir: agentWorkspaceDir,
	})
}
