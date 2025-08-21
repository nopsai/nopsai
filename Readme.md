# NopsAI

### Core Philosophy & Architecture

* **LLM-Powered Pipelines**: Instead of rigid scripts, Nopsai uses high-level, natural language "goals" (e.g., "build the docker image") which are translated into executable commands by an AI agent at runtime.
* **Microservice-Based**: The system is built on a decoupled, microservice architecture, making it resilient, scalable, and easy to extend. The core services are:
    * **`nopsai`**: The main API gateway that manages pipeline runs, agent lifecycles, and the central database.
    * **`agent`**: An ephemeral, per-run service that orchestrates a single pipeline, communicating with the LLM to execute steps.
    * **`llm-agent`**: An AI service that uses the Gemini LLM to translate goals into executable shell commands.
    * **`git-bot`**: Integrates with GitHub to handle webhooks, trigger pipelines, and update check run statuses.
* **Agent as a Service Model**: For each pipeline run, a dedicated, ephemeral agent is launched to act as a self-contained orchestrator, making the system highly scalable and resilient.
* **Immediate Status Reporting**: The agent reports the final pipeline status the moment all tasks are complete, providing rapid feedback in the UI before post-run cleanup begins.

---
### Pipeline & Step Configuration

* **Natural Language or Scripts**: Steps can be defined using a simple `goal` in natural language or a traditional `script` for direct command execution.
* **Per-Step Container Images**: Each step can run in its own dedicated container image, allowing you to use the perfect environment and toolset for every task. A default image can be set for the entire pipeline.
* **Persistent Volume Mounting**: Steps can define `volumes` to be mounted, allowing for data persistence and sharing across runs. The agent automatically creates any specified volumes that do not already exist.
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
* **Configurable UI Views**: You can choose how pipeline status is displayed in the GitHub UI with accurately rendered dependency graphs using `display_options`:
    * **`flat`**: A clean, simple list of steps.
    * **`tree`**: A detailed, correctly ordered dependency tree showing the execution hierarchy.
    * **`mermaid`**: A rendered MermaidJS graph of the pipeline's true dependency flow, from start to finish.
* **Centralized Overrides**: A powerful feature for administrators to enforce specific workflows. You can store both trigger rules and entire pipeline definitions in the Nopsai database.
    * A **Trigger Override** for a repository will ignore the in-repo `.nopsai/triggers.yaml` and use the centrally defined rules.
    * These central triggers point to centrally stored **Pipelines**, allowing you to manage and reuse standard pipelines across many repositories.
    * When an override is used, the pipeline name is automatically appended with `-overridden` in the GitHub UI for clarity.

---
### Future Plans

* **Event-Driven Architecture**: The readme outlines a future vision to move from a synchronous, API-driven system to an asynchronous, event-driven architecture using a message bus (like RabbitMQ or NATS) for even greater resilience and scalability.