
protoc --go_out=. --go-grpc_out=. pkg/proto/agent.proto
mv nopsai/pkg/proto/* pkg/proto/ && rm -rf nopsai/

docker-compose up --build -d


curl -X POST -H "Content-Type: application/x-yaml" --data-binary "@sample-pipeline/2-pipeline.yaml" http://localhost:8080/v1/run



Nopsai: An LLM-Powered CI/CD System
Nopsai is a modern, microservice-based CI/CD system that leverages the power of Large Language Models (LLMs) to orchestrate and execute complex pipelines. Instead of relying on rigid, pre-defined scripts, Nopsai uses high-level, natural language "goals" which are translated into executable commands by an AI agent at runtime.

This document provides an overview of the system's architecture, the role of each service, and how they integrate to create a flexible and powerful automation platform.

🏛️ Final Architecture: Agent as a Service
The core of Nopsai is a powerful and elegant "Agent as a Service" model. For each pipeline run, a dedicated, ephemeral agent service is launched to act as the self-contained orchestrator for that specific job. This design makes the system highly scalable, resilient, and decoupled.

Architectural Flow
Initiation: A User submits a pipeline YAML to the nopsai service.

Launch: nopsai creates the initial records in the Database and then uses the Docker Host to launch a new, dedicated agent container for that specific run.

Provisioning: The agent starts, connects to the Docker Host, and provisions all necessary resources: a shared volume and the pipeline container.

Orchestration Loop:

The agent holds the pipeline state in memory and determines the next step based on dependencies.

It sends a request with the full context (goal, history, files) to the llm-agent.

The llm-agent returns a specific Action.

The agent executes this Action inside the pipeline container using docker exec.

Status Reporting: After each step, the agent sends a stateless update to the nopsai service, which records the result in the Database.

Cleanup: When the pipeline finishes (succeeds, fails, or times out), the agent cleans up the pipeline container and then terminates itself. The nopsai service then cleans up the agent's volume and, if configured, the agent container itself.

🛠️ Service Roles & Integrations
The system is composed of three core microservices, each with a distinct and focused responsibility.

nopsai (API Gateway & Database Service)
The nopsai service is the single, authoritative entry point to the system and the sole guardian of the database.

Role: API Gateway & Database Proxy

Key Responsibilities:

Receives Pipeline Definitions: It exposes an HTTP endpoint (POST /v1/run) that accepts a pipeline definition in YAML format from the user.

Initializes Runs: It creates the high-level run and step records in the PostgreSQL database, including the final timeout_at timestamp.

Launches Agents: Its primary function is to use the Docker API to launch a new, dedicated agent container for each pipeline run, injecting all necessary configuration via environment variables.

Receives Status Updates: It provides an internal API endpoint (POST /v1/runs/{runID}/steps/{stepName}) for agents to report back the final status of each step.

Integrations:

Receives requests from the User.

Launches and manages the lifecycle of the agent container using the Docker Host.

Is the only service that connects to the PostgreSQL Database.

agent (The Run Orchestrator)
The agent is an ephemeral, single-purpose service that manages one pipeline run from start to finish. It is the "brain" of a live run.

Role: Stateful Orchestrator for a Single Run

Key Responsibilities:

Resource Management: On startup, it uses the Docker API to provision all necessary resources: a shared Docker volume and the pipeline container itself.

State Management: It holds the entire execution history and a live view of the file system in its own memory, ensuring the most up-to-date context.

Orchestration Loop: It manages the step-by-step execution, determining the next step based on dependencies, and calls the llm-agent for instructions.

Execution: It uses docker exec to run commands inside its sibling pipeline container, ensuring a clean separation between orchestration and the execution environment.

Timeout Enforcement: It is responsible for its own lifecycle and will shut down if the pipeline's configured timeout is reached.

Cleanup: After the pipeline finishes, it is responsible for stopping and removing the pipeline container before terminating itself.

Integrations:

Is launched by the nopsai service.

Communicates with the Docker Host to manage containers and volumes.

Sends requests to the llm-agent to get actions.

Sends status updates back to the nopsai service's API.

llm-agent (AI Specialist)
The llm-agent is a pure, stateless function that translates a high-level goal into a concrete, executable action.

Role: AI-Powered Action Generator

Key Responsibilities:

Receives a self-contained context bundle from the agent (the goal, in-memory history, live file snapshot, and environment variables).

Builds a detailed prompt and queries the configured Gemini LLM.

Returns a single, structured Action (e.g., EXECUTE_COMMAND, REPLACE_FILE) back to the agent.

Integrations:

Only receives gRPC requests from agent containers. It has no knowledge of the wider system and does not connect to the database.

🚀 How to Run a Pipeline
To start a pipeline, submit the YAML definition to the nopsai service's /v1/run endpoint.

Bash

curl -X POST \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@path/to/your/pipeline.yaml" \
  http://localhost:8080/v1/run