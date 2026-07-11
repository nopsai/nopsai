package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/google/go-github/v53/github"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/gittrigger"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/pkg/auth"
)

func findStepByName(steps []models.PipelineStep, name string) (models.PipelineStep, bool) {
	for _, step := range steps {
		if step.GetName() == name {
			return step, true
		}
	}
	return models.PipelineStep{}, false
}

func (a *App) handleGitEvent(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil ||
		!strings.EqualFold(strings.TrimSpace(claims.Provider), serviceauth.ProviderInternalService) ||
		!containsFold(claims.Roles, serviceauth.RoleGitBot) {
		http.Error(w, "git-bot service identity required", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "Missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	payload, err := github.ParseWebHook(eventType, body)
	if err != nil {
		http.Error(w, "Could not parse webhook", http.StatusBadRequest)
		return
	}

	var (
		owner, repo, ref, commitSHA string
		repoFullName                string
		headCommit                  *github.HeadCommit
		pusher                      *github.User
		cloneURL                    string
		sshURL                      string
		branchName                  string
		targetRef                   string
		commitURL                   string
		commitMessage               string
		commitAuthorName            string
		commitAuthorEmail           string
		commitAuthorUsername        string
		pusherName                  string
		pusherEmail                 string
		beforeSHA                   string
		changedFiles                []string
		changedFilesKnown           bool
		isRerun                     bool
		rerunCheckRun               *github.CheckRun
	)
	deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	triggerEventID := deliveryID

	switch event := payload.(type) {
	case *github.PushEvent:
		if event.GetAfter() == "0000000000000000000000000000000000000000" {
			log.Info().Msg("Ignoring push event for branch deletion.")
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.After == nil {
			log.Warn().Msg("Received push event with missing essential repository or commit data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		commitSHA = event.GetAfter()
		ref = event.GetRef()
		if strings.HasPrefix(ref, "refs/heads/") {
			branchName = strings.TrimPrefix(ref, "refs/heads/")
		}
		headCommit = event.HeadCommit
		pusher = event.Pusher
		beforeSHA = event.GetBefore()
		changedFiles = githubPushChangedFiles(event)
		changedFilesKnown = len(event.Commits) > 0

		if event.Repo.CloneURL != nil {
			cloneURL = event.Repo.GetCloneURL()
		}
		if event.Repo.SSHURL != nil {
			sshURL = event.Repo.GetSSHURL()
		}
		if headCommit != nil {
			if headCommit.URL != nil {
				commitURL = headCommit.GetURL()
			}
			if headCommit.Message != nil {
				message := headCommit.GetMessage()
				if idx := strings.Index(message, "\n"); idx >= 0 {
					commitMessage = message[:idx]
				} else {
					commitMessage = message
				}
			}
			if headCommit.Author != nil {
				if headCommit.Author.Name != nil {
					commitAuthorName = headCommit.Author.GetName()
				}
				if headCommit.Author.Email != nil {
					commitAuthorEmail = headCommit.Author.GetEmail()
				}
				if headCommit.Author.Login != nil {
					commitAuthorUsername = headCommit.Author.GetLogin()
				}
			}
		}
		if pusher != nil {
			if pusher.Name != nil {
				pusherName = pusher.GetName()
			}
			if pusher.Email != nil {
				pusherEmail = pusher.GetEmail()
			}
		}

		log.Info().Str("repo", repoFullName).Str("commit", commitSHA).Msg("Processing push event")
	case *github.PullRequestEvent:
		if event.GetAction() == "closed" {
			log.Info().Msg("Ignoring pull_request event with 'closed' action to prevent duplicate runs on merge.")
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.PullRequest == nil || event.PullRequest.Head == nil || event.PullRequest.Head.SHA == nil {
			log.Warn().Msg("Received pull_request event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		eventType = "pull_request"
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		commitSHA = event.GetPullRequest().GetHead().GetSHA()
		ref = event.GetPullRequest().GetHead().GetRef()
		branchName = ref
		if strings.HasPrefix(branchName, "refs/heads/") {
			branchName = strings.TrimPrefix(branchName, "refs/heads/")
		} else if branchName != "" {
			ref = fmt.Sprintf("refs/heads/%s", branchName)
		}
		if prBase := event.GetPullRequest().GetBase(); prBase != nil {
			targetRef = prBase.GetRef()
			if targetRef != "" && !strings.HasPrefix(targetRef, "refs/") {
				targetRef = fmt.Sprintf("refs/heads/%s", targetRef)
			}
		}
		if prUser := event.GetPullRequest().GetUser(); prUser != nil {
			if name := prUser.GetName(); name != "" {
				pusherName = name
			} else {
				pusherName = prUser.GetLogin()
			}
			pusherEmail = prUser.GetEmail()
		}
		if pusherName == "" && commitAuthorName != "" {
			pusherName = commitAuthorName
		}
		log.Info().Str("repo", repoFullName).Str("commit", commitSHA).Msg("Processing pull_request event")
	case *github.CheckRunEvent:
		if event.GetAction() != "rerequested" {
			log.Info().Msgf("Received check_run event with action '%s', ignoring.", event.GetAction())
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.CheckRun == nil || event.CheckRun.HeadSHA == nil {
			log.Warn().Msg("Received rerequested check_run event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		commitSHA = event.GetCheckRun().GetHeadSHA()
		rerunCheckRun = event.GetCheckRun()
		isRerun = true
		if len(event.CheckRun.PullRequests) > 0 {
			eventType = "pull_request"
			pr := event.CheckRun.PullRequests[0]
			if pr != nil {
				if head := pr.GetHead(); head != nil {
					ref = head.GetRef()
				}
				if base := pr.GetBase(); base != nil {
					targetRef = base.GetRef()
					if targetRef != "" && !strings.HasPrefix(targetRef, "refs/") {
						targetRef = fmt.Sprintf("refs/heads/%s", targetRef)
					}
				}
			}
		} else {
			eventType = "push"
			if event.CheckRun.CheckSuite != nil && event.CheckRun.CheckSuite.HeadBranch != nil {
				ref = "refs/heads/" + event.CheckRun.CheckSuite.GetHeadBranch()
			} else {
				ref = commitSHA
			}
		}
	case *github.CheckSuiteEvent:
		if event.GetAction() != "rerequested" {
			log.Info().Msgf("Received check_suite event with action '%s', ignoring.", event.GetAction())
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.CheckSuite == nil || event.CheckSuite.HeadSHA == nil {
			log.Warn().Msg("Received rerequested check_suite event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		isRerun = true

		suiteInfo, err := a.findSuiteCheckRun(owner, repo, event.GetCheckSuite().GetID(), event.GetCheckSuite().GetHeadSHA())
		if err != nil {
			log.Error().Err(err).Msg("Failed to resolve check run for rerequested suite")
			http.Error(w, "Could not find check run for this suite.", http.StatusInternalServerError)
			return
		}
		rerunCheckRun = &github.CheckRun{
			ID:      github.Int64(suiteInfo.CheckRunID),
			HeadSHA: github.String(suiteInfo.HeadSHA),
		}
		commitSHA = suiteInfo.HeadSHA
		if suiteInfo.PullRequestHeadRef != "" {
			rerunCheckRun.PullRequests = []*github.PullRequest{{
				Head: &github.PullRequestBranch{Ref: github.String(suiteInfo.PullRequestHeadRef)},
			}}
		}

		if len(event.CheckSuite.PullRequests) > 0 {
			eventType = "pull_request"
			pr := event.CheckSuite.PullRequests[0]
			if pr != nil {
				if head := pr.GetHead(); head != nil {
					ref = head.GetRef()
				}
				if base := pr.GetBase(); base != nil {
					targetRef = base.GetRef()
					if targetRef != "" && !strings.HasPrefix(targetRef, "refs/") {
						targetRef = fmt.Sprintf("refs/heads/%s", targetRef)
					}
				}
			}
		} else if suiteInfo.PullRequestHeadRef != "" {
			eventType = "pull_request"
			ref = suiteInfo.PullRequestHeadRef
		} else {
			eventType = "push"
			if event.CheckSuite.HeadBranch != nil {
				ref = "refs/heads/" + event.CheckSuite.GetHeadBranch()
			} else if suiteInfo.HeadBranch != "" {
				ref = "refs/heads/" + suiteInfo.HeadBranch
			} else {
				ref = commitSHA
			}
		}
	default:
		log.Info().Msgf("Received unhandled event type '%s', ignoring.", eventType)
		w.WriteHeader(http.StatusOK)
		return
	}

	if eventType == "push" && branchName != "" {
		hasOpenPR, err := a.branchHasOpenPullRequest(owner, repo, branchName)
		if err != nil {
			log.Error().Err(err).Str("repo", repoFullName).Str("branch", branchName).Msg("Failed to check for open pull requests; proceeding with push event")
		} else if hasOpenPR {
			log.Info().Str("repo", repoFullName).Str("branch", branchName).Msg("Skipping push event because branch has open pull request")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Push event skipped: branch has open pull request."))
			return
		}
	}

	if owner == "" || repo == "" || commitSHA == "" {
		log.Warn().Msg("Skipping event due to missing owner, repo, or commit SHA")
		w.WriteHeader(http.StatusOK)
		return
	}
	if triggerEventID == "" {
		triggerEventID = deriveTriggerEventID(map[string]string{
			"repo_owner": owner,
			"repo_name":  repo,
			"ref":        ref,
			"commit_sha": commitSHA,
		})
	}
	if triggerEventID == "" {
		triggerEventID = uuid.NewString()
	}

	if eventType == "push" && !strings.HasPrefix(ref, "refs/") {
		var storedRef sql.NullString
		err := a.db.QueryRow(
			context.Background(),
			"SELECT git_ref FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND git_commit_sha = $3 ORDER BY created_at DESC LIMIT 1",
			owner, repo, commitSHA,
		).Scan(&storedRef)
		if err == nil && storedRef.Valid && strings.HasPrefix(storedRef.String, "refs/") {
			log.Info().Str("commit", commitSHA).Str("ref", storedRef.String).Msg("Recovered original ref for rerun event")
			ref = storedRef.String
		} else if err != nil && err != sql.ErrNoRows {
			log.Warn().Err(err).Str("commit", commitSHA).Msg("Failed to recover original ref for rerun event")
		}
	}

	if beforeSHA != "" && beforeSHA != "0000000000000000000000000000000000000000" {
		a.cancelStaleCheckRuns(owner, repo, beforeSHA)
	}

	manifest, pipelineSource, err := a.fetchTriggerManifest(owner, repo, commitSHA)
	if err != nil {
		if errors.Is(err, errManifestNotFound) {
			log.Info().Str("repo", repoFullName).Msg("No trigger manifest found; skipping event.")
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Error().Err(err).Str("repo", repoFullName).Msg("Failed to load trigger manifest")
		http.Error(w, "Failed to load trigger manifest", http.StatusInternalServerError)
		return
	}

	pipelines, baseScope := findPipelinesForGitEvent(manifest, gittrigger.Event{
		Type:              eventType,
		Ref:               ref,
		TargetRef:         targetRef,
		RepositoryName:    repo,
		ChangedFiles:      changedFiles,
		ChangedFilesKnown: changedFilesKnown,
	})
	if len(pipelines) == 0 {
		log.Info().Str("repo", repoFullName).Str("ref", ref).Msg("No pipelines matched event.")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("No pipeline found for this event."))
		return
	}

	anyTriggered := false
	buildFailedRunGitContext := func(checkRunID int64) map[string]string {
		gitContext := map[string]string{
			"repo_owner":             owner,
			"repo_name":              repo,
			"clone_url":              cloneURL,
			"ssh_url":                sshURL,
			"ref":                    ref,
			"target_ref":             targetRef,
			"commit_sha":             commitSHA,
			"commit_url":             commitURL,
			"commit_message":         commitMessage,
			"commit_author_name":     commitAuthorName,
			"commit_author_email":    commitAuthorEmail,
			"commit_author_username": commitAuthorUsername,
			"pusher_name":            pusherName,
			"pusher_email":           pusherEmail,
			"trigger_event_id":       triggerEventID,
		}
		if checkRunID != 0 {
			gitContext["check_run_id"] = strconv.FormatInt(checkRunID, 10)
		}
		return gitContext
	}
	for _, p := range pipelines {
		originalPath := p.Path
		effectiveScope := baseScope

		if strings.HasPrefix(p.Path, "http://") || strings.HasPrefix(p.Path, "https://") {
			errMsg := fmt.Sprintf("Remote pipeline URLs are not supported (entry: %s)", p.Path)
			fallbackDef := fmt.Sprintf("name: remote-url\nsteps: []\n# %s", p.Path)
			checkRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, []byte(fallbackDef), pipelineSource)
			if createErr == nil {
				a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, errMsg)
			}
			log.Warn().Str("repo", repoFullName).Msg(errMsg)
			continue
		}
		if p.Path == "" {
			log.Warn().Str("repo", repoFullName).Msg("Pipeline entry missing path; skipping.")
			continue
		}

		var pipelineYAML []byte
		pipelineSourceForCheck := pipelineSource // Start with the trigger's source
		var err error
		var checkRunIDStr string
		if isRerun && rerunCheckRun != nil {
			checkRunIDStr = strconv.FormatInt(rerunCheckRun.GetID(), 10)
		}

		dbPath, dbName, extPart, parseErr := configsync.SplitPipelineIdentifier(p.Path)
		if parseErr != nil {
			log.Warn().Err(parseErr).Str("pipeline", p.Path).Msg("Skipping pipeline due to invalid identifier")
			continue
		}

		callerID := repositoryFullName(owner, repo)
		normalizedPipelineID := configsync.BuildPipelineIdentifier(dbPath, dbName)
		authChecks := make([]ResourceUseAuthResult, 0, 2)
		repoSource := repositoryPipelineSourceForIdentifier(dbPath, dbName, extPart)
		preferRepositoryPipeline := !isDatabasePipelineSource(pipelineSource)

		if preferRepositoryPipeline {
			pipelineYAML, err = a.requestGitBotPipeline(owner, repo, commitSHA, repoSource)
			if err == nil {
				pipelineSourceForCheck = "repository"
				p.Path = originalPath
			} else if errors.Is(err, errPipelineNotFound) {
				err = nil
			}
		}

		if len(pipelineYAML) == 0 && err == nil {
			pipelineInDB, existsErr := a.pipelineExistsInDB(dbPath, dbName)
			if existsErr != nil {
				err = existsErr
			} else if pipelineInDB {
				pipelineSourceForCheck = "database override"
				authz, authErr := a.AuthorizeResourceUse(context.Background(), ResourceUseAuthInput{
					CallerType:   model.SubjectTypeRepository,
					CallerID:     callerID,
					Action:       "pipeline.use",
					ResourceType: grantResourcePipeline,
					ResourceID:   normalizedPipelineID,
					EventType:    eventType,
					Ref:          ref,
					Repo:         callerID,
				})
				if authErr != nil || !authz.Allowed {
					authz = normalizeResourceUseFailureResult(authz, authErr)
					summary := resourceUseFailureSummary(model.SubjectTypeRepository, callerID, authz, authErr)
					checkRunID := a.failGitHubAuthorizationCheck(owner, repo, commitSHA, checkRunIDStr, normalizedPipelineID, pipelineSourceForCheck, summary)
					placeholderDef := fmt.Sprintf("name: %q\nsteps: []\n", dbName)
					a.recordAuthorizationDeniedPipelineRun(
						normalizedPipelineID,
						"",
						[]byte(placeholderDef),
						buildFailedRunGitContext(checkRunID),
						effectiveScope,
						pipelineSourceForCheck,
						"github",
						model.SubjectTypeRepository,
						callerID,
						summary,
						[]ResourceUseAuthResult{authz},
					)
					log.Warn().
						Err(authErr).
						Str("repository", callerID).
						Str("pipeline", normalizedPipelineID).
						Str("auth_reason", authz.Reason).
						Str("caller_team", authz.CallerTeam).
						Str("resource_team", authz.ResourceTeam).
						Str("visibility", authz.Visibility).
						Msg("Repository is not authorized to use pipeline")
					continue
				}
				authChecks = append(authChecks, authz)
				pipelineYAML, err = a.fetchPipelineFromDB(dbPath, dbName)
				if err == nil {
					pipelineSourceForCheck = "database override"
					log.Info().Str("pipeline", p.Path).Msg("Using authorized pipeline definition from database.")
				}
			} else if preferRepositoryPipeline {
				err = errPipelineNotFound
			} else {
				if isDatabasePipelineSource(pipelineSource) {
					log.Warn().Str("pipeline", p.Path).Msg("Pipeline not found in database; falling back to repository definition.")
				}
				pipelineYAML, err = a.requestGitBotPipeline(owner, repo, commitSHA, repoSource)
				if err == nil {
					pipelineSourceForCheck = "repository"
					p.Path = originalPath
				}
			}
		}

		if err != nil {
			identifier := originalPath
			if errors.Is(err, errPipelineNotFound) {
				identifier = repoSource.Path
			}
			summary := ""
			switch {
			case errors.Is(err, errPipelineNotFound):
				summary = fmt.Sprintf("Error: Could not locate pipeline `%s`.", identifier)
			default:
				summary = fmt.Sprintf("Error: %v", err)
			}

			fallbackDef := fmt.Sprintf("name: %s\nsteps: []", identifier)
			var createErr error
			checkRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, []byte(fallbackDef), pipelineSourceForCheck)
			if createErr != nil {
				log.Error().Err(createErr).Str("pipeline", identifier).Msg("Failed to create check run after pipeline retrieval error")
				http.Error(w, "Failed to create check run", http.StatusInternalServerError)
				return
			}
			a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)

			if errors.Is(err, errPipelineNotFound) {
				gitContextForRun := buildFailedRunGitContext(checkRunID)
				placeholderDef := fallbackDef
				if !strings.HasSuffix(placeholderDef, "\n") {
					placeholderDef += "\n"
				}
				placeholderDef += fmt.Sprintf("# %s\n", summary)
				a.recordMissingPipelineRun(originalPath, "", []byte(placeholderDef), gitContextForRun, effectiveScope, pipelineSourceForCheck, summary)
			}
			continue
		}

		var pipeline models.Pipeline
		if err := yaml.Unmarshal(pipelineYAML, &pipeline); err != nil {
			log.Error().Err(err).Msg("Failed to parse pipeline YAML")
			summary := fmt.Sprintf("Error: Pipeline definition is invalid. %v", err)
			checkRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, pipelineYAML, pipelineSourceForCheck)
			if createErr != nil {
				http.Error(w, "Failed to create check run", http.StatusInternalServerError)
				return
			}
			a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)
			continue
		}

		if effectiveScope != "" {
			scopeAuthz, scopeAuthErr := a.AuthorizeResourceUse(context.Background(), ResourceUseAuthInput{
				CallerType:   model.SubjectTypeRepository,
				CallerID:     callerID,
				Action:       "scope.use",
				ResourceType: grantResourceScope,
				ResourceID:   effectiveScope,
				EventType:    eventType,
				Ref:          ref,
				Repo:         callerID,
			})
			if scopeAuthErr != nil || !scopeAuthz.Allowed {
				scopeAuthz = normalizeResourceUseFailureResult(scopeAuthz, scopeAuthErr)
				summary := resourceUseFailureSummary(model.SubjectTypeRepository, callerID, scopeAuthz, scopeAuthErr)
				checkRunID := a.failGitHubAuthorizationCheck(owner, repo, commitSHA, checkRunIDStr, normalizedPipelineID, pipelineSourceForCheck, summary)
				authChecks = append(authChecks, scopeAuthz)
				a.recordAuthorizationDeniedPipelineRun(
					normalizedPipelineID,
					pipeline.Version,
					pipelineYAML,
					buildFailedRunGitContext(checkRunID),
					effectiveScope,
					pipelineSourceForCheck,
					"github",
					model.SubjectTypeRepository,
					callerID,
					summary,
					authChecks,
				)
				log.Warn().
					Err(scopeAuthErr).
					Str("repository", callerID).
					Str("scope", effectiveScope).
					Str("auth_reason", scopeAuthz.Reason).
					Str("caller_team", scopeAuthz.CallerTeam).
					Str("resource_team", scopeAuthz.ResourceTeam).
					Str("visibility", scopeAuthz.Visibility).
					Msg("Repository is not authorized to use scope")
				continue
			}
		}

		headers := map[string]string{
			"X-Git-Repo-Owner":             owner,
			"X-Git-Repo-Name":              repo,
			"X-Git-Commit-SHA":             commitSHA,
			"X-Git-Check-Run-ID":           checkRunIDStr,
			"X-Git-Ref":                    ref,
			"X-Git-Target-Ref":             targetRef,
			"X-Nopsai-Scope":               effectiveScope,
			"X-Nopsai-Pipeline-Path":       dbPath,
			"X-Git-Clone-URL":              cloneURL,
			"X-Git-SSH-URL":                sshURL,
			"X-Git-Commit-URL":             commitURL,
			"X-Git-Commit-Message":         commitMessage,
			"X-Git-Commit-Author-Name":     commitAuthorName,
			"X-Git-Commit-Author-Email":    commitAuthorEmail,
			"X-Git-Commit-Author-Username": commitAuthorUsername,
			"X-Git-Pusher-Name":            pusherName,
			"X-Git-Pusher-Email":           pusherEmail,
			"X-Nopsai-Pipeline-Source":     pipelineSourceForCheck,
			"X-Nopsai-Trigger-Event-ID":    triggerEventID,
			"X-Nopsai-Caller-Type":         model.SubjectTypeRepository,
			"X-Nopsai-Caller-ID":           callerID,
			"X-Nopsai-Trigger-Source":      "github_" + eventType,
		}
		if isRerun {
			headers["X-Git-Rerun-Commit-SHA"] = commitSHA
		}

		var req *http.Request
		if pipelineSourceForCheck == "database override" {
			req = httptest.NewRequest(http.MethodPost, "/v1/run/"+originalPath, nil)
			req.SetPathValue("pipelineName", originalPath)
		} else {
			req = httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(pipelineYAML))
			req.Header.Set("Content-Type", "application/x-yaml")
		}
		for key, value := range headers {
			if value != "" {
				req.Header.Set(key, value)
			}
		}
		req = a.withDispatcherInternalSubject(req)

		recorder := httptest.NewRecorder()
		a.handleRunPipeline(recorder, req)
		result := recorder.Result()
		responseBody, _ := io.ReadAll(result.Body)
		result.Body.Close()

		if result.StatusCode != http.StatusCreated {
			responseText := strings.TrimSpace(string(responseBody))
			log.Warn().
				Str("repository", repoFullName).
				Str("pipeline", originalPath).
				Str("scope", effectiveScope).
				Int("status", result.StatusCode).
				Str("response", responseText).
				Msg("Failed to trigger Nopsai pipeline from Git event")
			summary := fmt.Sprintf("Failed to trigger Nopsai pipeline. The nopsai service responded with status %d.\n\nError: %s", result.StatusCode, responseText)
			var checkRunID int64
			if checkRunIDStr != "" {
				if parsedID, err := strconv.ParseInt(checkRunIDStr, 10, 64); err == nil {
					checkRunID = parsedID
					a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)
				}
			}
			if checkRunID == 0 {
				createdCheckRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, pipelineYAML, pipelineSourceForCheck)
				if createErr != nil {
					log.Error().Err(createErr).Str("pipeline", originalPath).Msg("Failed to create check run after run authorization error")
					http.Error(w, "Failed to create check run", http.StatusInternalServerError)
					return
				}
				checkRunID = createdCheckRunID
				a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)
			}
			if !a.gitPipelineRunExistsForFailure(context.Background(), triggerEventID, checkRunID, originalPath) {
				a.recordAuthorizationDeniedPipelineRun(
					originalPath,
					pipeline.Version,
					pipelineYAML,
					buildFailedRunGitContext(checkRunID),
					effectiveScope,
					pipelineSourceForCheck,
					"github_"+eventType,
					model.SubjectTypeRepository,
					callerID,
					summary,
					authChecks,
				)
			}
			continue
		}

		anyTriggered = true
	}

	if anyTriggered {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Pipelines triggered."))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("No pipelines triggered."))
	}
}

func (a *App) failGitHubAuthorizationCheck(owner, repo, commitSHA, checkRunIDStr, pipelineID, pipelineSource, summary string) int64 {
	checkRunID, _ := strconv.ParseInt(strings.TrimSpace(checkRunIDStr), 10, 64)
	if checkRunID == 0 {
		fallbackName := strings.Trim(strings.TrimSpace(pipelineID), "/")
		if fallbackName == "" {
			fallbackName = "authorization-denied"
		}
		fallbackDef := fmt.Sprintf("name: %s\nsteps: []\n", fallbackName)
		var err error
		checkRunID, err = a.createGitHubCheckRun(owner, repo, commitSHA, []byte(fallbackDef), pipelineSource)
		if err != nil {
			log.Error().Err(err).Str("pipeline", pipelineID).Msg("Failed to create check run after authorization denial")
			return 0
		}
	}
	a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)
	return checkRunID
}

func (a *App) gitPipelineRunExistsForFailure(ctx context.Context, triggerEventID string, checkRunID int64, identifier string) bool {
	if a == nil || a.db == nil {
		return false
	}
	if checkRunID != 0 {
		var exists bool
		if err := a.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pipeline_runs WHERE git_check_run_id = $1)`, checkRunID).Scan(&exists); err != nil {
			log.Warn().Err(err).Int64("check_run_id", checkRunID).Msg("Failed to check existing pipeline run for Git check")
			return false
		}
		if exists {
			return true
		}
	}

	triggerEventID = strings.TrimSpace(triggerEventID)
	if triggerEventID == "" {
		return false
	}
	pathPart, namePart, _, err := configsync.SplitPipelineIdentifier(identifier)
	if err != nil {
		return false
	}
	var exists bool
	if err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pipeline_runs
			WHERE trigger_event_id = $1
			  AND pipeline_name = $2
			  AND pipeline_path = $3
		)
	`, triggerEventID, sanitizeInput(namePart), pathPart).Scan(&exists); err != nil {
		log.Warn().Err(err).Str("trigger_event_id", triggerEventID).Str("pipeline", identifier).Msg("Failed to check existing pipeline run for Git trigger")
		return false
	}
	return exists
}

func (a *App) findEncryptedSecret(secretName, repoFullName, scope string) (string, string, bool, error) {
	var encryptedValue sql.NullString
	storageScope := runtimeScopeForStorage(scope)
	resourceScope := runtimeScopeForResource(storageScope)

	err := a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3, storageScope)+" LIMIT 1", secretName, repoFullName, storageScope).Scan(&encryptedValue)
	if err == nil {
		return encryptedValue.String, model.BuildNamedResourceID(repoFullName, resourceScope, secretName), true, nil
	}
	if err != pgx.ErrNoRows {
		return "", "", false, err
	}

	err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2, storageScope)+" LIMIT 1", secretName, storageScope).Scan(&encryptedValue)
	if err == nil {
		return encryptedValue.String, model.BuildNamedResourceID("", resourceScope, secretName), true, nil
	}
	if err == pgx.ErrNoRows {
		return "", "", false, nil
	}
	return "", "", false, err
}

func findPipelinesForEvent(manifest models.Manifest, eventType, ref, repoName string) ([]models.PipelineSource, string) {
	return findPipelinesForGitEvent(manifest, gittrigger.Event{
		Type:           eventType,
		Ref:            ref,
		RepositoryName: repoName,
	})
}

func findPipelinesForGitEvent(manifest models.Manifest, event gittrigger.Event) ([]models.PipelineSource, string) {
	match := gittrigger.Find(manifest, event)
	return match.Pipelines, match.Scope
}

func branchMatchesAnyPattern(branchName string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchBranchPattern(pattern, branchName) {
			return true
		}
	}
	return false
}

func (a *App) getTriggerOverride(ctx context.Context, fullName string) (string, error) {
	var triggerDef string
	err := a.db.QueryRow(ctx, "SELECT trigger_definition FROM triggers WHERE repository_name = $1", fullName).Scan(&triggerDef)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return triggerDef, nil
}

func isRepoSkipped(repoName string, skipList []string) bool {
	for _, pattern := range skipList {
		if matchBranchPattern(pattern, repoName) {
			return true
		}
	}
	return false
}

func githubPushChangedFiles(event *github.PushEvent) []string {
	if event == nil {
		return nil
	}
	seen := map[string]struct{}{}
	files := make([]string, 0)
	for _, commit := range event.Commits {
		if commit == nil {
			continue
		}
		for _, values := range [][]string{commit.Added, commit.Modified, commit.Removed} {
			for _, file := range values {
				file = strings.TrimSpace(file)
				if file == "" {
					continue
				}
				if _, ok := seen[file]; ok {
					continue
				}
				seen[file] = struct{}{}
				files = append(files, file)
			}
		}
	}
	return files
}

func (a *App) fetchTriggerManifest(owner, repo, commitSHA string) (models.Manifest, string, error) {
	manifest, source, found, err := a.loadTriggerManifestOverride(context.Background(), owner, repo)
	if err != nil {
		return models.Manifest{}, "", err
	}
	if found {
		return manifest, source, nil
	}

	// GitHub keeps repository fallback behavior. Other providers use GitOps/DB
	// trigger definitions until provider-neutral repository reads are available.
	content, err := a.requestGitBotFile(owner, repo, commitSHA, ".nopsai/triggers.yaml", errManifestNotFound)
	if err != nil {
		return models.Manifest{}, "", err
	}
	if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
		return models.Manifest{}, "", err
	}
	return manifest, "git", nil
}

func (a *App) loadTriggerManifestOverride(ctx context.Context, owner, repo string) (models.Manifest, string, bool, error) {
	fullName := repositoryFullName(owner, repo)
	var manifest models.Manifest
	matches, err := a.repositoryTeamMatches(ctx, owner, repo)
	if err != nil {
		return manifest, "", false, err
	}
	teamPaths := make([]string, 0, len(matches))
	for _, match := range matches {
		teamPaths = append(teamPaths, match.Path)
	}
	specificKeys, ownerWideKeys := repositoryTriggerOverrideKeys(owner, repo, teamPaths)
	dbSpecificKeys, err := a.triggerOverrideKeysEndingWith(ctx, fullName)
	if err != nil {
		return manifest, "", false, err
	}
	dbOwnerWideKeys, err := a.triggerOverrideKeysEndingWith(ctx, repositoryFullName(owner, "all"))
	if err != nil {
		return manifest, "", false, err
	}
	specificKeys = sortTriggerKeysBySpecificity(appendUniqueStrings(specificKeys, dbSpecificKeys))
	ownerWideKeys = sortTriggerKeysBySpecificity(appendUniqueStrings(ownerWideKeys, dbOwnerWideKeys))

	// 1. Try Specific Repo Override
	for _, key := range specificKeys {
		if overrideDef, err := a.getTriggerOverride(ctx, key); err != nil {
			return manifest, "", false, err
		} else if overrideDef != "" {
			if err := yaml.Unmarshal([]byte(overrideDef), &manifest); err != nil {
				return manifest, "", false, err
			}
			log.Info().Str("repository", fullName).Str("trigger", key).Msg("Using trigger override from database")
			return manifest, "database override", true, nil
		}
	}

	// 2. Try Owner-Wide "all" Override
	for _, key := range ownerWideKeys {
		if overrideDef, err := a.getTriggerOverride(ctx, key); err != nil {
			return manifest, "", false, err
		} else if overrideDef != "" {
			if err := yaml.Unmarshal([]byte(overrideDef), &manifest); err != nil {
				return manifest, "", false, err
			}
			log.Info().Str("repository", fullName).Str("owner_trigger", key).Msg("Using owner-wide trigger override from database")
			return manifest, "database owner override", true, nil
		}
	}
	return manifest, "", false, nil
}

func (a *App) ensureCheckRunAsync(runID uuid.UUID, pipeline models.Pipeline, resolvedPipelineDef []byte, gitCtx map[string]string, pipelineSource string, isRerun bool) {
	owner := strings.TrimSpace(gitCtx["repo_owner"])
	repo := strings.TrimSpace(gitCtx["repo_name"])
	commitSHA := strings.TrimSpace(gitCtx["commit_sha"])
	checkRunIDStr := strings.TrimSpace(gitCtx["check_run_id"])
	if owner == "" || repo == "" || commitSHA == "" {
		return
	}

	go func() {
		ctx := context.Background()
		if checkRunIDStr != "" {
			checkRunID, err := strconv.ParseInt(checkRunIDStr, 10, 64)
			if err != nil {
				log.Warn().Err(err).Str("check_run_id", checkRunIDStr).Msg("Invalid check run ID provided; skipping initialization")
				return
			}
			if isRerun {
				if err := a.initializeGitHubCheckRun(owner, repo, checkRunID, resolvedPipelineDef, pipeline.Name); err != nil {
					log.Error().Err(err).Int64("check_run_id", checkRunID).Msg("Failed to initialize rerun check run (async)")
				}
			}
			if _, err := a.db.Exec(ctx, "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, runID); err != nil {
				log.Error().Err(err).Str("run_id", runID.String()).Int64("check_run_id", checkRunID).Msg("Failed to persist provided check run ID (async)")
			}
			return
		}

		checkRunID, err := a.createGitHubCheckRun(owner, repo, commitSHA, resolvedPipelineDef, pipelineSource)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to create check run (async)")
			return
		}

		if _, err := a.db.Exec(ctx, "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, runID); err != nil {
			log.Error().Err(err).Str("run_id", runID.String()).Int64("check_run_id", checkRunID).Msg("Failed to persist check run ID (async)")
		} else {
			log.Info().Str("run_id", runID.String()).Int64("check_run_id", checkRunID).Msg("Attached check run to pipeline run (async)")
		}
	}()
}

func (a *App) fetchPipelineFromDB(path, name string) ([]byte, error) {
	var pipelineDef string
	err := a.db.QueryRow(context.Background(), "SELECT definition FROM pipelines WHERE path = $1 AND name = $2", path, name).Scan(&pipelineDef)
	if err == pgx.ErrNoRows {
		return nil, errPipelineNotFound
	}
	if err != nil {
		return nil, err
	}
	return []byte(pipelineDef), nil
}

func (a *App) pipelineExistsInDB(path, name string) (bool, error) {
	var exists bool
	err := a.db.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM pipelines WHERE path = $1 AND name = $2)", path, name).Scan(&exists)
	return exists, err
}

func repositoryPipelineSourceForIdentifier(path, name, ext string) models.PipelineSource {
	repoPath := configsync.BuildPipelineFilePath(path, name, ext)
	if !strings.HasPrefix(repoPath, ".nopsai/") {
		repoPath = ".nopsai/" + repoPath
	}
	return models.PipelineSource{Path: repoPath}
}

func isDatabasePipelineSource(source string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "database")
}

func (a *App) notifyImmediateCheckFailure(owner, repo string, checkRunID int64, commitSHA, summary string) {
	gitContext := map[string]string{
		"repo_owner":   owner,
		"repo_name":    repo,
		"check_run_id": strconv.FormatInt(checkRunID, 10),
		"commit_sha":   commitSHA,
	}
	a.notifyGitBotOfFinalStatus("failure", "", "", summary, gitContext)
}
