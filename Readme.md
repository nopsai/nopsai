
# NopsAI

### Core Philosophy & Architecture

* **LLM-Powered Pipelines**: Instead of rigid scripts, Nopsai uses high-level, natural language "goals" (e.g., "build the docker image") which are translated into executable commands by an AI agent at runtime.
* **Microservice-Based**: The system is built on a decoupled, microservice architecture, making it resilient, scalable, and easy to extend. The core services are:
    * **`nopsai`**: The main API gateway that manages pipeline runs, agent lifecycles, and the central database.
    * **`agent`**: An ephemeral, per-run service that orchestrates a single pipeline and talks directly to Gemini to resolve LLM-driven steps.
    * **`git-bot`**: Integrates with GitHub to receive webhooks, forward events to `nopsai`, expose GitHub adapter APIs (files, pipelines, check runs), and publish status updates.
* **Agent as a Service Model**: For each pipeline run, a dedicated, ephemeral agent is launched to act as a self-contained orchestrator, making the system highly scalable and resilient.
* **Immediate Status Reporting**: The agent reports the final pipeline status the moment all tasks are complete, providing rapid feedback in the UI before post-run cleanup begins.

---
### Pipeline & Step Configuration

* **Declarative Variables**: A pipeline's YAML file declares a list of required variables under the `variables` key. It does not define the values, creating a clean separation between the pipeline's logic and its configuration.
* **Natural Language or Scripts**: Steps can be defined using a simple `goal` in natural language or a traditional `script` for direct command execution.
* **Per-Step Container Images**: Each step can run in its own dedicated container image, allowing you to use the perfect environment and toolset for every task. A default image can be set for the entire pipeline.
* **Persistent Volume Mounting**: Steps can define `volumes` to be mounted, allowing for data persistence and sharing across runs. The agent automatically creates any specified volumes that do not already exist.
* **Dependency Management**: Steps can define dependencies on other steps using `depends_on`, ensuring they run in the correct order.
* **Parallel Execution**: Independent steps in the pipeline are automatically executed in parallel to reduce build times.
* **Failure Tolerance**: You can configure individual steps to not halt the entire pipeline on failure by setting `ignore_failure: true`.
* **Timeouts**: Pipelines can have a global `timeout` to prevent them from running indefinitely.
* **LLM Context Control**: Fine-grained control over whether the LLM can access file contents (`llm_content_sharing`) or see the output of previous steps (`llm_output_sharing`) at both the pipeline and per-step level.
* **Informative Container Naming**: Agent and step containers are now given descriptive names for easier debugging and monitoring, using the format `agent-<repo>-<pipeline>-<run_id>` and `<repo>-<pipeline>-<step>-<run_id>` respectively.

---
### Environment & Secret Management

Nopsai features a powerful, hierarchical system for managing both secrets and plaintext environment variables across different environments (`dev`, `prod`, etc.).

* **Self-Hosted & Secure**: Nopsai includes a built-in, self-hosted management system. Secrets are stored encrypted (using AES-256-GCM) in its database, while environment variables are stored in plaintext.
* **Four-Layer Hierarchy**: The system uses a strict, four-layer hierarchy to resolve the value of any required variable, ensuring that specific contexts always override general ones. The layers are:
    1.  Repository-specific, for a specific scope (e.g., `prod` secret for `my-org/my-repo`).
    2.  General, for a specific scope (e.g., a global `prod` secret).
    3.  Repository-specific, with no scope (falls back to the default scope).
    4.  General, with no scope.
* **Strict Scope Isolation**: Scopes are treated as isolated contexts. A trigger for a specific scope (e.g., `prod`) will **only** resolve variables tagged for that scope. It will **never** fall back to an unscoped value, preventing accidental configuration leaks. If a required variable is not found for the specified scope, the pipeline will fail immediately.
* **Scoped Injection**: A step must explicitly declare which secrets it needs via a `secrets` block. The agent will only inject the requested secrets into that specific step's environment.
* **Log Masking**: The agent automatically redacts secret values from all logs, preventing accidental exposure.

---
### GitHub Integration & Overrides

* **Git-Based Triggers**: Pipelines are triggered by standard Git events like `push` and `pull_request` based on rules defined in a `.nopsai/triggers.yaml` file. This file is also where you specify the `scope` for a given trigger.
* **Configurable UI Views**: You can choose how pipeline status is displayed in the GitHub UI with accurately rendered dependency graphs using `display_options`:
    * **`flat`**: A clean, simple list of steps.
    * **`tree`**: A detailed, correctly ordered dependency tree showing the execution hierarchy.
    * **`mermaid`**: A rendered MermaidJS graph of the pipeline's true dependency flow, from start to finish.
* **Centralized Overrides**: A powerful feature for administrators to enforce specific workflows. You can store both trigger rules and entire pipeline definitions in the Nopsai database.
    * A **Trigger Override** for a repository will ignore the in-repo `.nopsai/triggers.yaml` and use the centrally defined rules.
    * These central triggers point to centrally stored **Pipelines**, allowing you to manage and reuse standard pipelines across many repositories.
    * When an override is used, the pipeline name is automatically appended with `-overridden` in the GitHub UI for clarity.
* **Pipeline-in-Pipeline Context Sharing**: When an `include` step is used to run a child pipeline, the parent agent now securely passes a snapshot of its execution history. This gives the child pipeline's LLM full context of what has already occurred, allowing for more intelligent, context-aware actions.

---
### Future Plans

* **Event-Driven Architecture**: The readme outlines a future vision to move from a synchronous, API-driven system to an asynchronous, event-driven architecture using a message bus (like RabbitMQ or NATS) for even greater resilience and scalability.
