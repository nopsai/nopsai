import type { WikiSection } from '../types.js';

export const gettingStartedSection: WikiSection = {
  id: 'getting-started',
  title: 'Getting started',
  owner: 'Product',
  description: 'The path from an empty machine to a green run: install the stack, finish setup, learn the topology, then build and trigger a first pipeline.',
  articles: [
    {
      id: 'what-nopsai-is',
      title: 'What NopsAI is',
      docType: 'concept',
      audiences: ['new-user', 'administrator', 'developer'],
      summary:
        'NopsAI is a self-hosted automation control plane that executes YAML-defined workflows through Docker or Kubernetes runners.',
      keywords: ['overview', 'introduction', 'product', 'what is'],
      keyFacts: [
        'A pipeline can combine deterministic shell scripts, LLM-backed goals, reusable steps, child pipelines, and human approval gates in one graph.',
        'Runtime variables, encrypted secrets, governed Knowledge Context, Models, Agent roles, and MCP Profiles are all resolved before a step executes.',
        'Runs start manually, from a schedule, from a GitHub App event, from a Git webhook source, from an external API trigger, or from a parent pipeline.',
        'Final deliverables can be Markdown, JSON, HTML, PDF, Excel, or a dashboard publication, and are stored separately from raw task logs.',
        'Everything is self-hosted: there is no NopsAI-operated cloud component in the current repository.',
      ],
      details: [
        'The product is GitOps-friendly by design. Pipelines, steps, schedules, triggers, scopes, knowledge, dashboards, and access can all live in a configuration repository, while durable execution, audit, credential, monitoring, and setup state stay in PostgreSQL.',
        'LLM-backed work runs inside the per-run agent. There is no separate always-on LLM service; the agent calls the provider selected by the resolved Model.',
        'The UI, CLI, REST API, hosted MCP surface, and runners all pass through the same authentication, authorization, and audit boundaries. No interface has a private path around AAA.',
      ],
      limits: [
        'The repository defines no cloud-provider-specific infrastructure automation. Cloud installs treat NopsAI as a portable Kubernetes workload.',
      ],
      related: ['control-execution-plane', 'run-lifecycle', 'concepts-glossary', 'install-local-docker-compose'],
      sources: [
        { repositoryPath: 'doc/architecture-overview.md', purpose: 'Component map and deployment shape.' },
        { repositoryPath: 'doc/feature-reference.md', purpose: 'Functional capabilities exposed by the codebase and UI.' },
      ],
    },
    {
      id: 'requirements',
      title: 'Requirements',
      docType: 'how-to',
      audiences: ['new-user', 'administrator'],
      summary: 'What must be true on the host before the stack will start, and the values you have to generate first.',
      keywords: ['requirements', 'prerequisites', 'ports', 'docker', 'secrets', 'before you install', 'generate'],
      keyFacts: [
        'Everything is a container. The control plane, UI, PostgreSQL, and the runner all run on one Compose bridge network named `nopsai-net` — see [Architecture and networking](architecture-and-networking).',
        'Seven values must exist before the first start: `POSTGRES_PASSWORD`, `DATABASE_URL`, `NOPSAI_MASTER_KEY`, `JWT_SIGNING_KEY`, `SERVICE_JWT_SIGNING_KEY`, `AAA_SHARED_INTERNAL_TOKEN`, and `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD`. Compose exits with `set <NAME>` when one is missing.',
        '`JWT_SIGNING_KEY` and `SERVICE_JWT_SIGNING_KEY` must be different values, so a user token can never be replayed as a service token.',
        'Published ports bind to `127.0.0.1` through `NOPSAI_BIND_ADDRESS`: 80 for the UI, 8080 for the API, 8081 for git-bot, 9091 for the dispatcher, and 5432 for PostgreSQL.',
        'The bootstrap administrator is `NOPSAI_BOOTSTRAP_ADMIN_EMAIL`, default `admin@example.com`, and must rotate its password at first login unless `NOPSAI_BOOTSTRAP_ADMIN_MUST_CHANGE_PASSWORD` is set to `false`.',
        'Image tags come from `NOPSAI_VERSION` (default `dev`) and `NOPSAI_IMAGE_REGISTRY` (default `ghcr.io/nopsai`). Either build them from a checkout or make that registry reachable.',
      ],
      prerequisites: [
        { label: 'Container runtime', value: 'Docker Engine with the Compose v2 plugin', verification: 'docker compose version' },
        { label: 'Daemon access', value: 'The current user can talk to the Docker daemon without sudo', verification: 'docker ps' },
        { label: 'Free ports', value: '80, 8080, 8081, 9091, and 5432 unused on the loopback interface', verification: 'lsof -nP -iTCP:80,8080,8081,9091,5432 -sTCP:LISTEN' },
        { label: 'Value generation', value: 'A source of high-entropy values for the seven bootstrap secrets', verification: 'openssl rand -hex 32' },
        { label: 'Images', value: 'A repository checkout to build from, or network access to the configured image registry' },
      ],
      steps: [
        {
          title: 'Confirm the runtime',
          description: 'Compose v2 is required; the v1 `docker-compose` binary is not supported by this file.',
          commands: [
            { title: 'Check Docker and Compose', language: 'bash', code: 'docker compose version\ndocker ps' },
          ],
          expectedOutput: 'Compose reports a v2 version and `docker ps` answers without a permission error.',
        },
        {
          title: 'Confirm the ports are free',
          description: 'The stack publishes five ports. A port already in use is the most common reason a first start half-succeeds.',
          commands: [
            { title: 'List conflicting listeners', language: 'bash', code: 'lsof -nP -iTCP:80,8080,8081,9091,5432 -sTCP:LISTEN' },
          ],
          verification: 'Nothing is listed. Anything reported must be stopped or the published port changed before starting.',
        },
        {
          title: 'Decide how the stack is reached',
          description:
            'By default every published port binds to `127.0.0.1`, so the install is reachable only from the workstation running it. Set `NOPSAI_BIND_ADDRESS` only when you consciously want the stack on the network.',
          warning:
            'Binding to a non-loopback address publishes the API, git-bot, dispatcher, and PostgreSQL ports. Work through the production hardening checklist before doing that.',
        },
        {
          title: 'Generate the seven values',
          description:
            'Generate them fresh rather than copying from another environment. The database URL has to carry the same password you generated for PostgreSQL.',
          commands: [
            {
              title: 'Generate bootstrap values',
              language: 'bash',
              code: [
                'POSTGRES_PASSWORD=$(openssl rand -hex 16)',
                'cat <<EOF',
                'POSTGRES_PASSWORD=$POSTGRES_PASSWORD',
                'DATABASE_URL=postgres://nopsai:$POSTGRES_PASSWORD@db:5432/nopsai?sslmode=disable',
                'NOPSAI_MASTER_KEY=$(openssl rand -hex 32)',
                'JWT_SIGNING_KEY=$(openssl rand -hex 32)',
                'SERVICE_JWT_SIGNING_KEY=$(openssl rand -hex 32)',
                'AAA_SHARED_INTERNAL_TOKEN=$(openssl rand -hex 32)',
                'NOPSAI_BOOTSTRAP_ADMIN_PASSWORD=$(openssl rand -hex 12)',
                'EOF',
              ].join('\n'),
              expectedOutput: 'Seven lines, each with a distinct value, ready to be written to `.env` in the next page.',
            },
          ],
          verification: 'Confirm `JWT_SIGNING_KEY` and `SERVICE_JWT_SIGNING_KEY` are not the same string.',
        },
      ],
      details: [
        'The Compose stack is built for evaluation and development. A shared or production install uses a release bundle or the Helm chart, and the same seven values still have to exist — they just come from a secret store instead of a local `.env`.',
        'Nothing here is wasted on a later move: the values, the pipelines, and the configuration you create locally can be exported into a configuration repository and applied to another install.',
        'The Docker runner you install later mounts `/var/run/docker.sock`, which lets it create containers on its host. Choose a host you are willing to treat as an execution boundary.',
      ],
      limits: [
        'The repository does not publish minimum CPU, memory, or disk figures for the Compose stack. Size the host for the workloads your pipelines will run, not for the control plane alone.',
      ],
      related: ['install-local-docker-compose', 'architecture-and-networking', 'deployment-models', 'production-hardening'],
      sources: [
        { repositoryPath: 'docker-compose.yaml', purpose: 'Required variables, published ports, bind address, and network name.' },
        { repositoryPath: 'release/version.txt', purpose: 'The product version that image tags follow.' },
      ],
    },
    {
      id: 'install-local-docker-compose',
      title: 'Install locally with Docker Compose',
      docType: 'tutorial',
      audiences: ['new-user', 'developer', 'administrator'],
      summary: 'Start a local stack, confirm the control-plane services, and verify that the Docker runner registers.',
      keywords: ['install', 'local', 'compose', 'evaluation', 'quickstart'],
      keyFacts: [
        'The checked-in Compose file is for local evaluation and development, not production.',
        'The UI is published on http://localhost/ and the API on http://localhost:8080.',
        'The Docker runner needs Docker socket access because it creates agent and step containers. Treat the runner host as a trusted execution boundary. See [Runners and the dispatcher](runners-and-dispatcher).',
        'Compose fails fast if the required bootstrap secrets are missing, so set them in `.env` before starting. [Requirements](requirements) lists all seven.',
      ],
      prerequisites: [
        { label: 'Repository', value: 'A current checkout of the NopsAI repository', verification: 'git rev-parse --short HEAD' },
        { label: 'Runtime', value: 'Docker 26+ with Compose v2', verification: 'docker compose version' },
        { label: 'Ports', value: '80, 8080, 9091, and 5432 free on the workstation' },
        { label: 'Permission', value: 'Local Docker access for the current user', verification: 'docker ps' },
      ],
      steps: [
        {
          title: 'Set the required bootstrap secrets',
          description:
            'Compose refuses to start without them. Generate high-entropy values rather than reusing anything from another environment.',
          commands: [
            {
              title: 'Create .env',
              language: 'bash',
              code: [
                'cat >> .env <<EOF',
                'POSTGRES_PASSWORD=$(openssl rand -hex 16)',
                'DATABASE_URL=postgres://nopsai:$(grep POSTGRES_PASSWORD .env | cut -d= -f2)@db:5432/nopsai?sslmode=disable',
                'NOPSAI_MASTER_KEY=$(openssl rand -hex 32)',
                'JWT_SIGNING_KEY=$(openssl rand -hex 32)',
                'SERVICE_JWT_SIGNING_KEY=$(openssl rand -hex 32)',
                'AAA_SHARED_INTERNAL_TOKEN=$(openssl rand -hex 32)',
                'NOPSAI_BOOTSTRAP_ADMIN_PASSWORD=$(openssl rand -hex 12)',
                'EOF',
              ].join('\n'),
              placeholders: ['Adjust the database user and host if you changed the Compose defaults.'],
            },
          ],
          warning: 'JWT_SIGNING_KEY and SERVICE_JWT_SIGNING_KEY must be different values.',
        },
        {
          title: 'Build and start the stack',
          description: 'Run the Compose topology from the repository root.',
          commands: [{ title: 'Start Compose', language: 'bash', code: 'docker compose up -d --build' }],
          expectedOutput:
            'Compose creates the nopsai-net network and starts PostgreSQL, API, AAA, dispatcher, git-bot, UI, Gotenberg, the socket proxy, and the Docker runner.',
        },
        {
          title: 'Check service health',
          description: 'Confirm the core services are up before opening the setup flow.',
          commands: [
            {
              title: 'Inspect local services',
              language: 'bash',
              code: 'docker compose ps\ncurl -s localhost:8080/livez\ncurl -s localhost:8080/healthz',
            },
          ],
          verification: '`/livez` answers immediately; `/healthz` becomes ready once PostgreSQL is reachable.',
        },
        {
          title: 'Follow the setup logs',
          description: 'Keep the services that own first-run setup and runner registration visible while you complete the wizard.',
          commands: [
            { title: 'Follow logs', language: 'bash', code: 'docker compose logs -f nopsai aaa dispatcher docker-runner' },
          ],
          verification: 'Open http://localhost/ and confirm the setup page loads.',
        },
      ],
      details: [
        'The Compose stack is optimized for local inspection. It is the fastest way to validate product behavior before moving to a release bundle or a Helm deployment.',
        'Everything you configure through the wizard can later be exported to a configuration repository, so a local evaluation is not throwaway work.',
      ],
      examples: [
        {
          title: 'Tear the stack down again',
          language: 'bash',
          code: 'docker compose down\n# add -v to also drop the database volume\ndocker compose down -v',
          expectedOutput: 'All containers stop and the nopsai-net network is removed.',
        },
      ],
      limits: ['Local fallback secrets exist for development only and must be replaced outside a workstation.'],
      related: ['complete-first-install-wizard', 'docker-compose', 'production-hardening'],
      sources: [
        { repositoryPath: 'docker-compose.yaml', purpose: 'The exact local topology, ports, and environment variables.' },
        { repositoryPath: 'doc/enterprise-gates.md', purpose: 'Startup gates that separate local defaults from production requirements.' },
      ],
    },
    {
      id: 'complete-first-install-wizard',
      title: 'Complete the first-install wizard',
      docType: 'tutorial',
      audiences: ['new-user', 'administrator'],
      summary: 'Run the one-time bootstrap that unlocks the platform, creates the first administrator, and seeds GitOps.',
      keywords: ['setup', 'bootstrap', 'wizard', 'first install', 'onboarding'],
      keyFacts: [
        'Normal authenticated routes and APIs stay locked until setup completes once.',
        '`GET /v1/setup/preflight` reports exactly what still blocks setup, including a database that is still starting.',
        'The bootstrap administrator must rotate the provisioned password at first login by default.',
        'The wizard can seed starter profiles, generate secrets, and create the initial GitOps layout and repository teams.',
      ],
      prerequisites: [
        { label: 'Stack', value: 'A running control plane', verification: 'curl -s localhost:8080/healthz' },
        { label: 'Bootstrap admin', value: 'NOPSAI_BOOTSTRAP_ADMIN_EMAIL and NOPSAI_BOOTSTRAP_ADMIN_PASSWORD set' },
        { label: 'Database', value: 'PostgreSQL reachable from the API' },
        { label: 'Browser access', value: 'http://localhost/ reachable from your workstation' },
      ],
      steps: [
        {
          title: 'Check preflight',
          description: 'Preflight tells you whether the platform can be set up right now and, if not, why.',
          commands: [{ title: 'Read preflight', language: 'bash', code: 'curl -s localhost:8080/v1/setup/preflight | jq' }],
          expectedOutput: 'A JSON document listing outstanding blockers. An empty blocker list means setup can proceed.',
        },
        {
          title: 'Run the wizard',
          description: 'Open the UI and work through the setup steps: administrator, secrets, starter profiles, and GitOps seeding.',
          verification: '`GET /v1/setup/status` reports that setup has completed.',
          warning:
            'Setup apply errors include the actionable write or configuration reason. Read the message rather than retrying blindly.',
        },
        {
          title: 'Rotate the bootstrap password',
          description: 'Log in as the bootstrap administrator. The forced rotation runs before you reach the application.',
          verification: 'You can log in with the new password and reach the dashboard.',
        },
        {
          title: 'Confirm the runner registered',
          description: 'Open System, then the dispatcher workspace, and confirm at least one runner is connected and dispatchable.',
          verification: 'The runner appears in the fleet table with a reachable, dispatch-enabled status.',
        },
      ],
      details: [
        'Setup preflight is deliberately available before authentication so an operator can diagnose a stuck install without a token.',
        'First-install setup does not force an env-file write for dispatcher TLS when a valid effective service JWT fallback is already configured.',
      ],
      related: ['first-install-wizard', 'authentication-and-sso', 'gitops-and-config-repositories'],
      sources: [
        { repositoryPath: 'doc/first-install-wizard.md', purpose: 'The bootstrap flow, starter profiles, and production guardrails.' },
        { repositoryPath: 'services/nopsai/setup_preflight.go', purpose: 'Preflight behavior during cold starts.' },
      ],
    },
    {
      id: 'architecture-and-networking',
      title: 'Architecture and networking',
      docType: 'concept',
      audiences: ['new-user', 'administrator', 'operator'],
      summary: 'Which containers exist, how they address each other, which ports are published, and why a runner never needs an inbound port.',
      keywords: ['architecture', 'networking', 'ports', 'nopsai-net', 'topology', 'containers', 'routes'],
      keyFacts: [
        'Compose creates one bridge network named `nopsai-net`; every service resolves the others by container name on it.',
        'Five ports are published to the host, all on `127.0.0.1` by default: UI 80, API 8080, git-bot 8081, dispatcher 9091, PostgreSQL 5432.',
        'In-network addresses are fixed by Compose: the API is `http://nopsai:8080`, AAA is `http://aaa:8082`, git-bot is `http://nopsai-git-bot:8081`, Gotenberg is `http://gotenberg:3000`, and the Docker socket proxy is `tcp://docker-socket-proxy:2375`.',
        'The dispatcher process listens on `:9090` inside its container; the published host port 9091 maps onto it. Callers find it through `DISPATCHER_GRPC_ADDRESS`.',
        'Runners dial out to the dispatcher and keep that stream open. Work is assigned back over it, so a runner needs no inbound port and no public address.',
        'The `docker-runner` service in the checked-in Compose file only builds the image — its entrypoint is `true`. A working runner is installed separately.',
      ],
      details: [
        'Two planes share the network. The durable control plane is the API, AAA, PostgreSQL, git-bot, Gotenberg, and the socket proxy: everything that must survive a restart lives there. The ephemeral execution plane is the dispatcher, runners, per-run agents, and step containers.',
        'Only two hops need to work from a runner host: the dispatcher gRPC endpoint it dials, and the API URL the per-run agent calls back on for status, logs, outputs, and approvals. Everything else the runner needs arrives over those two connections.',
        'The Docker socket proxy exists so System Logs can read container logs without handing the API a writable Docker socket. It exposes allow-listed reads only.',
        'When the generated runner install detects that the dispatcher address resolves to the machine running the command, it rewrites the address to `host.docker.internal` and adds a host-gateway mapping, because a bridge container cannot reach the host through `localhost`.',
        'Inbound Git traffic terminates at git-bot on 8081 for GitHub App deliveries, while other providers deliver to the API through a configured Git webhook source. Nothing else needs to be reachable from the internet.',
      ],
      examples: [
        {
          title: 'Compose topology and published ports',
          language: 'text',
          code: [
            'host (127.0.0.1)          nopsai-net (bridge)',
            '',
            ':80    ───────────────▶  nopsai-ui',
            '                              │ http://nopsai:8080',
            ':8080  ───────────────▶  nopsai ──────┬───────▶ aaa            (http://aaa:8082)',
            '                              │       ├───────▶ db             (postgres:5432)',
            '                              │       ├───────▶ gotenberg      (http://gotenberg:3000)',
            '                              │       └───────▶ docker-socket-proxy (tcp://…:2375)',
            '                              │',
            ':8081  ───────────────▶  nopsai-git-bot ──────▶ nopsai',
            '                              │',
            ':9091  ───────────────▶  dispatcher (listens :9090)',
            '                              ▲',
            '                              │ runner dials out and keeps the stream open',
            '                         docker runner ──▶ agent ──▶ step containers',
            '',
            ':5432  ───────────────▶  db',
          ].join('\n'),
        },
        {
          title: 'Inspect the network and what is attached to it',
          language: 'bash',
          code: "docker network inspect nopsai-net --format '{{range .Containers}}{{.Name}} {{.IPv4Address}}\\n{{end}}'",
          expectedOutput: 'One line per attached container, including any runner you install later on the same network.',
        },
        {
          title: 'Check each published surface',
          language: 'bash',
          code: [
            'curl -s localhost:8080/livez',
            'curl -s localhost:8080/healthz',
            'curl -s localhost:8080/version',
            'curl -sI localhost/ | head -1',
          ].join('\n'),
          expectedOutput: '`/livez` answers as soon as the process is up; `/healthz` turns ready once PostgreSQL is reachable; `/version` returns build identity; the UI answers with an HTTP status line.',
        },
      ],
      related: ['control-execution-plane', 'add-docker-runner', 'networking-and-exposure', 'docker-compose'],
      sources: [
        { repositoryPath: 'docker-compose.yaml', purpose: 'Network name, published ports, and the in-network service addresses.' },
        { repositoryPath: 'services/dispatcher/internal/app/app.go', purpose: 'Dispatcher listen address default.' },
        { repositoryPath: 'services/nopsai/internal/runnerinstall/docker.go', purpose: 'Host-gateway rewrite and runner network placement.' },
      ],
    },
    {
      id: 'add-docker-runner',
      title: 'Add a Docker runner',
      docType: 'tutorial',
      audiences: ['new-user', 'administrator', 'operator'],
      summary: 'Generate a one-time install command from the control plane and start a runner container on the same Docker network.',
      keywords: ['runner', 'docker', 'install', 'bootstrap', 'register', 'dispatcher', 'scopes', 'capacity'],
      keyFacts: [
        'The control plane generates the install. You never assemble runner identity, service tokens, or TLS material by hand.',
        '`GET /v1/system/dispatcher/runner-bootstrap-command` returns a one-time command; its token expires in 10 minutes and is consumed by the first successful download.',
        'Query parameters choose identity and placement: `runner_id`, `runner_name`, `runner_scopes`, `runner_capacity`, `runner_network_mode` (`bridge`, `host`, or `auto`), and `runner_image`.',
        'In bridge mode the generated `docker run` joins the network named by `DOCKER_NETWORK_NAME`, which is `nopsai-net` for the local stack.',
        'The container runs with `--restart always`, mounts `/var/run/docker.sock`, and carries `RUNNER_ID`, `RUNNER_SCOPES`, `RUNNER_CAPACITY`, `DISPATCHER_GRPC_ADDRESS`, `NOPSAI_API_URL`, and the service JWT and TLS values as environment.',
        'An explicitly empty `runner_scopes` value means all scopes; omitting the parameter falls back to the configured default, then to `prod`.',
        'The install script stops early when the runner image cannot be pulled, or when the image architecture does not match the Docker host.',
      ],
      prerequisites: [
        { label: 'Control plane', value: 'A running API that has completed first-install setup', verification: 'curl -s localhost:8080/healthz' },
        { label: 'Token', value: 'An administrator token in `NOPSAI_TOKEN`', verification: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" localhost:8080/v1/auth/me' },
        { label: 'Runner host', value: 'A host with Docker and access to `/var/run/docker.sock`', verification: 'docker info --format "{{.Architecture}}"' },
        { label: 'Runner image', value: 'The runner image tag available locally or pullable on that host', verification: 'docker image inspect ghcr.io/nopsai/nopsai-docker-runner:dev >/dev/null && echo present' },
      ],
      steps: [
        {
          title: 'Generate the install command',
          description:
            'Ask the control plane for a bootstrap command for the runner you want. Scopes decide which work this runner may be given; capacity decides how many runs it accepts at once.',
          commands: [
            {
              title: 'Request a bootstrap command',
              language: 'bash',
              code: [
                'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  "http://localhost:8080/v1/system/dispatcher/runner-bootstrap-command?runner_id=runner-local-1&runner_scopes=prod&runner_capacity=2&runner_network_mode=bridge" \\',
                '  | jq -r .bootstrap_command',
              ].join('\n'),
              placeholders: ['`runner_id` and `runner_scopes` should match how you intend to route work.'],
              expectedOutput: 'A single-line command that downloads a one-time install script with a bearer token and runs it.',
            },
          ],
          warning: 'The command expires 10 minutes after it is generated and is consumed by the first successful download. Generate a new one per host.',
        },
        {
          title: 'Run it on the runner host',
          description:
            'Paste the generated command on the Docker host that will execute pipeline work. For the local stack, that is the same workstation running Compose.',
          expectedOutput:
            'The script prints the runner ID, dispatcher address, and network mode, pulls the runner image if it is missing, starts the container, and tails its first log lines.',
          verification: 'docker ps --filter "label=nopsai.io/runner-id" --format "{{.Names}} {{.Status}}"',
        },
        {
          title: 'Confirm the runner is on the same network',
          description:
            'A bridge-mode runner must sit on `nopsai-net` to reach the dispatcher by name. This is the failure most often mistaken for a credentials problem.',
          commands: [
            {
              title: 'List attached containers',
              language: 'bash',
              code: "docker network inspect nopsai-net --format '{{range .Containers}}{{.Name}}\\n{{end}}'",
            },
          ],
          verification: 'The runner container appears in the list alongside the control-plane containers.',
        },
        {
          title: 'Confirm registration with the dispatcher',
          description: 'Registration is what makes the runner dispatchable. Check it from the API or from System, then the dispatcher workspace.',
          commands: [
            {
              title: 'Read dispatcher status',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/system/dispatcher | jq',
            },
          ],
          expectedOutput: 'The runner appears with its ID, scopes, and capacity, reachable and dispatch-enabled.',
          verification: 'If it does not appear, follow the container logs: `docker logs -f <runner container>`.',
        },
      ],
      details: [
        'Prefer the bootstrap command over hand-written Compose for a first runner: it carries the dispatcher address, service JWT settings, and TLS material that the control plane currently considers correct, and it fails loudly when one of them is not configured.',
        '`GET /v1/system/dispatcher/runner-compose` returns the same install as a Compose service fragment when you would rather keep the runner in a Compose file than as a standalone container.',
        'When the dispatcher address in the response is not reachable from the runner host, fix the address before installing. The generated command reports the address it will use, and the response carries warnings when the control plane had to derive it from the request.',
        'Host network mode exists for the case where the host can reach the dispatcher but bridge containers cannot. It is the right answer on some VM setups and the wrong answer on a normal local install.',
      ],
      limits: [
        'One bootstrap command installs one runner. A second host needs its own command and its own runner ID.',
      ],
      related: ['runners-and-dispatcher', 'architecture-and-networking', 'first-script-pipeline', 'private-registry-auth'],
      sources: [
        { repositoryPath: 'services/nopsai/internal/runnerinstall/docker.go', purpose: 'Install spec, generated docker run, network placement, and one-time token behavior.' },
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'Dispatcher install and status routes.' },
      ],
    },
    {
      id: 'first-script-pipeline',
      title: 'Create and run your first pipeline',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author', 'developer'],
      summary: 'Write a three-step pipeline where each step depends on the previous one and passes a value forward through runtime outputs.',
      keywords: ['first pipeline', 'script', 'steps', 'depends_on', 'outputs', 'variables', 'tutorial', 'run'],
      keyFacts: [
        'A pipeline needs only `name` and `steps`; `container_image` sets the image every step runs in unless a step overrides it. [Pipeline anatomy](pipeline-anatomy) covers every top-level directive.',
        'Steps run in dependency order, not list order. `depends_on` is what serialises them — see [Dependencies and parallelism](dependencies-and-parallelism).',
        'A step publishes a value by writing a file under `/nopsai/outputs` whose name is exactly the output name, then declaring it under `outputs`. [Step outputs](step-outputs) has the full rules.',
        'A consumer reads it in `variables` as `$steps.<step>.outputs.<NAME>`, which must be the entire value of that variable.',
        'Output names must match `^[A-Za-z_][A-Za-z0-9_]*$`, and a missing file fails the step with `required output file /nopsai/outputs/<NAME> was not produced`.',
        '`POST /v1/pipelines/validate` and `PUT /v1/pipelines/{name}` both take the YAML document as the request body; validation also accepts JSON with the definition under `yaml` or `content`.',
        'A dependency path to the producer is required to consume its output, but it does not have to be a direct `depends_on` edge.',
      ],
      prerequisites: [
        { label: 'Setup complete', value: 'First-install setup has finished and you can sign in', verification: 'curl -s localhost:8080/v1/setup/status' },
        { label: 'Runner', value: 'At least one registered, dispatch-enabled runner', verification: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" localhost:8080/v1/system/dispatcher | jq' },
        { label: 'Scope', value: 'A runtime scope to run in, such as the scope created during setup' },
      ],
      steps: [
        {
          title: 'Write the pipeline',
          description:
            'Three steps: one produces a tag, one consumes it and produces an artifact name, one reports both. Each step declares what it publishes and what it consumes.',
          commands: [
            {
              title: 'first-pipeline.yaml',
              language: 'yaml',
              code: [
                'name: first-pipeline',
                'container_image: alpine:3.20',
                'steps:',
                '  - name: prepare',
                '    script: |',
                '      echo "1.0.$(date +%s)" > /nopsai/outputs/BUILD_TAG',
                '      echo "prepared $(cat /nopsai/outputs/BUILD_TAG)"',
                '    outputs:',
                '      - name: BUILD_TAG',
                '',
                '  - name: build',
                '    depends_on: [prepare]',
                '    variables:',
                '      BUILD_TAG: $steps.prepare.outputs.BUILD_TAG',
                '    script: |',
                '      echo "building $BUILD_TAG"',
                '      echo "app-$BUILD_TAG-linux-amd64" > /nopsai/outputs/ARTIFACT',
                '    outputs:',
                '      - name: ARTIFACT',
                '',
                '  - name: report',
                '    depends_on: [build]',
                '    variables:',
                '      BUILD_TAG: $steps.prepare.outputs.BUILD_TAG',
                '      ARTIFACT: $steps.build.outputs.ARTIFACT',
                '    script: |',
                '      echo "built $ARTIFACT from tag $BUILD_TAG"',
              ].join('\n'),
              expectedOutput: '`report` prints both values, which proves the whole chain resolved.',
            },
          ],
        },
        {
          title: 'Validate before running it',
          description:
            'Validation catches an undefined dependency, an output that is consumed without a dependency path, and a reference embedded in a larger string, each with a distinct message.',
          commands: [
            {
              title: 'Validate the definition',
              language: 'bash',
              code: [
                'curl -sX POST http://localhost:8080/v1/pipelines/validate \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Content-Type: application/yaml" \\',
                '  --data-binary @first-pipeline.yaml | jq',
              ].join('\n'),
              placeholders: ['A JSON request works too, with the definition under `yaml` or `content`.'],
            },
          ],
          verification: 'Validation returns no errors. Read the message before changing the graph — it names the failure mode.',
        },
        {
          title: 'Save the pipeline',
          description: 'Store the definition under the name you want to run it by. The UI editor writes the same definition through the same route.',
          commands: [
            {
              title: 'Create or update the pipeline',
              language: 'bash',
              code: [
                'curl -sX PUT http://localhost:8080/v1/pipelines/first-pipeline \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Content-Type: application/yaml" \\',
                '  --data-binary @first-pipeline.yaml',
              ].join('\n'),
            },
          ],
          verification: 'The pipeline appears in Pipelines in the UI, and `GET /v1/pipelines` lists it.',
        },
        {
          title: 'Run it',
          description: 'Start a run in the scope you want it resolved in. The scope decides which variables and secrets the run can see.',
          commands: [
            {
              title: 'Start a run',
              language: 'bash',
              code: [
                'curl -sX POST http://localhost:8080/v1/run/first-pipeline \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Accept: application/json" \\',
                '  -H "Content-Type: application/json" \\',
                '  -d \'{"scope":"platform/production"}\' | jq',
              ].join('\n'),
              placeholders: ['Replace `platform/production` with a scope that exists in your install.'],
            },
          ],
          expectedOutput:
            '`{"run_id": "...", "trigger_event_id": ""}`. Keep the run ID: the next pages use it for logs and history. Without `Accept: application/json` the same route answers with a plain-text confirmation instead.',
        },
        {
          title: 'Confirm the value travelled',
          description: 'Read the last step output rather than trusting a green status: the point of this pipeline is that a value crossed two step boundaries.',
          commands: [
            {
              title: 'Read the run logs',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs/$RUN_ID/logs" | jq -r \'.[].line\' | tail -20',
            },
          ],
          verification: 'The `report` step prints `built app-1.0.<timestamp>-linux-amd64 from tag 1.0.<timestamp>`.',
        },
      ],
      details: [
        'Steps that do not depend on each other run concurrently up to runner capacity. This pipeline is deliberately a straight line so the output chain is visible; removing `depends_on` from `build` would make it start immediately and fail to resolve `BUILD_TAG`.',
        'The long form of an output reference is `$steps.<step>.<task>.outputs.<NAME>`. A single-task step publishes under its own name, so the short `$steps.<step>.outputs.<NAME>` used here resolves to the same value.',
        'Mark an output `sensitive: true` when it carries a credential; the value stays available downstream but is masked wherever logs are rendered. Ordinary outputs stay readable on purpose, so release evidence such as versions and image references remains reviewable.',
        '`RUNTIME_OUTPUT_MAX_BYTES` caps a single output value and defaults to 65536 bytes.',
      ],
      limits: [
        'A runtime output reference is never a valid `depends_on` value; declare the dependency by name.',
      ],
      related: ['first-variables-and-secrets', 'step-outputs', 'pipeline-anatomy', 'first-run-logs-history'],
      sources: [
        { repositoryPath: 'pkg/models/runtime_outputs.go', purpose: 'Reference parsing for the short and long output forms.' },
        { repositoryPath: 'services/agent/internal/app/runtime_outputs.go', purpose: 'Output file collection and the missing-file failure.' },
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'Validate, save, and run routes used in this walkthrough.' },
      ],
    },
    {
      id: 'first-variables-and-secrets',
      title: 'Add a variable and a secret',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author', 'security'],
      summary: 'Store a scoped variable and an encrypted secret, then consume both from the pipeline you just built.',
      keywords: ['variable', 'secret', 'scope', 'encrypted', 'redaction', 'environment', 'tutorial'],
      keyFacts: [
        'A scope is the namespace a run resolves variables and secrets in, and it is chosen when the run starts. [Variables and scopes](pipeline-variables) has the resolution rules; [Credentials and secrets store](credentials) covers the platform credential registry.',
        '`PUT /v1/variables/{name}?scope=<scope>` and `PUT /v1/secrets/{name}?scope=<scope>` both take `{"value": "..."}`.',
        'Listing secrets returns names and metadata only. Reading a value is a separate route guarded by its own action, `secret.read_value`.',
        'A pipeline declares what it needs: `variables` at the top level, and `secrets` on the step that consumes them.',
        'A bare name resolves in the run scope; `scope/path:NAME` resolves in another scope and is still injected under the bare name.',
        'A declared variable that cannot be resolved fails the run before execution instead of starting with a missing value.',
      ],
      prerequisites: [
        { label: 'Pipeline', value: 'The pipeline from the previous page, saved and runnable' },
        { label: 'Scope', value: 'The scope you ran it in, for example `platform/production`' },
        { label: 'Token', value: 'A token allowed to write variables and secrets in that scope' },
      ],
      steps: [
        {
          title: 'Store a variable',
          description: 'Variables are plain configuration. They are readable back through the API, which makes them the wrong place for credentials.',
          commands: [
            {
              title: 'Create a scoped variable',
              language: 'bash',
              code: [
                'curl -sX PUT "http://localhost:8080/v1/variables/RELEASE_CHANNEL?scope=platform/production" \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Content-Type: application/json" \\',
                '  -d \'{"value":"stable"}\'',
              ].join('\n'),
            },
          ],
          verification: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/variables?scope=platform/production" | jq',
        },
        {
          title: 'Store a secret',
          description: 'Secrets are encrypted with the master key before storage. The value can be read back later, but only by a caller granted `secret.read_value` on that secret.',
          commands: [
            {
              title: 'Create a scoped secret',
              language: 'bash',
              code: [
                'RELEASE_TOKEN=$(openssl rand -hex 12)',
                'curl -sX PUT "http://localhost:8080/v1/secrets/RELEASE_TOKEN?scope=platform/production" \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Content-Type: application/json" \\',
                '  -d "{\\"value\\":\\"$RELEASE_TOKEN\\"}"',
              ].join('\n'),
            },
          ],
          warning: 'Losing `NOPSAI_MASTER_KEY` makes every stored secret unreadable. It belongs in the same place as your other break-glass material.',
        },
        {
          title: 'Declare them in the pipeline',
          description:
            'The pipeline states what it requires. The variable is declared once at the top level; the secret is declared on the step that needs it, so it is not injected into steps that do not.',
          commands: [
            {
              title: 'Extend first-pipeline.yaml',
              language: 'yaml',
              code: [
                'name: first-pipeline',
                'container_image: alpine:3.20',
                'variables:',
                '  - RELEASE_CHANNEL',
                'steps:',
                '  - name: prepare',
                '    script: |',
                '      echo "1.0.$(date +%s)" > /nopsai/outputs/BUILD_TAG',
                '      echo "channel is $RELEASE_CHANNEL"',
                '    outputs:',
                '      - name: BUILD_TAG',
                '',
                '  - name: publish',
                '    depends_on: [prepare]',
                '    secrets:',
                '      - RELEASE_TOKEN',
                '    variables:',
                '      BUILD_TAG: $steps.prepare.outputs.BUILD_TAG',
                '    script: |',
                '      echo "publishing $BUILD_TAG to $RELEASE_CHANNEL"',
                '      echo "token length is ${#RELEASE_TOKEN}"',
              ].join('\n'),
              expectedOutput: 'The step can use the secret, but printing it would show as a masked value in the logs.',
            },
          ],
        },
        {
          title: 'Run it and check redaction',
          description: 'Run in the same scope, then read the logs and confirm the secret value never appears in clear text.',
          commands: [
            {
              title: 'Run and inspect',
              language: 'bash',
              code: [
                'curl -sX POST http://localhost:8080/v1/run/first-pipeline \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Accept: application/json" \\',
                '  -H "Content-Type: application/json" \\',
                '  -d \'{"scope":"platform/production"}\' | jq -r .run_id',
              ].join('\n'),
            },
          ],
          verification: 'The logs show the channel and the token length, never the token itself.',
        },
      ],
      details: [
        'Names must match `^[A-Za-z0-9_.-]+$`, and the same runtime name may not resolve from two different scopes in one place — that is a conflict, not a precedence rule.',
        'Cross-scope use is explicit: `platform/shared:ARTIFACT_BUCKET` resolves in `platform/shared` and is injected as `ARTIFACT_BUCKET`. It never happens implicitly.',
        'To keep a secret in a configuration repository instead of the database, encrypt it first through `POST /v1/secrets/encrypt`; the envelope is safe to commit and resolves at run time.',
        'Step `variables` apply to every task in a step; task `variables` are applied afterwards and win.',
        'Scope is not team path. Scope decides which values resolve; team path decides who owns the resource and who is notified.',
      ],
      limits: [
        'A secret value is recoverable by anyone granted `secret.read_value` on it. Grant that action deliberately: it is the difference between storing a credential and publishing it.',
      ],
      related: ['pipeline-variables', 'credentials', 'first-script-pipeline', 'first-external-trigger'],
      sources: [
        { repositoryPath: 'services/nopsai/secrets_variables_handlers.go', purpose: 'Scope query parameter, request body, and encryption on write.' },
        { repositoryPath: 'pkg/models/runtime_refs.go', purpose: 'Scoped reference parsing and name validation.' },
      ],
    },
    {
      id: 'connect-git-repository',
      title: 'Connect a Git repository',
      docType: 'tutorial',
      audiences: ['new-user', 'developer', 'administrator'],
      summary: 'Give NopsAI access to a repository so it can read pipelines, knowledge, and trigger manifests.',
      keywords: ['git', 'github', 'repository', 'connect', 'integration'],
      keyFacts: [
        'GitHub uses a GitHub App with one or more installations, configured in `setting/git-apps/github.yaml`.',
        'GitLab, Bitbucket, Gitea, and generic providers use managed Git Webhook Sources instead.',
        'Repository access is what lets a run check out code and read repo-local Knowledge Context at the run commit.',
      ],
      prerequisites: [
        { label: 'Provider', value: 'A repository on GitHub, GitLab, Bitbucket, or Gitea' },
        { label: 'Permission', value: 'Administrator access to configure Git Apps or webhook sources' },
        { label: 'Reachability', value: 'The provider must be able to reach your NopsAI webhook endpoint' },
        { label: 'Credentials', value: 'A GitHub App private key, or a webhook signing secret for other providers' },
      ],
      steps: [
        {
          title: 'Register the App or webhook source',
          description:
            'For GitHub, add the App ID and private key credential reference, then add the installation. For other providers, create a Git Webhook Source and copy its signing secret into the provider.',
          verification: 'Use the installation Verify action, or check recent deliveries on the webhook source.',
        },
        {
          title: 'Confirm repository visibility',
          description: 'List the repositories the installation can reach.',
          commands: [
            {
              title: 'List installation repositories',
              language: 'bash',
              code: 'curl -s localhost:8080/v1/git-apps/github/installations/$INSTALLATION_ID/repositories \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
              placeholders: ['$INSTALLATION_ID — from the installations list.'],
            },
          ],
        },
        {
          title: 'Send a test event',
          description: 'Push a commit, then check that the delivery arrived and was accepted.',
          verification: 'The delivery appears with a success status under the webhook source or App installation.',
        },
      ],
      details: [
        'Store the App private key and webhook secret as credential references, not as inline environment values. The legacy inline variables remain only for migration.',
        'Internal service URLs such as `git_bot_api_url` stay in system configuration and do not belong in the app-scoped Git App file.',
      ],
      related: ['git-triggers', 'git-webhook-sources', 'credentials'],
      sources: [
        { repositoryPath: 'doc/git-apps.md', purpose: 'Multi-installation management, GitOps schema, and git-bot routing.' },
        { repositoryPath: 'doc/git-webhook-sources.md', purpose: 'Non-GitHub provider configuration and security.' },
      ],
    },
    {
      id: 'trigger-pipeline-from-git',
      title: 'Trigger a pipeline from Git',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author', 'developer'],
      summary: 'Add a `.nopsai.yaml` trigger manifest so pushes and pull requests start pipelines automatically.',
      keywords: ['trigger', 'ci', 'push', 'pull request', 'webhook'],
      keyFacts: [
        'The trigger manifest lives in the repository, so trigger rules are reviewed like code.',
        'Path filters fail open: when the provider does not report changed files, the rule still matches so CI is not silently skipped.',
        '`branches` and `skip_branches` are evaluated together — includes first, then exclusions.',
      ],
      prerequisites: [
        { label: 'Repository', value: 'A connected repository from the previous tutorial' },
        { label: 'Pipeline', value: 'A pipeline file committed in the repository' },
        { label: 'Permission', value: 'Write access to the repository' },
        { label: 'Scope', value: 'A runtime scope the triggered run may use' },
      ],
      steps: [
        {
          title: 'Add the manifest',
          description: 'Commit `.nopsai.yaml` at the repository root.',
          commands: [
            {
              title: '.nopsai.yaml',
              language: 'yaml',
              code: [
                'team: platform/payments',
                'triggers:',
                '  - on: push',
                '    branches: [main]',
                '    include_paths:',
                '      - "services/payments/**"',
                '    pipelines:',
                '      - .nopsai/pipelines/ci.yaml',
                '    scope: platform/staging',
              ].join('\n'),
            },
          ],
        },
        {
          title: 'Push and watch',
          description: 'Push a change under the filtered path and confirm a run starts.',
          verification: 'A run appears under Pipeline Runs with the trigger recorded as its entry point.',
        },
        {
          title: 'Diagnose a non-match',
          description:
            'If no run starts, check the delivery on the webhook source or App installation, then compare the event name, branch, and changed paths against your rule.',
          commands: [
            {
              title: 'Check trigger analytics',
              language: 'bash',
              code: 'curl -s "localhost:8080/v1/monitoring/triggers/analytics" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
            },
          ],
        },
      ],
      details: [
        'For non-GitHub providers the manifest must also name the `webhook_source` that receives deliveries.',
        'Setting `management` marks the manifest as NopsAI-managed so platform trigger overrides apply.',
      ],
      related: ['git-triggers', 'git-webhook-sources', 'monitoring'],
      sources: [
        { repositoryPath: 'doc/triggering.md', purpose: 'Local GitHub and generic Git webhook simulation.' },
        { repositoryPath: 'pkg/gittrigger/matcher.go', purpose: 'Exact branch, tag, and path matching behavior.' },
      ],
    },
    {
      id: 'first-external-trigger',
      title: 'Trigger a run from outside',
      docType: 'tutorial',
      audiences: ['new-user', 'developer', 'automation-author'],
      summary: 'Create an external API trigger, restrict who may call it, map the payload into run variables, and invoke it.',
      keywords: ['external trigger', 'invoke', 'api', 'allowed_callers', 'idempotency', 'webhook', 'integration'],
      keyFacts: [
        'An external trigger is a named entry point that lets another system start one specific pipeline. [External API triggers](external-triggers) documents every field, and [External triggers API](api-external-triggers) documents the call.',
        '`allowed_callers` names explicit `user`, `service_account`, or `auth_team` callers, and narrows an already-authorized set rather than granting access.',
        '`variable_mapping` builds run variables from `payload.<path>`, `event_type`, `variables.<name>`, or `literal:<value>`.',
        '`payload_schema` rejects a malformed call before a run starts; `rate_limit.per_minute` caps invocations over the previous minute.',
        'An `idempotency_key` is scoped by trigger and caller, so a retried call returns the original run instead of starting a second one.',
        'Invocation history per trigger is the fastest way to see why a caller believes it is triggering runs but is being rejected.',
      ],
      prerequisites: [
        { label: 'Pipeline', value: 'A saved pipeline to start, such as `first-pipeline`' },
        { label: 'Caller identity', value: 'A service account, or your own user, to name in `allowed_callers`' },
        { label: 'Token', value: 'A token for that caller, used when invoking' },
      ],
      steps: [
        {
          title: 'Define the trigger',
          description:
            'The trigger fixes the pipeline, the scope, and the team that owns the runs it produces. The payload contract is part of the definition, not an afterthought.',
          commands: [
            {
              title: 'External trigger definition',
              language: 'yaml',
              code: [
                'name: start-first-pipeline',
                'pipeline: first-pipeline',
                'enabled: true',
                'scope: platform/production',
                'allowed_callers:',
                '  - service_account: release-bot',
                'payload_schema:',
                '  type: object',
                '  required: [channel]',
                '  properties:',
                '    channel: { type: string }',
                'variable_mapping:',
                '  RELEASE_CHANNEL: payload.channel',
                '  TRIGGERED_BY: literal:external',
                'rate_limit:',
                '  per_minute: 10',
              ].join('\n'),
            },
          ],
        },
        {
          title: 'Create it',
          description: 'Create the trigger through the API or the External Triggers page. Keep the returned ID: invocation uses it.',
          commands: [
            {
              title: 'Create and list triggers',
              language: 'bash',
              code: [
                'curl -sX POST http://localhost:8080/v1/external-triggers \\',
                '  -H "Authorization: Bearer $NOPSAI_TOKEN" \\',
                '  -H "Content-Type: application/json" \\',
                '  --data @trigger.json | jq -r .id',
                '',
                'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/external-triggers | jq',
              ].join('\n'),
            },
          ],
          verification: 'The trigger is listed and enabled.',
        },
        {
          title: 'Invoke it',
          description: 'Call as the identity you allowed. The payload must satisfy the schema, and the mapped variables become run variables.',
          commands: [
            {
              title: 'Invoke with an idempotency key',
              language: 'bash',
              code: [
                'curl -sX POST "http://localhost:8080/v1/external-triggers/$TRIGGER_ID/invoke" \\',
                '  -H "Authorization: Bearer $CALLER_TOKEN" \\',
                '  -H "Content-Type: application/json" \\',
                '  -d \'{"idempotency_key":"first-1","payload":{"channel":"stable"}}\' | jq',
              ].join('\n'),
            },
          ],
          expectedOutput: 'A run is started and returned. Repeating the same call with the same key returns that same run.',
        },
        {
          title: 'Check the invocation history',
          description: 'Every accepted and rejected call is recorded against the trigger, with the reason.',
          commands: [
            {
              title: 'Read invocations',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/external-triggers/$TRIGGER_ID/invocations" | jq',
            },
          ],
          verification: 'The invocation appears with its caller, payload result, and the run it produced.',
        },
      ],
      details: [
        'An external trigger is the right entry point for a change-management system, a chat command, or another platform. For a repository event, use a Git trigger manifest instead: it carries branch and path rules the external trigger has no opinion about.',
        'For providers other than GitHub, a [Git webhook source](git-webhook-sources) terminates the delivery: each source owns its signing secret, receives deliveries at `POST /v1/git/webhooks/{sourceID}`, verifies the signature, and normalises the payload into the same event model GitHub App events use — so [trigger manifests](git-triggers) stay provider-neutral.',
        'Only the webhook ingress needs to be reachable by the provider. Nothing else about the install has to be exposed for Git-driven runs to work.',
        'An empty `allowed_callers` list does not widen access: AAA still authorizes the caller against the trigger resource.',
      ],
      related: ['external-triggers', 'git-webhook-sources', 'trigger-pipeline-from-git', 'tokens-and-service-accounts'],
      sources: [
        { repositoryPath: 'services/nopsai/external_triggers.go', purpose: 'Invocation, idempotency, and rate-limit handling.' },
        { repositoryPath: 'services/nopsai/external_triggers_gitops.go', purpose: 'Trigger document schema.' },
      ],
    },
    {
      id: 'first-run-logs-history',
      title: 'Runs, logs, and history',
      docType: 'tutorial',
      audiences: ['new-user', 'operator', 'automation-author'],
      summary: 'Read what a run did: its status, its step graph, its logs, its outputs, and how it compares to the runs before it.',
      keywords: ['run', 'logs', 'history', 'status', 'rerun', 'cancel', 'graph', 'outputs'],
      keyFacts: [
        'A run finishes `success`, `failure`, `warning`, `timed_out`, or `canceled`. [Pipeline runs](pipeline-runs) explains what each status means operationally.',
        '`warning` means work failed but was ignored: the failing task stays auditable as `failure (ignored)`.',
        '`timed_out` is what an expired run timeout or approval timeout produces — it is not a crash.',
        '`GET /v1/runs` lists runs, `GET /v1/runs/{runID}` returns detail, and `GET /v1/runs/{runID}/logs` returns log records whose text is under `line`.',
        'The run list filters on `limit`, `offset`, `teamId`, and `branch`. There is no pipeline filter: select on `pipeline_name` in the response.',
        '`POST /v1/runs/{runID}/rerun` answers with `runId`, while `POST /v1/run/{pipeline}` answers with `run_id` — and only when the request carries `Accept: application/json`, otherwise it returns a plain-text confirmation.',
        '`POST /v1/runs/{runID}/rerun` starts a new run from the same definition; `POST /v1/runs/{runID}/cancel` stops one in flight.',
        'Declared secrets and outputs marked `sensitive` are masked wherever logs are rendered.',
      ],
      prerequisites: [
        { label: 'A finished run', value: 'The run ID from the pipeline you built earlier' },
        { label: 'Token', value: 'A token allowed to read runs in that team and scope' },
      ],
      steps: [
        {
          title: 'Find the run',
          description: 'The run list is the entry point in the UI and over the API. Pipeline Runs shows the same records the API returns.',
          commands: [
            {
              title: 'List recent runs',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs" | jq \'.[0:5]\'',
            },
          ],
          verification: 'Your run appears with its pipeline name and status.',
        },
        {
          title: 'Read the run detail',
          description:
            'Run detail carries the execution graph and per-step state. In the UI, selecting a multi-task step reveals its task graph below the step overview.',
          commands: [
            {
              title: 'Fetch one run',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs/$RUN_ID" | jq',
            },
          ],
          expectedOutput: 'Steps in dependency order, each with its status and timing.',
        },
        {
          title: 'Read the logs',
          description: 'Logs are per run and stay attached to it. This is where the value your last step printed shows up.',
          commands: [
            {
              title: 'Tail the log lines',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs/$RUN_ID/logs" | jq -r \'.[].line\' | tail -30',
            },
          ],
          verification: 'Secret values appear masked; non-sensitive outputs appear in clear text.',
        },
        {
          title: 'Compare against history',
          description:
            'A single green run proves little. Pipeline detail keeps a Runs tab so you can see whether this run behaved like the ones before it, and Health carries the analysis surface.',
          commands: [
            {
              title: 'Filter the run list by pipeline',
              language: 'bash',
              code: [
                'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs?limit=50" \\',
                '  | jq \'[.[] | select(.pipeline_name == "first-pipeline") | {run_id, status, started_at}]\'',
              ].join('\n'),
            },
          ],
          verification: 'Both of your runs are listed, newest first.',
        },
        {
          title: 'Re-run and cancel',
          description: 'Re-running starts a fresh run from the same definition rather than resuming the old one. Cancelling stops a run that is still in flight.',
          commands: [
            {
              title: 'Rerun, then cancel',
              language: 'bash',
              code: [
                'RERUN=$(curl -sX POST -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs/$RUN_ID/rerun" | jq -r .runId)',
                'curl -sX POST -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/runs/$RERUN/cancel" | jq',
              ].join('\n'),
            },
          ],
          expectedOutput: 'The rerun appears in history as its own record, and the cancelled run finishes as `canceled`.',
        },
      ],
      details: [
        'Run list surfaces expose only an aggregate `final_output_status`. Generated deliverable content stays on authorized detail paths, so a list view never leaks output contents.',
        'Pipeline detail is organised as Flow, Definition, Trigger rules, Runs, Health, and Dependencies. Health owns the full analysis result and the AI Evaluation surface.',
        'System logs are a different thing from run logs: run logs are what your steps produced, [system logs](system-logs) are the platform services. Reach for those when a run never started.',
      ],
      related: ['pipeline-runs', 'pipeline-logs', 'system-logs', 'monitoring'],
      sources: [
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'Run list, detail, logs, rerun, and cancel routes.' },
        { repositoryPath: 'doc/runtime-flows.md', purpose: 'Cancellation, rerun, and child pipeline flows.' },
      ],
    },
  ],
};
