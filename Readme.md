
protoc --go_out=. --go-grpc_out=. pkg/proto/agent.proto
mv nopsai/pkg/proto/* pkg/proto/ && rm -rf nopsai/

https://github.com/settings/apps/nopsai

docker-compose down -v && docker container prune -f && docker volume prune -f

# Secrets

// General
curl http://localhost:8080/v1/secrets

curl -X DELETE http://localhost:8080/v1/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "General level secret"}' \
  http://localhost:8080/v1/secrets/TEST_SECRET


// Repositories
curl http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets

curl -X DELETE http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "repo level secret"}' \
  http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET




# Pipelines  

curl http://localhost:8080/v1/pipelines

curl http://localhost:8080/v1/pipelines/main-pipeline.yaml

curl -X DELETE http://localhost:8080/v1/pipelines/main-pipeline.yaml

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/main-pipeline.yaml



# Trigger

curl http://localhost:8080/v1/overrides

curl http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X DELETE http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/hosein-yousefii/test-app





# NopsAI

### Core Philosophy & Architecture

* **LLM-Powered Pipelines**: Instead of rigid scripts, Nopsai uses high-level, natural language "goals" (e.g., "build the docker image") which are translated into executable commands by an AI agent at runtime.
* **Microservice-Based**: The system is built on a decoupled, microservice architecture, making it resilient, scalable, and easy to extend. The core services are:
    * **`nopsai`**: The main API gateway that manages pipeline runs, agent lifecycles, and the central database.
    * **`agent`**: An ephemeral, per-run service that orchestrates a single pipeline, communicating with the LLM to execute steps.
    * **`llm-agent`**: An AI service that uses the Gemini LLM to translate goals into executable shell commands.
    * **`git-bot`**: Integrates with GitHub to handle webhooks, trigger pipelines, and update check run statuses.
* **Agent as a Service Model**: For each pipeline run, a dedicated, ephemeral agent is launched to act as a self-contained orchestrator, making the system highly scalable and resilient.

---
### Pipeline & Step Configuration

* **Natural Language or Scripts**: Steps can be defined using a simple `goal` in natural language or a traditional `script` for direct command execution.
* **Per-Step Container Images**: Each step can run in its own dedicated container image, allowing you to use the perfect environment and toolset for every task. A default image can be set for the entire pipeline.
* **Dependency Management**: Steps can define dependencies on other steps using `depends_on`, ensuring they run in the correct order.
* **Parallel Execution**: Independent steps in the pipeline are automatically executed in parallel to reduce build times.
* **Failure Tolerance**: You can configure individual steps to not halt the entire pipeline on failure by setting `ignore_failure: true`.
* **Timeouts**: Pipelines can have a global `timeout` to prevent them from running indefinitely.
* **Custom Environment Variables**: Define custom environment variables at the pipeline level that are available to all steps.
* **LLM Content Control**: Fine-grained control over whether the LLM can access file contents (`llm_content_sharing`) or see the output of previous steps (`llm_output_sharing`) at both the pipeline and per-step level.

---
### Secret Management

* **Self-Hosted & Secure**: Nopsai includes a built-in, self-hosted secret management system that stores secrets encrypted (using AES-256-GCM) in its database.
* **Hierarchical Scopes**: Secrets can be defined at two levels:
    * **General**: Available to all pipelines.
    * **Repository-level**: Specific to a single repository and will override a general secret of the same name.
* **Scoped Injection**: A step must explicitly declare which secrets it needs via a `secrets` block. The agent will only inject the requested secrets into that specific step's environment.
* **Fail-Fast Design**: The pipeline will fail immediately before starting if a step requests a secret that is not defined, preventing unexpected failures during a run.
* **Log Masking**: The agent automatically redacts secret values from all logs, preventing accidental exposure.

---
### GitHub Integration & Overrides

* **Git-Based Triggers**: Pipelines are triggered by standard Git events like `push` and `pull_request` based on rules defined in a `.nopsai/triggers.yaml` file in the repository.
* **Configurable UI Views**: You can choose how pipeline status is displayed in the GitHub UI using `display_options`:
    * **`flat`**: A clean, simple list of steps.
    * **`tree`**: A detailed view showing the dependency hierarchy.
    * **`mermaid`**: A rendered MermaidJS graph of the pipeline's dependency flow.
* **Centralized Overrides**: A powerful feature for administrators to enforce specific workflows. You can store both trigger rules and entire pipeline definitions in the Nopsai database.
    * A **Trigger Override** for a repository will ignore the in-repo `.nopsai/triggers.yaml` and use the centrally defined rules.
    * These central triggers point to centrally stored **Pipelines**, allowing you to manage and reuse standard pipelines across many repositories.
    * When an override is used, the pipeline name is automatically appended with `-overridden` in the GitHub UI for clarity.

---
### Future Plans

* **Event-Driven Architecture**: The readme outlines a future vision to move from a synchronous, API-driven system to an asynchronous, event-driven architecture using a message bus (like RabbitMQ or NATS) for even greater resilience and scalability.












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