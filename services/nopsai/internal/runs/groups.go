package runs

import "strings"

type GroupResolutionCandidate struct {
	Kind     string
	Value    string
	Required bool
}

const (
	GroupResolutionPath = "path"
	GroupResolutionRepo = "repo"
)

func GroupResolutionCandidates(explicitGroupPath, pipelinePath string, gitContext map[string]string) []GroupResolutionCandidate {
	explicitGroupPath = strings.Trim(strings.TrimSpace(explicitGroupPath), "/")
	if explicitGroupPath != "" {
		return []GroupResolutionCandidate{{
			Kind:     GroupResolutionPath,
			Value:    explicitGroupPath,
			Required: true,
		}}
	}

	candidates := []GroupResolutionCandidate{}
	pipelinePath = strings.Trim(strings.TrimSpace(pipelinePath), "/")
	if pipelinePath != "" {
		candidates = append(candidates, GroupResolutionCandidate{Kind: GroupResolutionPath, Value: pipelinePath})
	}

	repoName := strings.TrimSpace(gitContext["repo_name"])
	if repoName != "" {
		candidates = append(candidates, GroupResolutionCandidate{
			Kind:  GroupResolutionRepo,
			Value: repositoryFullName(gitContext["repo_owner"], repoName),
		})
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
