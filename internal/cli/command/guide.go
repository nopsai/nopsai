package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type guideTopic struct {
	Name    string
	Summary string
	Body    string
}

func newGuideCommand() *cobra.Command {
	var list bool
	command := &cobra.Command{
		Use:   "guide [TOPIC]",
		Short: "Show CLI examples and operator guidance",
		Args: func(command *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("accepts at most one topic")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			if list || len(args) == 0 {
				return renderGuideTopicList(command)
			}
			return renderGuideTopic(command, args[0])
		},
	}
	command.Flags().BoolVar(&list, "list", false, "list available guide topics")
	return command
}

func guideTopics() map[string]guideTopic {
	return map[string]guideTopic{
		"config": {
			Name:    "config",
			Summary: "Contexts, tokens, API overrides, and safe automation defaults.",
			Body: strings.TrimSpace(`
Contexts keep API URLs separate from credentials:

  nopsai context add prod --api https://api.nopsai.example
  nopsai context use prod
  nopsai login --token

Use --api for a one-command service address override without saving it:

  NOPSAI_TOKEN="$TOKEN" nopsai --api https://api-dr.example api request GET /v1/auth/me

Config files live under $NOPSAI_CONFIG_DIR or the OS user config directory.
Commit declarative GitOps config, not credentials.yaml. Environment tokens win
over stored credentials, which keeps CI deterministic.`),
		},
		"api": {
			Name:    "api",
			Summary: "Route discovery, required parameters, query flags, and payload samples.",
			Body: strings.TrimSpace(`
Search and inspect the compiled API catalog locally:

  nopsai api routes --domain monitoring --method GET
  nopsai api describe POST /v1/run --output text

Call registered routes with safe path expansion:

  nopsai api call GET '/v1/pipelines/{pipelineName...}' \
    --path pipelineName=delivery/release \
    --query include_source=true

When a route has required path/query/body inputs, api call reports the exact
--path, --query, --data, or --data-raw flag to provide. Use api request for
already-expanded paths or byte-exact uploads/downloads.`),
		},
		"install": {
			Name:    "install",
			Summary: "First-install environments, generated secrets, and service addresses.",
			Body: strings.TrimSpace(`
Use the first-install wizard for local or cluster setup:

  nopsai install

Noninteractive shortcuts stay available for GitOps and CI:

  nopsai install docker-compose --run
  nopsai install kubernetes --output-dir ./nopsai-prod --values-file values.yaml

Docker Compose generation writes .env with local generated secrets and editable
service addresses. Kubernetes generation writes values.yaml and expects an
external Secret for database URL, master key, JWT keys, service JWT key, and the
AAA shared internal token.`),
		},
		"gitops": {
			Name:    "gitops",
			Summary: "Declarative config, locks, and automation-safe CLI patterns.",
			Body: strings.TrimSpace(`
Keep environment state reviewable:

  nopsai install kubernetes --output-dir ./env/prod --values-file values.yaml
  git add env/prod/values.yaml env/prod/.nopsai/install.lock

Deploy from stored files after review:

  cd env/prod
  nopsai install kubernetes --output-dir . --values-file values.yaml --deploy --wait

Release locks are non-secret and should travel with environment state. Secrets
belong in the platform secret manager, SOPS, Sealed Secrets, External Secrets,
or equivalent enterprise secret workflow.`),
		},
		"monitoring": {
			Name:    "monitoring",
			Summary: "Health checks, platform doctor, metrics, and operational readouts.",
			Body: strings.TrimSpace(`
The default interactive home shows context, token, session, /healthz, /version,
and setup preflight when an API is configured.

Run the heavier diagnostic suite explicitly:

  nopsai platform doctor
  nopsai platform doctor --output json

Doctor checks local docker/kubectl/helm availability, API preflight, metrics,
AAA token acceptance, and dispatcher monitoring. JSON/YAML output is intended
for CI and monitoring ingestion.`),
		},
		"aaa": {
			Name:    "aaa",
			Summary: "Authentication, authorization, product roles, and token behavior.",
			Body: strings.TrimSpace(`
The CLI never bypasses AAA or audit middleware. Bearer tokens are ordinary API,
personal-access, or service-account tokens:

  nopsai login --token
  nopsai api request GET /v1/auth/me
  nopsai api describe POST /v1/authz/resource-use/check --output text

Public endpoints can be called without credentials by passing --no-auth.
Internal routes are visible in the catalog for operators, but still require
the correct internal service token and server-side authorization.`),
		},
		"mcp": {
			Name:    "mcp",
			Summary: "Hosted MCP endpoint samples and GitOps-safe MCP configuration.",
			Body: strings.TrimSpace(`
The hosted MCP endpoint is available through the same API transport:

  printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
    nopsai api request POST /v1/mcp --data -

Inspect MCP system routes and samples:

  nopsai api routes --domain mcp
  nopsai api describe POST /v1/mcp --output text

MCP servers and profiles remain owned by system or team configuration and are
permission-filtered through AAA before tools are exposed.`),
		},
	}
}

func renderGuideTopicList(command *cobra.Command) error {
	topics := guideTopics()
	names := make([]string, 0, len(topics))
	for name := range topics {
		names = append(names, name)
	}
	sort.Strings(names)
	if _, err := fmt.Fprintln(command.OutOrStdout(), "Available guide topics:"); err != nil {
		return err
	}
	for _, name := range names {
		topic := topics[name]
		if _, err := fmt.Fprintf(command.OutOrStdout(), "  %-12s %s\n", topic.Name, topic.Summary); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(command.OutOrStdout(), "\nUse `nopsai guide TOPIC` for examples.")
	return err
}

func renderGuideTopic(command *cobra.Command, raw string) error {
	name := strings.ToLower(strings.TrimSpace(raw))
	topic, ok := guideTopics()[name]
	if !ok {
		return fmt.Errorf("unknown guide topic %q; use `nopsai guide --list`", raw)
	}
	if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\n\n%s\n", strings.ToUpper(topic.Name), topic.Body); err != nil {
		return err
	}
	return nil
}
