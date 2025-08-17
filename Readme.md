
protoc --go_out=. --go-grpc_out=. pkg/proto/agent.proto
mv nopsai/pkg/proto/* pkg/proto/ && rm -rf nopsai/

https://github.com/settings/apps/nopsai

docker-compose up --build -d

curl http://localhost:8080/v1/secrets

curl -X PUT -H "Content-Type: application/x-yaml" --data-binary "@.nopsai/main-pipeline.yaml" http://localhost:8080/v1/pipelines/main-pipeline.yaml

curl http://localhost:8080/v1/pipelines/main-production.yaml

curl http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/hosein-yousefii/test-app



Nopsai: The LLM-Powered CI/CD System
Nopsai is a modern, microservice-based CI/CD system that leverages the power of Large Language Models (LLMs) to orchestrate and execute complex pipelines. Instead of relying on rigid, pre-defined scripts, Nopsai uses high-level, natural language "goals" which are translated into executable commands by an AI agent at runtime. This approach provides a flexible, intelligent, and powerful automation platform.

Key Features
Natural Language Pipelines: Define your pipeline steps using simple, human-readable goals (e.g., "build the docker image and push it to the registry").

Per-Step Container Images: Each step can run in its own dedicated container image, allowing you to use the perfect environment and toolset for every task.

Parallel Step Execution: Independent steps in your pipeline are automatically executed in parallel, dramatically reducing your build times.

Persistent & Hybrid Execution: A main pipeline container is kept running for speed, while custom-image steps are executed in their own dedicated containers, giving you the best of both worlds.

Configurable GitHub UI: Choose between a clean, flat list or a detailed dependency tree view for your pipeline status right in the GitHub interface.

Extensible & Microservice-Based: The system is built on a decoupled, microservice architecture, making it resilient, scalable, and easy to extend.

Architecture
The core of Nopsai is a powerful "Agent as a Service" model. For each pipeline run, a dedicated, ephemeral agent service is launched to act as the self-contained orchestrator for that specific job. This design makes the system highly scalable and resilient.

Service Roles
The system is composed of four core microservices, each with a distinct and focused responsibility:

Service	Description
nopsai	The main API gateway and database service. It receives pipeline definitions, manages the lifecycle of agent containers, and records pipeline status.
agent	An ephemeral service that manages a single pipeline run. It communicates with the llm-agent to get instructions and executes them in the appropriate container.
llm-agent	An AI-powered service that translates the high-level goals from the agent into specific, executable actions using the Gemini LLM.
git-bot	Integrates with GitHub to listen for webhooks, trigger pipeline runs, and update the status of check runs with a professional tree or flat view.

Export to Sheets
Getting Started
Prerequisites
Docker and Docker Compose

A Gemini API Key

A GitHub App set up for the git-bot integration

Installation
Clone the repository:

Bash

git clone https://github.com/hosein-yousefii/pre-nopsai.git
cd pre-nopsai
Configure your environment:

Copy the .env.example file to .env and fill in the required values, including your GEMINI_API_KEY, database credentials, and GitHub App details.

Run the application:

Bash

docker-compose up --build
This will build the container images and start all the necessary services.

How to Use
Triggering a Pipeline via API
To start a pipeline, submit the YAML definition to the nopsai service's /v1/run endpoint.

Bash

curl -X POST \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@path/to/your/pipeline.yaml" \
  http://localhost:8080/v1/run
Triggering a Pipeline via GitHub
You can also trigger a pipeline by pushing to a configured GitHub repository. The git-bot service listens for incoming webhooks and will automatically start a pipeline run. For more details on setting up the webhook, see doc/triggering.md.

Pipeline Configuration
Pipelines are defined in a .nopsai.yaml file in the root of your repository. Here is a full example:

YAML

name: My-Awesome-Pipeline
description: An example pipeline with all features.
container_image: "pipeline-image:latest" # Default image for all steps
display_options:
  github_view: "tree" # Options: "tree" or "flat"

steps:
  - name: build
    script: echo "Building the application..."

  - name: test
    image: "golang:1.21" # Custom image for this step
    script: echo "Running tests in a Go environment..."
    depends_on:
      - build

  - name: security-scan
    goal: "run a security scan on the source code"
    depends_on:
      - build

  - name: deploy
    goal: "deploy the application to production"
    depends_on:
      - test
      - security-scan
Future Plans
The most impactful change to elevate the architecture is to move from a synchronous, API-call-based system to an asynchronous, event-driven architecture. This will be achieved by introducing a message bus (like RabbitMQ, NATS, or Kafka) as the central nervous system for all services. This will improve resilience, scalability, and further decouple the microservices.

Contributing
Contributions are welcome! Please feel free to submit a pull request or open an issue to discuss your ideas.

License
This project is licensed under the MIT License. See the LICENSE file for details.



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