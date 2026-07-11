package runs

import "strings"

type TeamResolutionCandidate struct {
	Kind     string
	Value    string
	Required bool
}

const (
	TeamResolutionPath = "path"
	TeamResolutionRepo = "repo"
)

func TeamResolutionCandidates(explicitTeamPath, pipelinePath string, gitContext map[string]string) []TeamResolutionCandidate {
	explicitTeamPath = strings.Trim(strings.TrimSpace(explicitTeamPath), "/")
	if explicitTeamPath != "" {
		return []TeamResolutionCandidate{{
			Kind:     TeamResolutionPath,
			Value:    explicitTeamPath,
			Required: true,
		}}
	}

	candidates := []TeamResolutionCandidate{}
	repoName := strings.TrimSpace(gitContext["repo_name"])
	if repoName != "" {
		candidates = append(candidates, TeamResolutionCandidate{
			Kind:  TeamResolutionRepo,
			Value: repositoryFullName(gitContext["repo_owner"], repoName),
		})
	}

	pipelinePath = strings.Trim(strings.TrimSpace(pipelinePath), "/")
	if pipelinePath != "" {
		candidates = append(candidates, TeamResolutionCandidate{Kind: TeamResolutionPath, Value: pipelinePath})
	}
	return candidates
}

func repositoryFullName(owner, repo string) string {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" {
		return repo
	}
	return owner + "/" + repo
}
