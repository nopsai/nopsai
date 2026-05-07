package main

import "net/http"

func (a *App) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/login", a.handleAuthLogin)
	mux.HandleFunc("POST /v1/auth/refresh", a.handleAuthRefresh)
	mux.HandleFunc("POST /v1/auth/logout", a.handleAuthLogout)
	mux.HandleFunc("POST /v1/auth/password", a.handleAuthChangePassword)
	mux.HandleFunc("POST /v1/auth/email", a.handleAuthUpdateEmail)
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
	mux.HandleFunc("GET /v1/system/config/sync", a.handleGetConfigSyncStatus)
	mux.HandleFunc("POST /v1/system/config/sync", a.handleConfigSync)
	mux.HandleFunc("POST /v1/internal/config/sync", a.handleConfigSync)
	mux.HandleFunc("GET /v1/system/dispatcher", a.handleDispatcherStatus)
	mux.HandleFunc("POST /v1/system/dispatcher/runners/{runnerID}/dispatch", a.handleUpdateRunnerDispatch)
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
	a.registerGitHubRoutes(mux)
	a.registerGroupRoutes(mux)
	a.registerSystemRoutes(mux)
	a.registerPipelineRoutes(mux)
	a.registerSecretVariableRoutes(mux)
	a.registerRunRoutes(mux)

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
