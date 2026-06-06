package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v53/github"
)

type checksProvider interface {
	CreateQueued(context.Context, createQueuedCheckRunRequest) (int64, error)
	MarkInProgress(context.Context, checkRunProgressUpdate) error
	Conclude(context.Context, checkRunConclusionUpdate) error
	ListForRef(ctx context.Context, owner, repo, ref string) ([]checkRunSummary, error)
}

type createQueuedCheckRunRequest struct {
	Owner string
	Repo  string
	Ref   string
	Name  string
}

type checkRunProgressUpdate struct {
	Owner      string
	Repo       string
	CheckRunID int64
	Name       string
	Title      string
	Summary    string
}

type checkRunConclusionUpdate struct {
	Owner       string
	Repo        string
	CheckRunID  int64
	Name        string
	Conclusion  string
	Title       string
	Summary     string
	CompletedAt time.Time
}

type checkRunSummary struct {
	ID                 int64
	Name               string
	Status             string
	HeadSHA            string
	HeadBranch         string
	PullRequestHeadRef string
	AppID              int64
	HasApp             bool
	CheckSuiteID       int64
	HasCheckSuite      bool
}

type githubChecksProvider struct {
	client *github.Client
}

func newGitHubChecksProvider(client *github.Client) checksProvider {
	return githubChecksProvider{client: client}
}

func (p githubChecksProvider) CreateQueued(ctx context.Context, req createQueuedCheckRunRequest) (int64, error) {
	checkRun, _, err := p.client.Checks.CreateCheckRun(ctx, req.Owner, req.Repo, github.CreateCheckRunOptions{
		Name:    req.Name,
		HeadSHA: req.Ref,
		Status:  github.String("queued"),
	})
	if err != nil {
		return 0, err
	}
	if checkRun == nil || checkRun.ID == nil {
		return 0, fmt.Errorf("created check run did not include an id")
	}
	return checkRun.GetID(), nil
}

func (p githubChecksProvider) MarkInProgress(ctx context.Context, update checkRunProgressUpdate) error {
	opts := github.UpdateCheckRunOptions{
		Name:   update.Name,
		Status: github.String("in_progress"),
	}
	if update.Title != "" || update.Summary != "" {
		opts.Output = &github.CheckRunOutput{
			Title:   github.String(update.Title),
			Summary: github.String(update.Summary),
		}
	}
	_, _, err := p.client.Checks.UpdateCheckRun(ctx, update.Owner, update.Repo, update.CheckRunID, opts)
	return err
}

func (p githubChecksProvider) Conclude(ctx context.Context, update checkRunConclusionUpdate) error {
	completedAt := update.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	opts := github.UpdateCheckRunOptions{
		Name:        update.Name,
		Status:      github.String("completed"),
		Conclusion:  github.String(update.Conclusion),
		CompletedAt: &github.Timestamp{Time: completedAt},
		Output: &github.CheckRunOutput{
			Title:   github.String(update.Title),
			Summary: github.String(update.Summary),
		},
	}
	_, _, err := p.client.Checks.UpdateCheckRun(ctx, update.Owner, update.Repo, update.CheckRunID, opts)
	return err
}

func (p githubChecksProvider) ListForRef(ctx context.Context, owner, repo, ref string) ([]checkRunSummary, error) {
	response, _, err := p.client.Checks.ListCheckRunsForRef(ctx, owner, repo, ref, &github.ListCheckRunsOptions{})
	if err != nil {
		return nil, err
	}
	if response == nil || len(response.CheckRuns) == 0 {
		return nil, nil
	}
	runs := make([]checkRunSummary, 0, len(response.CheckRuns))
	for _, checkRun := range response.CheckRuns {
		if checkRun == nil {
			continue
		}
		summary := checkRunSummary{
			ID:      checkRun.GetID(),
			Name:    checkRun.GetName(),
			Status:  checkRun.GetStatus(),
			HeadSHA: checkRun.GetHeadSHA(),
		}
		if app := checkRun.GetApp(); app != nil {
			summary.HasApp = true
			summary.AppID = app.GetID()
		}
		if suite := checkRun.CheckSuite; suite != nil {
			summary.HeadBranch = suite.GetHeadBranch()
			if suite.ID != nil {
				summary.HasCheckSuite = true
				summary.CheckSuiteID = suite.GetID()
			}
		}
		if len(checkRun.PullRequests) > 0 && checkRun.PullRequests[0] != nil {
			if head := checkRun.PullRequests[0].GetHead(); head != nil {
				summary.PullRequestHeadRef = head.GetRef()
			}
		}
		runs = append(runs, summary)
	}
	return runs, nil
}
