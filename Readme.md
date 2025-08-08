
protoc --go_out=. --go-grpc_out=. pkg/proto/agent.proto
mv nopsai/pkg/proto/* pkg/proto/ && rm -rf nopsai/

docker-compose up --build -d


curl -X POST -H "Content-Type: application/x-yaml" --data-binary "@sample-pipeline/2-pipeline.yaml" http://localhost:8080/v1/run



Final Architecture: Agent as a Service
This model promotes the agent to a first-class, ephemeral service responsible for the entire lifecycle of a single pipeline run. The central system (nopsai) acts as a simple API gateway and a secure proxy to the database, enforcing a clean separation of concerns. This is a powerful and elegant design that is highly scalable and resilient.

Service Roles & Integrations
nopsai (API Gateway & Database Service)
Role: The single, authoritative entry point to the system and the sole guardian of the database.

Responsibilities:

API Gateway: Receives the initial pipeline YAML from the user.

Database Proxy: It is the only service that communicates with PostgreSQL. It exposes a secure internal API for agents to report their status.

Run Initialization: Creates the initial high-level run and step records in the database.

Agent Launcher: Its primary action is to use the Docker API to launch a new, dedicated agent container for the pipeline run, injecting all necessary configuration.

Integrations:

Receives requests from the User.

Launches the agent container using the Docker Host.

Receives status updates (e.g., "step completed," "run failed") from the agent and writes them to the Database.

agent (The Run Orchestrator)
Role: An ephemeral, single-purpose service that manages one and only one pipeline run from start to finish.

Responsibilities:

Resource Management: On startup, it uses the Docker API to provision all necessary resources:

A shared Docker volume for the workspace.

The pipeline container itself, attached to the shared volume.

State Management: It holds the entire execution history and a live view of the file system in its own memory. It is the source of truth for the live run.

Orchestration Loop: It manages the step-by-step execution, determines the next step based on dependencies, and calls the llm-agent for instructions.

Execution: It uses docker exec to run commands inside the sibling pipeline container.

Cleanup: After the pipeline finishes (succeeds, fails, or times out), it is responsible for stopping and removing the pipeline container before terminating itself.

Integrations:

Is launched by nopsai.

Communicates with the Docker Host to manage containers and volumes.

Sends requests to the llm-agent to get actions.

Sends status updates back to the nopsai service's API.

llm-agent (AI Specialist)
Role: A pure, stateless function that translates a goal into a concrete action. It has no knowledge of the wider system.

Responsibilities:

Receives a self-contained context bundle from the agent (goal, in-memory history, live file snapshot).

Builds a prompt and queries the Gemini LLM.

Returns a single, structured Action back to the agent.

Integrations:

Only receives requests from the agent containers. It does not connect to the database or any other service.