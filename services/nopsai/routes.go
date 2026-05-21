package main

import "net/http"

func (a *App) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/login", a.handleAuthLogin)
	mux.HandleFunc("POST /v1/auth/refresh", a.handleAuthRefresh)
	mux.HandleFunc("POST /v1/auth/logout", a.handleAuthLogout)
	mux.HandleFunc("POST /v1/auth/password", a.handleAuthChangePassword)
	mux.HandleFunc("POST /v1/auth/email", a.handleAuthUpdateEmail)
	mux.HandleFunc("GET /v1/auth/personal-tokens", a.handleListPersonalAccessTokens)
	mux.HandleFunc("POST /v1/auth/personal-tokens", a.handleCreatePersonalAccessToken)
	mux.HandleFunc("DELETE /v1/auth/personal-tokens/{tokenID}", a.handleRevokePersonalAccessToken)
	mux.HandleFunc("GET /v1/auth/me", a.handleAuthMe)
	mux.HandleFunc("GET /v1/audit", a.handleListAuditLogs)
	mux.HandleFunc("GET /v1/admin/users", a.handleListUsers)
	mux.HandleFunc("POST /v1/admin/users", a.handleCreateUser)
	mux.HandleFunc("PUT /v1/admin/users/{userID}", a.handleUpdateUser)
	mux.HandleFunc("PATCH /v1/admin/users/{userID}", a.handleUpdateUser)
	mux.HandleFunc("DELETE /v1/admin/users/{userID}", a.handleDeleteUser)
	mux.HandleFunc("POST /v1/admin/user-roles", a.handleAddUserRole)
	mux.HandleFunc("DELETE /v1/admin/user-roles", a.handleDeleteUserRole)
	mux.HandleFunc("GET /v1/admin/roles", a.handleListRoles)
	mux.HandleFunc("POST /v1/admin/roles", a.handleCreateRole)
	mux.HandleFunc("DELETE /v1/admin/roles", a.handleDeleteRole)
}

func (a *App) registerAccessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/access/grants", a.handleCreateAccessGrant)
	mux.HandleFunc("GET /v1/access/grants", a.handleListAccessGrants)
	mux.HandleFunc("DELETE /v1/access/grants/{grantID}", a.handleDeleteAccessGrant)
	mux.HandleFunc("GET /v1/access/groups", a.handleListAccessGroups)
	mux.HandleFunc("GET /v1/access/auth-groups", a.handleListAccessGroups)
	mux.HandleFunc("GET /v1/access/effective-permissions", a.handleGetEffectivePermissions)
	mux.HandleFunc("POST /v1/authz/resource-use/check", a.handleResourceUseCheck)
	mux.HandleFunc("POST /v1/authz/resource-use/batch-check", a.handleResourceUseBatchCheck)
	mux.HandleFunc("/v1/resources/", a.handleResourceAccessRoute)
}

func (a *App) registerGitHubRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/git/events", a.handleGitEvent)
}

func (a *App) registerGroupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/groups", a.handleCreateGroup)
	mux.HandleFunc("GET /v1/groups", a.handleGetGroups)
	mux.HandleFunc("PUT /v1/groups/{groupID}", a.handleUpdateGroup)
	mux.HandleFunc("DELETE /v1/groups/{groupID}", a.handleDeleteGroup)
	mux.HandleFunc("PUT /v1/groups/{groupID}/move", a.handleMoveGroup)
}

func (a *App) registerSystemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/system/config", a.handleGetSystemConfig)
	mux.HandleFunc("PUT /v1/system/config", a.handleUpdateSystemConfig)
	mux.HandleFunc("GET /v1/system/llm-profiles", a.handleListLLMProfiles)
	mux.HandleFunc("PUT /v1/system/llm-profiles", a.handleReplaceLLMProfiles)
	mux.HandleFunc("PUT /v1/system/llm-profiles/{profileName}", a.handleUpsertLLMProfile)
	mux.HandleFunc("DELETE /v1/system/llm-profiles/{profileName}", a.handleDeleteLLMProfile)
	mux.HandleFunc("POST /v1/system/llm-profiles/{profileName}/test", a.handleTestLLMProfile)
	mux.HandleFunc("GET /v1/system/mcp/servers", a.handleListMCPServers)
	mux.HandleFunc("POST /v1/system/mcp/servers", a.handleCreateMCPServer)
	mux.HandleFunc("GET /v1/system/mcp/servers/{serverName}", a.handleGetMCPServer)
	mux.HandleFunc("PUT /v1/system/mcp/servers/{serverName}", a.handleUpsertMCPServer)
	mux.HandleFunc("DELETE /v1/system/mcp/servers/{serverName}", a.handleDeleteMCPServer)
	mux.HandleFunc("POST /v1/system/mcp/servers/{serverName}/test", a.handleTestMCPServer)
	mux.HandleFunc("POST /v1/system/mcp/servers/{serverName}/discover-tools", a.handleDiscoverMCPServerTools)
	mux.HandleFunc("GET /v1/system/mcp/profiles", a.handleListMCPProfiles)
	mux.HandleFunc("POST /v1/system/mcp/profiles", a.handleCreateMCPProfile)
	mux.HandleFunc("GET /v1/system/mcp/profiles/{profileName}", a.handleGetMCPProfile)
	mux.HandleFunc("PUT /v1/system/mcp/profiles/{profileName}", a.handleUpsertMCPProfile)
	mux.HandleFunc("DELETE /v1/system/mcp/profiles/{profileName}", a.handleDeleteMCPProfile)
	mux.HandleFunc("POST /v1/system/mcp/profiles/{profileName}/test", a.handleTestMCPProfile)
	mux.HandleFunc("GET /v1/system/config/sync", a.handleGetConfigSyncStatus)
	mux.HandleFunc("POST /v1/system/config/sync", a.handleConfigSync)
	mux.HandleFunc("GET /v1/system/config-repo", a.handleGetGlobalConfigRepository)
	mux.HandleFunc("PUT /v1/system/config-repo", a.handleUpsertGlobalConfigRepository)
	mux.HandleFunc("DELETE /v1/system/config-repo", a.handleDeleteGlobalConfigRepository)
	mux.HandleFunc("GET /v1/system/config-repo/sync", a.handleGetGlobalConfigRepositorySyncStatus)
	mux.HandleFunc("POST /v1/system/config-repo/sync", a.handleSyncGlobalConfigRepository)
	mux.HandleFunc("GET /v1/system/config-repos", a.handleListConfigRepositories)
	mux.HandleFunc("POST /v1/system/config-repos/sync", a.handleSyncAllConfigRepositories)
	mux.HandleFunc("POST /v1/internal/config/sync", a.handleConfigSync)
	mux.HandleFunc("GET /v1/system/dispatcher", a.handleDispatcherStatus)
	mux.HandleFunc("GET /v1/system/dispatcher/runner-compose", a.handleGenerateRunnerCompose)
	mux.HandleFunc("GET /v1/system/dispatcher/runner-bootstrap-command", a.handleGenerateRunnerBootstrapCommand)
	mux.HandleFunc("GET /v1/system/dispatcher/runner-bootstrap", a.handleRunnerBootstrap)
	mux.HandleFunc("POST /v1/system/dispatcher/runners/{runnerID}/dispatch", a.handleUpdateRunnerDispatch)
}

func (a *App) registerSetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/setup/preflight", a.handleSetupPreflight)
	mux.HandleFunc("GET /v1/setup/status", a.handleGetSetupStatus)
	mux.HandleFunc("GET /v1/setup/templates", a.handleGetSetupTemplates)
	mux.HandleFunc("GET /v1/setup/templates.zip", a.handleDownloadSetupTemplates)
	mux.HandleFunc("POST /v1/setup/bootstrap", a.handleBootstrapSetup)
}

func (a *App) registerFolderConfigRepositoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/groups/{folderID}/config-repo", a.handleGetFolderConfigRepository)
	mux.HandleFunc("PUT /v1/groups/{folderID}/config-repo", a.handleUpsertFolderConfigRepository)
	mux.HandleFunc("DELETE /v1/groups/{folderID}/config-repo", a.handleDeleteFolderConfigRepository)
	mux.HandleFunc("GET /v1/groups/{folderID}/config-repo/sync", a.handleGetFolderConfigRepositorySyncStatus)
	mux.HandleFunc("POST /v1/groups/{folderID}/config-repo/sync", a.handleSyncFolderConfigRepository)
}

func (a *App) registerPipelineRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/pipelines", a.handleListPipelines)
	mux.HandleFunc("GET /v1/pipelines/{pipelineName...}", a.handleGetPipeline)
	mux.HandleFunc("PUT /v1/pipelines/{pipelineName...}", a.handleCreateOrUpdatePipeline)
	mux.HandleFunc("DELETE /v1/pipelines/{pipelineName...}", a.handleDeletePipeline)
	mux.HandleFunc("GET /v1/steps", a.handleListReusableSteps)
	mux.HandleFunc("GET /v1/steps/{stepPath...}", a.handleGetStepRoute)
	mux.HandleFunc("PUT /v1/steps/{stepName...}", a.handleCreateOrUpdateReusableStep)
	mux.HandleFunc("DELETE /v1/steps/{stepName...}", a.handleDeleteReusableStep)
	mux.HandleFunc("GET /v1/overrides", a.handleListTriggerOverrides)
	mux.HandleFunc("GET /v1/overrides/{repoOwner}/{repoName}", a.handleGetTriggerOverride)
	mux.HandleFunc("PUT /v1/overrides/{repoOwner}/{repoName}", a.handleCreateOrUpdateTriggerOverride)
	mux.HandleFunc("DELETE /v1/overrides/{repoOwner}/{repoName}", a.handleDeleteTriggerOverride)
}

func (a *App) registerKnowledgeContextRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/knowledge-contexts", a.handleListKnowledgeContexts)
	mux.HandleFunc("GET /v1/knowledge-contexts/{knowledgeID...}", a.handleGetKnowledgeContext)
	mux.HandleFunc("PUT /v1/knowledge-contexts/{knowledgeID...}", a.handleUpsertKnowledgeContext)
	mux.HandleFunc("DELETE /v1/knowledge-contexts/{knowledgeID...}", a.handleDeleteKnowledgeContext)
}

func (a *App) registerSecretVariableRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/secrets", a.handleListGeneralSecrets)
	mux.HandleFunc("GET /v1/secrets/scopes", a.handleListSecretScopes)
	mux.HandleFunc("GET /v1/secrets/{secretName}", a.handleGetGeneralSecretValue)
	mux.HandleFunc("PUT /v1/secrets/{secretName}", a.handleCreateOrUpdateGeneralSecret)
	mux.HandleFunc("DELETE /v1/secrets/{secretName}", a.handleDeleteGeneralSecret)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/secrets", a.handleListRepoSecrets)
	mux.HandleFunc("PUT /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}", a.handleCreateOrUpdateRepoSecret)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}", a.handleDeleteRepoSecret)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/branches", a.handleListRepoBranches)
	mux.HandleFunc("GET /v1/variables", a.handleListGeneralVariables)
	mux.HandleFunc("GET /v1/variables/scopes", a.handleListVariableScopes)
	mux.HandleFunc("GET /v1/variables/{variableName}", a.handleGetGeneralVariableValue)
	mux.HandleFunc("PUT /v1/variables/{variableName}", a.handleCreateOrUpdateGeneralVariable)
	mux.HandleFunc("DELETE /v1/variables/{variableName}", a.handleDeleteGeneralVariable)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/variables", a.handleListRepoVariables)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}", a.handleGetRepoVariableValue)
	mux.HandleFunc("PUT /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}", a.handleCreateOrUpdateRepoVariable)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}", a.handleDeleteRepoVariable)
}

func (a *App) registerRunRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/runs/{runID}/status", a.handleGetRunStatus)
	mux.HandleFunc("POST /v1/run", a.handleRunPipeline)
	mux.HandleFunc("POST /v1/run/{pipelineName...}", a.handleRunPipeline)
	mux.HandleFunc("GET /v1/runs", a.handleListRuns)
	mux.HandleFunc("GET /v1/runs/{runID}", a.handleGetRunDetails)
	mux.HandleFunc("DELETE /v1/runs/{runID}", a.handleDeleteRun)
	mux.HandleFunc("GET /v1/runs-by-check/{checkRunID}", a.handleGetRunByCheckID)
	mux.HandleFunc("POST /v1/runs/{runID}/rerun", a.handleRerunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/cancel", a.handleCancelRun)
	mux.HandleFunc("POST /v1/runs/{runID}/finalize", a.handleFinalizeRun)
	mux.HandleFunc("POST /v1/runs/{runID}/steps/{stepName}/tasks/{taskName}", a.handleTaskUpdate)
	mux.HandleFunc("POST /v1/runs/{runID}/logs/ingest", a.handleIngestLogs)
	mux.HandleFunc("GET /v1/runs/{runID}/logs", a.handleGetRunLogs)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/branches/{branch...}", a.handleDeleteRepoBranchRuns)
}

func (a *App) buildHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	a.registerAuthRoutes(mux)
	a.registerAccessRoutes(mux)
	a.registerGitHubRoutes(mux)
	a.registerGroupRoutes(mux)
	a.registerSystemRoutes(mux)
	a.registerFolderConfigRepositoryRoutes(mux)
	a.registerPipelineRoutes(mux)
	a.registerKnowledgeContextRoutes(mux)
	a.registerSecretVariableRoutes(mux)
	a.registerRunRoutes(mux)
	a.registerSetupRoutes(mux)

	var handler http.Handler = mux
	handler = a.authzMiddleware(handler)
	handler = a.authMiddleware(handler)
	handler = a.auditMiddleware(handler)
	handler = recoveryMiddleware(handler)
	handler = loggingMiddleware(handler)
	handler = requestIDMiddleware(handler)
	handler = corsMiddleware(handler)
	return handler
}
