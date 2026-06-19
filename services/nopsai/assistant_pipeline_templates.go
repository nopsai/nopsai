package nopsai

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type pipelineGenerationRequest struct {
	Name  string
	Goal  string
	Scope string
}

type pipelineGenerationResult struct {
	TemplateID      string
	Assumptions     []string
	RequiredVars    []string
	RequiredSecrets []string
	YAML            string
}

type pipelineTemplate struct {
	ID              string
	MatchTerms      []string
	RequiredVars    []string
	RequiredSecrets []string
	Assumptions     []string
	Render          func(pipelineGenerationRequest, pipelineTemplate) (pipelineGenerationResult, error)
}

var generatedPipelineNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func renderGeneratedPipeline(req pipelineGenerationRequest) pipelineGenerationResult {
	req.Name = normalizeGeneratedPipelineName(req.Name)
	if strings.TrimSpace(req.Goal) == "" {
		req.Goal = "Describe the desired automation goal here."
	}
	template := selectPipelineTemplate(req)
	result, err := template.Render(req, template)
	if err != nil || strings.TrimSpace(result.YAML) == "" {
		result = renderGenericPipelineTemplate(req, genericPipelineTemplate())
	}
	return result
}

func selectPipelineTemplate(req pipelineGenerationRequest) pipelineTemplate {
	corpus := strings.ToLower(req.Name + " " + req.Goal)
	best := genericPipelineTemplate()
	bestScore := 0
	for _, template := range pipelineTemplates() {
		score := 0
		for _, term := range template.MatchTerms {
			if strings.Contains(corpus, strings.ToLower(term)) {
				score++
			}
		}
		if score > bestScore {
			best = template
			bestScore = score
		}
	}
	if bestScore == 0 {
		return genericPipelineTemplate()
	}
	return best
}

func pipelineTemplates() []pipelineTemplate {
	return []pipelineTemplate{
		{
			ID:         "docker-ddd-publish-approval",
			MatchTerms: []string{"docker", "image", "publish", "container", "registry", "ddd", "domain-driven", "domain driven"},
			RequiredVars: []string{
				"IMAGE_REGISTRY",
				"IMAGE_REPOSITORY",
				"IMAGE_TAG",
			},
			RequiredSecrets: []string{
				"REGISTRY_USERNAME",
				"REGISTRY_PASSWORD",
			},
			Assumptions: []string{
				"The repository contains a Dockerfile at the repository root.",
				"The service keeps Domain-Driven Design boundaries in domain, application, and infrastructure packages or folders.",
				"The runner can build Docker images and reach the target registry.",
				"Registry credentials are supplied through scoped NopsAI secrets, not inline YAML.",
				"The final approval group exists at platform/prod.",
			},
			Render: renderDockerDDDPublishApprovalTemplate,
		},
		{
			ID:         "golang-aws-ecs",
			MatchTerms: []string{"go", "golang", "aws", "ecs", "ecr", "deploy"},
			RequiredVars: []string{
				"AWS_REGION",
				"AWS_ACCOUNT_ID",
				"ECR_REPOSITORY",
				"ECS_CLUSTER",
				"ECS_SERVICE",
				"NOPSAI_COMMIT_SHA",
			},
			RequiredSecrets: []string{
				"AWS_ACCESS_KEY_ID",
				"AWS_SECRET_ACCESS_KEY",
			},
			Assumptions: []string{
				"The Go service has a Dockerfile at the repository root.",
				"The runner can build Docker images and reach ECR/ECS.",
				"A production approval folder exists at platform/prod.",
				"AWS credentials are supplied through scoped NopsAI secrets, not inline YAML.",
			},
			Render: renderGolangAWSECSTemplate,
		},
	}
}

func genericPipelineTemplate() pipelineTemplate {
	return pipelineTemplate{
		ID:              "generic-investigate-build-validate",
		MatchTerms:      []string{},
		RequiredVars:    []string{},
		RequiredSecrets: []string{},
		Assumptions: []string{
			"The exact runtime, deployment target, and credentials need review before applying.",
			"The generated YAML is a GitOps proposal only.",
		},
		Render: func(req pipelineGenerationRequest, template pipelineTemplate) (pipelineGenerationResult, error) {
			return renderGenericPipelineTemplate(req, template), nil
		},
	}
}

func renderDockerDDDPublishApprovalTemplate(req pipelineGenerationRequest, template pipelineTemplate) (pipelineGenerationResult, error) {
	doc := baseGeneratedPipelineDoc(req, "Build and publish a Docker image after checking Domain-Driven Design boundaries.")
	doc["llm_enabled"] = false
	doc["variables"] = template.RequiredVars
	doc["steps"] = []map[string]any{
		{
			"name":  "ddd-standards-check",
			"image": "alpine:3.20",
			"tasks": []map[string]any{{
				"name": "check-ddd-boundaries",
				"script": strings.TrimSpace(`
set -eu
test -f Dockerfile
for layer in domain application infrastructure; do
  test -d "$layer" || test -d "src/$layer" || test -d "internal/$layer"
done
`),
			}},
		},
		{
			"name":       "test",
			"image":      "alpine:3.20",
			"depends_on": []string{"ddd-standards-check"},
			"tasks": []map[string]any{{
				"name": "run-project-tests",
				"script": strings.TrimSpace(`
set -eu
if [ -f Makefile ]; then
  make test
elif [ -f package.json ]; then
  apk add --no-cache nodejs npm
  npm ci
  npm test
elif [ -f go.mod ]; then
  apk add --no-cache go
  go test ./...
else
  echo "No known test entrypoint found; add make test, npm test, or go test before production use."
fi
`),
			}},
		},
		{
			"name":       "docker-build-publish",
			"image":      "docker:26-cli",
			"depends_on": []string{"test"},
			"secrets":    template.RequiredSecrets,
			"tasks": []map[string]any{{
				"name": "build-and-publish-image",
				"script": strings.TrimSpace(`
set -eu
IMAGE="$IMAGE_REGISTRY/$IMAGE_REPOSITORY:$IMAGE_TAG"
printf '%s' "$REGISTRY_PASSWORD" | docker login "$IMAGE_REGISTRY" --username "$REGISTRY_USERNAME" --password-stdin
docker build --pull -t "$IMAGE" .
docker push "$IMAGE"
`),
			}},
		},
		{
			"name":       "release-approval",
			"depends_on": []string{"docker-build-publish"},
			"approval": map[string]any{
				"type":                "image-publish-review",
				"groups":              []string{"platform/prod"},
				"allow_self_approval": false,
			},
		},
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return pipelineGenerationResult{}, err
	}
	return pipelineGenerationResult{
		TemplateID:      template.ID,
		Assumptions:     append([]string{}, template.Assumptions...),
		RequiredVars:    append([]string{}, template.RequiredVars...),
		RequiredSecrets: append([]string{}, template.RequiredSecrets...),
		YAML:            string(raw),
	}, nil
}

func renderGolangAWSECSTemplate(req pipelineGenerationRequest, template pipelineTemplate) (pipelineGenerationResult, error) {
	doc := baseGeneratedPipelineDoc(req, "Deploy a Go service to AWS ECS through ECR.")
	doc["llm_enabled"] = false
	doc["container_image"] = "alpine:3.20"
	doc["variables"] = template.RequiredVars
	doc["steps"] = []map[string]any{
		{
			"name":  "test",
			"image": "golang:1.22",
			"tasks": []map[string]any{{
				"name":   "go-test",
				"script": "go test ./...",
			}},
		},
		{
			"name":       "build",
			"image":      "golang:1.22",
			"depends_on": []string{"test"},
			"tasks": []map[string]any{{
				"name":   "go-build",
				"script": "CGO_ENABLED=0 GOOS=linux go build -o app ./...",
			}},
		},
		{
			"name":       "docker-build-push",
			"image":      "docker:26-cli",
			"depends_on": []string{"build"},
			"secrets":    template.RequiredSecrets,
			"tasks": []map[string]any{
				{
					"name": "login-ecr",
					"script": strings.TrimSpace(`
apk add --no-cache aws-cli
aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
`),
				},
				{
					"name": "build-push",
					"script": strings.TrimSpace(`
IMAGE="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/$ECR_REPOSITORY:$NOPSAI_COMMIT_SHA"
docker build -t "$IMAGE" .
docker push "$IMAGE"
`),
				},
			},
		},
		{
			"name":       "production-approval",
			"depends_on": []string{"docker-build-push"},
			"approval": map[string]any{
				"type":                "production-deploy",
				"groups":              []string{"platform/prod"},
				"allow_self_approval": false,
			},
		},
		{
			"name":       "deploy",
			"image":      "amazon/aws-cli:2.15.57",
			"depends_on": []string{"production-approval"},
			"secrets":    template.RequiredSecrets,
			"tasks": []map[string]any{{
				"name":   "update-ecs",
				"script": `aws ecs update-service --cluster "$ECS_CLUSTER" --service "$ECS_SERVICE" --force-new-deployment --region "$AWS_REGION"`,
			}},
		},
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return pipelineGenerationResult{}, err
	}
	return pipelineGenerationResult{
		TemplateID:      template.ID,
		Assumptions:     append([]string{}, template.Assumptions...),
		RequiredVars:    append([]string{}, template.RequiredVars...),
		RequiredSecrets: append([]string{}, template.RequiredSecrets...),
		YAML:            string(raw),
	}, nil
}

func renderGenericPipelineTemplate(req pipelineGenerationRequest, template pipelineTemplate) pipelineGenerationResult {
	doc := baseGeneratedPipelineDoc(req, "Plan, implement, and validate an automation change.")
	doc["container_image"] = "alpine:3.20"
	doc["llm_enabled"] = true
	doc["steps"] = []map[string]any{
		{
			"name": "plan",
			"tasks": []map[string]any{{
				"name": "draft-plan",
				"goal": "Create an implementation plan for: " + req.Goal,
			}},
		},
		{
			"name":       "implement",
			"depends_on": []string{"plan"},
			"tasks": []map[string]any{{
				"name": "draft-change",
				"goal": "Draft the safest GitOps-compatible change for: " + req.Goal,
			}},
		},
		{
			"name":       "validate",
			"depends_on": []string{"implement"},
			"tasks": []map[string]any{{
				"name": "validate-change",
				"goal": "Validate the proposed change and list risks, required variables, and rollback notes.",
			}},
		},
	}
	raw, _ := yaml.Marshal(doc)
	return pipelineGenerationResult{
		TemplateID:      template.ID,
		Assumptions:     append([]string{}, template.Assumptions...),
		RequiredVars:    append([]string{}, template.RequiredVars...),
		RequiredSecrets: append([]string{}, template.RequiredSecrets...),
		YAML:            string(raw),
	}
}

func baseGeneratedPipelineDoc(req pipelineGenerationRequest, description string) map[string]any {
	doc := map[string]any{
		"name":        req.Name,
		"version":     "latest",
		"description": description,
	}
	if scope := strings.Trim(strings.TrimSpace(req.Scope), "/"); scope != "" {
		doc["scope"] = scope
	}
	return doc
}

func normalizeGeneratedPipelineName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "generated-pipeline"
	}
	name = generatedPipelineNameSanitizer.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-._")
	if name == "" {
		return "generated-pipeline"
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-._")
	}
	if name == "" {
		return "generated-pipeline"
	}
	return name
}
