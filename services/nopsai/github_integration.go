package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
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

	pipelines, baseScope := findPipelinesForEvent(manifest, eventType, ref, repo)
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

		dbPath, dbName, extPart, parseErr := splitPipelineIdentifier(p.Path)
		if parseErr != nil {
			log.Warn().Err(parseErr).Str("pipeline", p.Path).Msg("Skipping pipeline due to invalid identifier")
			continue
		}

		callerID := repositoryFullName(owner, repo)
		normalizedPipelineID := buildPipelineIdentifier(dbPath, dbName)
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
						Str("caller_group", authz.CallerGroup).
						Str("resource_group", authz.ResourceGroup).
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
					Str("caller_group", scopeAuthz.CallerGroup).
					Str("resource_group", scopeAuthz.ResourceGroup).
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
	pathPart, namePart, _, err := splitPipelineIdentifier(identifier)
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

func (a *App) notifyGitBotOfFinalStatus(status, failedStep, failedTask, summary string, gitContext map[string]string) {
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)
	if checkRunID == 0 {
		if runID := strings.TrimSpace(gitContext["run_id"]); runID != "" {
			_ = a.db.QueryRow(context.Background(), "SELECT git_check_run_id FROM pipeline_runs WHERE run_id = $1", runID).Scan(&checkRunID)
		}
	}
	if checkRunID == 0 {
		return
	}
	gitBotURL := fmt.Sprintf("%s/v1/run/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"status":       status,
		"failed_step":  failedStep,
		"failed_task":  failedTask,
		"check_run_id": checkRunID,
		"repo_owner":   gitContext["repo_owner"],
		"repo_name":    gitContext["repo_name"],
		"commit_sha":   gitContext["commit_sha"],
		"summary":      summary,
	}
	body, _ := json.Marshal(payload)

	resp, err := a.postJSON(gitBotURL, body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of final status")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from git-bot")
	} else {
		log.Info().Msg("Successfully notified git-bot of final pipeline status.")
	}
}

func (a *App) notifyGitBotOfTaskStatus(runID, stepName, taskName, taskStatus string) {
	var repoOwner, repoName, commitSHA, pipelineDef sql.NullString
	var checkRunID sql.NullInt64
	var taskIndex, totalTasks int
	var startedAt, finishedAt sql.NullTime

	query := `
		SELECT
			r.git_repo_owner, r.git_repo_name, r.git_commit_sha, r.git_check_run_id, r.pipeline_definition,
			t.task_index, (SELECT COUNT(*) FROM task_runs WHERE run_id = r.run_id),
			t.started_at, t.finished_at
		FROM pipeline_runs r JOIN task_runs t ON r.run_id = t.run_id
		WHERE r.run_id = $1 AND t.step_name = $2 AND t.task_name = $3`

	err := a.db.QueryRow(context.Background(), query, runID, stepName, taskName).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID, &pipelineDef, &taskIndex, &totalTasks, &startedAt, &finishedAt)
	if err != nil || !repoOwner.Valid || !checkRunID.Valid {
		log.Warn().Str("run_id", runID).Err(err).Msg("Not a Git-triggered run with a check ID, skipping task status update.")
		return
	}

	var pipeline models.Pipeline
	var dependsOn []string
	if pipelineDef.Valid {
		if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err == nil {
			for _, step := range pipeline.Steps {
				if step.GetName() == stepName {
					for _, task := range step.GetTasks() {
						if task.Name == taskName {
							dependsOn = task.DependsOn
							break
						}
					}
					break
				}
			}
		}
	}

	gitBotURL := fmt.Sprintf("%s/v1/task/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"run_id":       runID,
		"repo_owner":   repoOwner.String,
		"repo_name":    repoName.String,
		"check_run_id": checkRunID.Int64,
		"commit_sha":   commitSHA.String,
		"step_name":    stepName,
		"task_name":    taskName,
		"task_status":  taskStatus,
		"task_index":   taskIndex,
		"total_tasks":  totalTasks,
		"depends_on":   dependsOn,
		"started_at":   startedAt.Time,
		"finished_at":  finishedAt.Time,
	}
	body, _ := json.Marshal(payload)

	resp, err := a.postJSON(gitBotURL, body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of task status")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from git-bot for task update")
	}
}

func (a *App) findEncryptedSecret(secretName, repoFullName, scope string) (string, string, bool, error) {
	var encryptedValue string
	storageScope := runtimeScopeForStorage(scope)
	resourceScope := runtimeScopeForResource(storageScope)

	err := a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3, storageScope)+" ORDER BY scope IS NULL ASC LIMIT 1", secretName, repoFullName, storageScope).Scan(&encryptedValue)
	if err == nil {
		return encryptedValue, model.BuildNamedResourceID(repoFullName, resourceScope, secretName), true, nil
	}
	if err != pgx.ErrNoRows {
		return "", "", false, err
	}

	err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2, storageScope)+" ORDER BY scope IS NULL ASC LIMIT 1", secretName, storageScope).Scan(&encryptedValue)
	if err == nil {
		return encryptedValue, model.BuildNamedResourceID("", resourceScope, secretName), true, nil
	}
	if err == pgx.ErrNoRows {
		return "", "", false, nil
	}
	return "", "", false, err
}

func findPipelinesForEvent(manifest models.Manifest, eventType, ref, repoName string) ([]models.PipelineSource, string) {
	for _, trigger := range manifest.Triggers {
		// Support "all" event type or specific match
		if trigger.On != eventType && trigger.On != "all" {
			continue
		}

		// Check for repo exceptions
		if len(trigger.SkipRepos) > 0 && isRepoSkipped(repoName, trigger.SkipRepos) {
			continue
		}

		if eventType == "push" {
			if strings.HasPrefix(ref, "refs/heads/") {
				branchName := strings.TrimPrefix(ref, "refs/heads/")
				branchIncluded := false
				if len(trigger.Branches) > 0 {
					branchIncluded = branchMatchesAnyPattern(branchName, trigger.Branches)
				} else if len(trigger.SkipBranches) > 0 {
					branchIncluded = true
				}
				// If "on: all", treat empty branches as "all branches"
				if trigger.On == "all" && len(trigger.Branches) == 0 {
					branchIncluded = true
				}

				if branchIncluded {
					if branchMatchesAnyPattern(branchName, trigger.SkipBranches) {
						continue
					}
					return trigger.Pipelines, trigger.Scope
				}
			} else if strings.HasPrefix(ref, "refs/tags/") {
				tagName := strings.TrimPrefix(ref, "refs/tags/")
				for _, pattern := range trigger.Tags {
					if matchBranchPattern(pattern, tagName) {
						return trigger.Pipelines, trigger.Scope
					}
				}
			}
		}

		if eventType == "pull_request" {
			return trigger.Pipelines, trigger.Scope
		}
	}
	return nil, ""
}

func branchMatchesAnyPattern(branchName string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchBranchPattern(pattern, branchName) {
			return true
		}
	}
	return false
}

func (a *App) getTriggerOverride(fullName string) (string, error) {
	var triggerDef string
	err := a.db.QueryRow(context.Background(), "SELECT trigger_definition FROM triggers WHERE repository_name = $1", fullName).Scan(&triggerDef)
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

func (a *App) fetchTriggerManifest(owner, repo, commitSHA string) (models.Manifest, string, error) {
	fullName := repositoryFullName(owner, repo)
	var manifest models.Manifest
	matches, err := a.repositoryGroupMatches(context.Background(), owner, repo)
	if err != nil {
		return manifest, "", err
	}
	groupPaths := make([]string, 0, len(matches))
	for _, match := range matches {
		groupPaths = append(groupPaths, match.Path)
	}
	specificKeys, ownerWideKeys := repositoryTriggerOverrideKeys(owner, repo, groupPaths)
	dbSpecificKeys, err := a.triggerOverrideKeysEndingWith(context.Background(), fullName)
	if err != nil {
		return manifest, "", err
	}
	dbOwnerWideKeys, err := a.triggerOverrideKeysEndingWith(context.Background(), repositoryFullName(owner, "all"))
	if err != nil {
		return manifest, "", err
	}
	specificKeys = sortTriggerKeysBySpecificity(appendUniqueStrings(specificKeys, dbSpecificKeys))
	ownerWideKeys = sortTriggerKeysBySpecificity(appendUniqueStrings(ownerWideKeys, dbOwnerWideKeys))

	// 1. Try Specific Repo Override
	for _, key := range specificKeys {
		if overrideDef, err := a.getTriggerOverride(key); err != nil {
			return manifest, "", err
		} else if overrideDef != "" {
			if err := yaml.Unmarshal([]byte(overrideDef), &manifest); err != nil {
				return manifest, "", err
			}
			log.Info().Str("repository", fullName).Str("trigger", key).Msg("Using trigger override from database")
			return manifest, "database override", nil
		}
	}

	// 2. Try Owner-Wide "all" Override
	for _, key := range ownerWideKeys {
		if overrideDef, err := a.getTriggerOverride(key); err != nil {
			return manifest, "", err
		} else if overrideDef != "" {
			if err := yaml.Unmarshal([]byte(overrideDef), &manifest); err != nil {
				return manifest, "", err
			}
			log.Info().Str("repository", fullName).Str("owner_trigger", key).Msg("Using owner-wide trigger override from database")
			return manifest, "database owner override", nil
		}
	}

	// 3. Fallback to Git
	content, err := a.requestGitBotFile(owner, repo, commitSHA, ".nopsai/triggers.yaml", errManifestNotFound)
	if err != nil {
		return manifest, "", err
	}
	if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
		return manifest, "", err
	}
	return manifest, "git", nil
}

func (a *App) postJSON(url string, body []byte) (*http.Response, error) {
	client := a.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return client.Post(url, "application/json", bytes.NewBuffer(body))
}

func (a *App) requestGitBotFile(owner, repo, ref, path string, notFoundErr error) (string, error) {
	payload := map[string]string{
		"owner": owner,
		"repo":  repo,
		"ref":   ref,
		"path":  path,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/file", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", err
		}
		return out.Content, nil
	case http.StatusNotFound:
		return "", notFoundErr
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("git-bot file request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (a *App) requestGitBotDirectory(owner, repo, ref, path string) (map[string]string, error) {
	payload := map[string]string{
		"owner": owner,
		"repo":  repo,
		"ref":   ref,
		"path":  path,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/contents", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Files map[string]string `json:"files"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		if out.Files == nil {
			out.Files = map[string]string{}
		}
		return out.Files, nil
	case http.StatusNotFound:
		return map[string]string{}, nil
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot contents request for '%s' failed with status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

func (a *App) branchHasOpenPullRequest(owner, repo, branch string) (bool, error) {
	payload := map[string]string{
		"owner":  owner,
		"repo":   repo,
		"branch": branch,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/branch/has-open-pr", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var out struct {
			HasOpenPR bool `json:"has_open_pr"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return false, err
		}
		return out.HasOpenPR, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("branch open PR check failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (a *App) ensureConfigRepoAccessible(owner, repo string) error {
	payload := map[string]string{
		"owner": owner,
		"repo":  repo,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/repo/access", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return fmt.Errorf("failed to verify config repository access: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(respBody))
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &errPayload); err == nil && errPayload.Error != "" {
		message = errPayload.Error
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("config repository '%s/%s' could not be found or Git Bot is not installed", owner, repo)
	case http.StatusForbidden:
		return fmt.Errorf("nopsai git-bot does not have permission to access config repository '%s/%s'", owner, repo)
	default:
		return fmt.Errorf("failed to verify config repository access for %s/%s (status %d): %s", owner, repo, resp.StatusCode, message)
	}
}

func (a *App) requestGitBotPipeline(owner, repo, ref string, source models.PipelineSource) ([]byte, error) {
	if source.Path != "" {
		content, err := a.requestGitBotFile(owner, repo, ref, source.Path, errPipelineNotFound)
		return []byte(content), err
	}

	payload := struct {
		Owner  string                `json:"owner"`
		Repo   string                `json:"repo"`
		Ref    string                `json:"ref"`
		Source models.PipelineSource `json:"source"`
	}{
		Owner:  owner,
		Repo:   repo,
		Ref:    ref,
		Source: source,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/pipeline", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return []byte(out.Content), nil
	case http.StatusNotFound:
		return nil, errPipelineNotFound
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot pipeline request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (a *App) findSuiteCheckRun(owner, repo string, suiteID int64, commitSHA string) (*suiteCheckRunResponse, error) {
	payload := map[string]interface{}{
		"owner":      owner,
		"repo":       repo,
		"suite_id":   suiteID,
		"commit_sha": commitSHA,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/find-suite-run", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to request suite check run from git-bot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot suite lookup failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out suiteCheckRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode git-bot suite lookup response: %w", err)
	}
	if out.CheckRunID == 0 || out.HeadSHA == "" {
		return nil, fmt.Errorf("git-bot returned incomplete suite check run data")
	}
	return &out, nil
}

func (a *App) createGitHubCheckRun(owner, repo, ref string, pipelineDef []byte, pipelineSource string) (int64, error) {
	payload := map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"ref":                 ref,
		"pipeline_definition": string(pipelineDef),
		"pipeline_source":     pipelineSource,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/create", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("git-bot check run creation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		CheckRunID int64 `json:"check_run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.CheckRunID, nil
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

func (a *App) initializeGitHubCheckRun(owner, repo string, checkRunID int64, pipelineDef []byte, pipelineName string) error {
	payload := map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"check_run_id":        checkRunID,
		"pipeline_definition": string(pipelineDef),
		"pipeline_name":       pipelineName,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/initialize", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("git-bot check run initialization failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (a *App) cancelStaleCheckRuns(owner, repo, beforeSHA string) {
	if beforeSHA == "" {
		return
	}
	payload := map[string]string{
		"owner":      owner,
		"repo":       repo,
		"before_sha": beforeSHA,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/cancel-stale", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to request stale check run cancellation")
		return
	}
	resp.Body.Close()
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
	repoPath := buildPipelineFilePath(path, name, ext)
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
