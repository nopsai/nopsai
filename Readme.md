
protoc --go_out=. --go-grpc_out=. pkg/proto/agent.proto
mv nopsai/pkg/proto/* pkg/proto/ && rm -rf nopsai/

https://github.com/settings/apps/nopsai

docker-compose up --build -d


curl -X POST -H "Content-Type: application/x-yaml" --data-binary "@sample-pipeline/2-pipeline.yaml" http://localhost:8080/v1/run



Nopsai: An LLM-Powered CI/CD System
Nopsai is a modern, microservice-based CI/CD system that leverages the power of Large Language Models (LLMs) to orchestrate and execute complex pipelines. Instead of relying on rigid, pre-defined scripts, Nopsai uses high-level, natural language "goals" which are translated into executable commands by an AI agent at runtime. This approach provides a flexible and powerful automation platform.

🏛️ Architecture: Agent as a Service
The core of Nopsai is a powerful "Agent as a Service" model. For each pipeline run, a dedicated, ephemeral agent service is launched to act as the self-contained orchestrator for that specific job. This design makes the system highly scalable, resilient, and decoupled.

Architectural Flow
Initiation: A user submits a pipeline YAML to the nopsai service's API endpoint (POST /v1/run).

Launch: The nopsai service creates the initial run and step records in the PostgreSQL database. It then uses the Docker API to launch a new, dedicated agent container for that specific run, injecting all necessary configuration via environment variables.

Provisioning: The agent starts, connects to the Docker Host, and provisions the necessary resources for the pipeline, including a shared Docker volume and the pipeline's execution container.

Orchestration Loop:

The agent holds the pipeline state in memory and determines the next step based on dependencies.

It sends a gRPC request with the full context (goal, history, files, environment variables) to the llm-agent.

The llm-agent returns a specific, structured Action (e.g., EXECUTE_COMMAND, REPLACE_FILE).

The agent executes this Action inside the pipeline container using docker exec.

Status Reporting: After each step, the agent sends a stateless status update via an HTTP POST request to the nopsai service, which records the result in the database.

Cleanup: When the pipeline finishes (succeeds, fails, or times out), the agent is responsible for stopping and removing the pipeline container before it terminates itself. The nopsai service then cleans up the agent's volume and, if configured, the agent container itself.

🛠️ Service Roles & Integrations
The system is composed of four core microservices, each with a distinct and focused responsibility.

nopsai (API Gateway & Database Service)
The nopsai service is the single, authoritative entry point to the system and the sole guardian of the database.

Role: API Gateway & Database Proxy

Key Responsibilities:

Receives pipeline definitions via an HTTP endpoint (POST /v1/run).

Initializes run and step records in the PostgreSQL database.

Launches and manages the lifecycle of agent containers using the Docker Host API.

Receives step status updates from agents to record in the database.

Integrations:

Receives requests from the User or the git-bot.

Connects to the PostgreSQL Database to manage state.

Interacts with the Docker Host to manage container lifecycles.

agent (The Run Orchestrator)
The agent is an ephemeral, single-purpose service that manages one pipeline run from start to finish. It is the "brain" of a live run.

Role: Stateful Orchestrator for a Single Run

Key Responsibilities:

On startup, it provisions all necessary resources: a shared Docker volume and the pipeline container.

Holds the execution history and a live view of the file system in its memory.

Manages the step-by-step execution loop by calling the llm-agent for instructions.

Uses docker exec to run commands inside its sibling pipeline container.

Enforces the pipeline's configured timeout.

Integrations:

Is launched by the nopsai service.

Sends gRPC requests to the llm-agent to get actions.

Sends HTTP status updates back to the nopsai service's API.

Interacts with the Docker Host to manage its specific pipeline container.

llm-agent (AI Specialist)
The llm-agent is a pure, stateless function that translates a high-level goal into a concrete, executable action.

Role: AI-Powered Action Generator

Key Responsibilities:

Receives a self-contained context bundle from an agent via gRPC.

Builds a detailed prompt and queries the configured Gemini LLM.

Returns a single, structured Action (e.g., EXECUTE_COMMAND) back to the agent.

Integrations:

Only receives gRPC requests from agent containers. It has no knowledge of the wider system and does not connect to the database or Docker.

git-bot (Git Integration Service)
The git-bot service acts as a bridge between GitHub events and the Nopsai system.

Role: GitHub Webhook Processor and Status Reporter

Key Responsibilities:

Listens for incoming webhooks from GitHub (e.g., push, check_run, check_suite).

Validates webhook signatures for security.

Fetches the .nopsai.yaml pipeline file from the repository.

Initiates a pipeline run by making an API call to the nopsai service.

Receives status updates from the nopsai service and updates the corresponding GitHub Check Run in the GitHub UI.

Integrations:

Receives webhooks from GitHub.

Makes API calls to the nopsai service to trigger runs.

Makes API calls to the GitHub API to create and update Check Runs.

🚀 How to Run a Pipeline
To start a pipeline, submit the YAML definition to the nopsai service's /v1/run endpoint.

Bash

curl -X POST \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@path/to/your/pipeline.yaml" \
  http://localhost:8080/v1/run




  FUTURE PLAN
  High-Level Architectural Vision: Decoupling with a Message Bus
The most impactful change to elevate your architecture is to move from a synchronous, API-call-based system to an asynchronous, event-driven architecture. This is achieved by introducing a message bus (like RabbitMQ, NATS, or Kafka) as the central nervous system for your services.

This shifts the paradigm from services telling each other what to do (via direct API calls) to services announcing that something has happened and letting other interested services react accordingly.

Here's how the core workflow would change:

Current Synchronous Flow	Proposed Asynchronous Flow
1. User POSTs to nopsai API.	1. User POSTs to a lightweight api-gateway service.
2. nopsai writes to DB.	2. api-gateway publishes a run.requested event to the message bus.
3. nopsai uses Docker to start agent.	3. A dedicated run-orchestrator service consumes the event and starts the agent.
4. agent POSTs status updates to nopsai.	4. agent publishes step.completed or run.failed events to the message bus.
5. nopsai updates the DB.	5. A dedicated status-processor service consumes these events and updates the DB.

Export to Sheets
## 🏛️ Proposed Microservice Architecture
This new model breaks down the responsibilities of the original nopsai service into more specialized, independently scalable components.

Here are the new and evolved service roles:

1. api-gateway
This service becomes the single public entry point.

Role: Handles all incoming HTTP requests, validates user input/authentication, and translates them into events.

Action: Publishes a run.requested event to the message bus. It does not know or care about how a pipeline is run.

Benefit: Extremely lightweight and fast. You can scale it independently to handle massive amounts of incoming traffic without being bogged down by business logic.

2. run-orchestrator
This service is the new "brain" for starting pipelines.

Role: Its only job is to listen for run.requested events.

Action: When it receives an event, it uses a Scheduler (see below) to launch a new agent container, injecting the necessary configuration.

Benefit: Decouples the API from the execution backend. You could have a pool of these orchestrators to process new pipeline requests in parallel, scaling based on how many new pipelines you need to start per minute.

3. agent (Largely Unchanged)
The agent's core logic remains the same, as it's already well-designed.

Role: Manages a single pipeline run from start to finish.

Action: Instead of calling the nopsai API directly, it publishes events like step.completed, run.succeeded, or run.failed to the message bus.

Benefit: Resilience. If the backend services are temporarily unavailable, the agent can still publish its status. The message bus will hold the event until a consumer is ready.

4. status-processor
This service handles the state of the system.

Role: Listens for all step.* and run.* events from the agents.

Action: Updates the PostgreSQL database based on the event content.

Benefit: Batching & Efficiency. This service can be optimized to batch database updates. Instead of writing to the DB on every single step, it could collect (for example) 10 status updates and write them in a single transaction, significantly reducing database load.

5. git-bot (Evolved)
The git-bot integrates with the new event-driven flow.

Role: Listens for GitHub webhooks as before.

Action: Instead of calling the nopsai API, it simply publishes a git.push.received or git.rerun.requested event to the message bus. The run-orchestrator would also subscribe to these events to initiate runs. It would also subscribe to run.* events to know when to update a GitHub Check Run.

Benefit: Fully decouples your Git integration from your core pipeline execution logic.

## 🚀 Introducing a Scheduler/Cluster Manager
Instead of the run-orchestrator talking directly to the Docker socket, it should talk to an abstraction layer.

Initial Implementation: This could be a simple internal library that wraps the Docker client, just as you have now.

Future Scalability: This is where the true power comes in. You can replace this abstraction with a client for Kubernetes, Nomad, or AWS ECS. The run-orchestrator would simply say "run an agent with this config," and the cluster manager would handle scheduling it on a large cluster of machines. This allows you to scale your agent execution across hundreds or thousands of nodes, far beyond the limits of a single Docker host.

By adopting this more decoupled, event-driven architecture, your nopsai service will be exceptionally well-prepared for high-throughput, enterprise-level scale.