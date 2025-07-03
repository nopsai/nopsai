package main

import (
	"flag"
	"fmt"
	"log"
	"nopsai/config"
	"nopsai/executor"
	"nopsai/llm"
	"nopsai/pipeline"
	"os"
	"regexp"
	"strings"
	"sync"
)

type StepState string

const (
	StatePending      StepState = "PENDING"
	StateRunning      StepState = "RUNNING"
	StateSucceeded    StepState = "SUCCEEDED"
	StateFailed       StepState = "FAILED"
	StateSkipped      StepState = "SKIPPED"
	StateUserRejected StepState = "USER_REJECTED"
)

type PlannedExecutionStep struct {
	Name          string
	Prompt        string
	Dependencies  []string
	IgnoreFailure bool
	State         StepState
	Output        string
	Error         error
}

var (
	configPath   string
	pipelinePath string
)

func main() {
	flag.StringVar(&configPath, "c", "./config.yaml", "Path to the configuration YAML file.")
	flag.StringVar(&pipelinePath, "p", "", "Path to the pipeline YAML file. (Required)")
	flag.Parse()

	if pipelinePath == "" {
		log.Println("Error: Pipeline file path must be provided via the -p flag.")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading configuration from %s: %v", configPath, err)
	}

	if cfg.Verbose {
		log.Println("verbose mode enabled")
	}

	userPipeline, err := pipeline.LoadPipeline(pipelinePath)
	if err != nil {
		log.Fatalf("error loading user pipeline: %v", err)
	}

	if cfg.Verbose {
		log.Printf("successfully loaded pipeline: %s, with %d steps\n", userPipeline.Name, len(userPipeline.Steps))
	}

	llmClient, err := llm.NewClient(cfg.GeminiAPIKey, cfg.LLMModelName, cfg.LLMMaxOutputTokens, cfg.LLMTemperature)
	if err != nil {
		log.Fatalf("Error creating LLM client: %v", err)
	}

	var planningContext strings.Builder
	for i, step := range userPipeline.Steps {
		planningContext.WriteString(fmt.Sprintf("Step %d - Name: '%s', Prompt: \"%s\", Dependencies: %v, IgnoreFailure: %t\n",
			i+1, step.Name, step.Prompt, step.Dependencies, step.IgnoreFailure))
	}

	llmExecutionPlan, err := llmClient.GenerateExecutionPlan(planningContext.String(), cfg.Verbose)
	if err != nil {
		log.Fatalf("LLM failed to generate an execution plan: %v", err)
	}
	if llmExecutionPlan == nil || len(llmExecutionPlan.PlannedSteps) == 0 {
		log.Fatal("LLM returned an empty or invalid execution plan.")
	}

	// if cfg.Verbose {
	// 	log.Println("LLM Execution Plan Received:")
	// 	for i, step := range llmExecutionPlan.PlannedSteps {
	// 		log.Printf("  Planned step %d: Name='%s', Description='%s'",
	// 			i+1, step.Name, step.Description)
	// 	}
	// }

	var currentExecutor executor.Executor
	switch strings.ToLower(cfg.ExecutorRuntime) {
	case "docker":
		currentExecutor = executor.DockerExecutor(executor.DockerCLIRuntime())
	case "local":
		currentExecutor = executor.LocalExecutor()
	default:
		log.Fatalf("Unsupported executor_runtime '%s' in configuration. Supported values are 'docker' or 'local'.", cfg.ExecutorRuntime)
	}

	if cfg.Verbose {
		log.Printf("\n--- Preparing Execution Environment (Executor: %s, Configured: %s) ---",
			currentExecutor.GetType(), cfg.ExecutorRuntime)
	}

	hostWorkspace := userPipeline.WorkspaceMount
	if hostWorkspace == "" {
		hostWorkspace = "."
		if cfg.Verbose {
			log.Printf("pipeline.yaml does not specify 'workspace_mount', defaulting host workspace to current directory '.'")
		}
	}

	pipelineCtx := executor.PipelineContext{
		PipelineName:      userPipeline.Name,
		ImageName:         userPipeline.ContainerImage,
		HostWorkspacePath: hostWorkspace,
		Environment:       userPipeline.Environment,
	}

	if err := currentExecutor.PrepareEnvironment(pipelineCtx, cfg.Verbose); err != nil {
		log.Fatalf("failed to prepare execution environment: %v", err)
	}
	defer func() {
		if err := currentExecutor.CleanupEnvironment(cfg.Verbose); err != nil {
			log.Printf("warning: error during environment cleanup: %v", err)
		}
	}()

	if cfg.Verbose {
		log.Println("\n--- Executing LLM's Planned steps ---")
	}
	plannedStepInfos := make(map[string]*PlannedExecutionStep)
	stepOutputs := make(map[string]string)
	var outputsMutex sync.Mutex

	for i := range llmExecutionPlan.PlannedSteps {
		step := llmExecutionPlan.PlannedSteps[i]
		plannedStepInfos[step.Name] = &PlannedExecutionStep{
			Name:          step.Name,
			Prompt:        step.Prompt,
			Dependencies:  step.Dependencies,
			IgnoreFailure: step.IgnoreFailure,
			State:         StatePending,
		}
	}

	sortedNames, err := TopologicalSortPlannedSteps(llmExecutionPlan.PlannedSteps)
	if err != nil {
		log.Fatalf("pipeline validation error in LLM's plan: %v", err)
	}

	if cfg.Verbose {
		log.Println("Execution order for planned steps:", sortedNames)
	}

	pipelineFailed := false
	for _, Name := range sortedNames {
		psi := plannedStepInfos[Name]

		if pipelineFailed {
			psi.State = StateSkipped
			psi.Error = fmt.Errorf("pipeline halted due to previous critical failure")
			if cfg.Verbose {
				log.Printf("'%s': SKIPPED because pipeline halted.", Name)
			}
			continue
		}

		psi.State = StateRunning
		resolvedPrompt := psi.Prompt
		outputsMutex.Lock()
		placeholderRegex := regexp.MustCompile(`{outputs\.([a-zA-Z0-9_]+)}`)
		resolvedPrompt = placeholderRegex.ReplaceAllStringFunc(resolvedPrompt, func(match string) string {
			parts := placeholderRegex.FindStringSubmatch(match)
			if len(parts) == 2 {
				depName := parts[1]
				if output, ok := stepOutputs[depName]; ok {
					if cfg.Verbose {
						log.Printf("  Substituting placeholder '%s' with output from '%s'", match, depName)
					}
					return strings.TrimSpace(output)
				}
				if cfg.Verbose {
					log.Printf("  Warning: Output for placeholder '%s' (step '%s') not found.", match, depName)
				}
				return match
			}
			return match
		})
		outputsMutex.Unlock()

		// if cfg.Verbose {
		// 	log.Printf("  Resolved Step Prompt (for LLM code gen): %s\n", resolvedPrompt)
		// }

		llmCodeContext := llm.PromptContextForCode{
			PreciseStepPrompt: resolvedPrompt,
		}

		generatedCode, err := llmClient.GenerateCodeForStep(llmCodeContext, cfg.Verbose)
		if err != nil {
			psi.Error = fmt.Errorf("LLM code generation failed for step '%s': %w", Name, err)
			psi.State = StateFailed
			log.Printf("'%s': %s - %v\n", Name, psi.State, psi.Error)
			if !psi.IgnoreFailure {
				log.Printf("'%s': Failed. stopping pipeline", Name)
				pipelineFailed = true
			} else {
				log.Printf("'%s': Failed but IgnoreFailure is true. continuing pipeline", Name)
			}
			continue
		}

		if strings.TrimSpace(generatedCode) == "" {
			psi.Error = fmt.Errorf("LLM generated an empty script for step '%s'", Name)
			psi.State = StateFailed
			log.Printf("'%s': %s - %v\n", Name, psi.State, psi.Error)
			if !psi.IgnoreFailure {
				log.Printf("'%s': Failed (empty script). Stopping pipeline.", Name)
				pipelineFailed = true
			} else {
				log.Printf("'%s': Failed (empty script) but IgnoreFailure is true. Continuing pipeline.", Name)
			}
			continue
		}

		stepCtx := executor.StepContext{
			Name:              psi.Name,
			StepScriptContent: generatedCode,
		}

		execResult := currentExecutor.ExecuteStep(stepCtx, cfg.Verbose)

		outputsMutex.Lock()
		stepOutputs[Name] = execResult.Stdout
		psi.Output = execResult.Stdout
		outputsMutex.Unlock()

		if execResult.Error != nil {
			psi.Error = execResult.Error
			psi.State = StateFailed
		} else if execResult.ExitCode != 0 {
			psi.Error = fmt.Errorf("script failed with exit code %d. Stderr: %s", execResult.ExitCode, execResult.Stderr)
			psi.State = StateFailed
		} else {
			psi.State = StateSucceeded
		}

		log.Printf("'%s': %s\n", Name, psi.State)
		if psi.Error != nil && cfg.Verbose {
			log.Printf("'%s': Failed, details: %v\n", Name, psi.Error)
		}

		if psi.State == StateFailed && !psi.IgnoreFailure {
			log.Printf("'%s': Failed. stopping pipeline", Name)
			pipelineFailed = true
		}
	}

	fmt.Println("\n--- Pipeline Execution Summary ---")
	for _, Name := range sortedNames {
		psi := plannedStepInfos[Name]
		fmt.Printf("%s: %s\n", psi.Name, psi.State)
		if psi.Error != nil {
			fmt.Printf("  Error: %v\n", psi.Error)
		}
		if psi.Output != "" {
			summaryOutput := psi.Output
			if len(summaryOutput) > 200 {
				summaryOutput = summaryOutput[:200] + "..."
			}
			fmt.Printf("  Output Preview: %s\n", summaryOutput)
		}
	}
	if cfg.Verbose {
		userStepsProcessed := make(map[string]bool)
		for _, Name := range sortedNames {
			if psi, ok := plannedStepInfos[Name]; ok && (psi.State == StateSucceeded || psi.State == StateFailed || psi.State == StateSkipped) {
				userStepsProcessed[psi.Name] = true
			}
		}

		// log.Println("--- User Step Summary ---")
		// for _, userStep := range userPipeline.Steps {
		// 	if _, processed := userStepsProcessed[userStep.Name]; processed {
		// 		log.Printf("User Step: %s - Status: Processed (corresponded to one or more planned steps that were executed, failed, or skipped).", userStep.Name)
		// 	} else {
		// 		fmt.Printf("User Step: %s, Status: SKIPPED (no planned steps executed or pipeline halted early)\n", userStep.Name)
		// 	}
		// }
	}

	fmt.Println("\nPipeline processing finished.")
}

func TopologicalSortPlannedSteps(steps []llm.PlannedStep) ([]string, error) {
	inDegree := make(map[string]int)
	graph := make(map[string][]string)
	stepMap := make(map[string]bool)

	for _, step := range steps {
		if step.Name == "" {
			return nil, fmt.Errorf("found a planned step with an empty Name")
		}
		stepMap[step.Name] = true
		if _, ok := inDegree[step.Name]; !ok {
			inDegree[step.Name] = 0
		}
		graph[step.Name] = []string{}
	}

	for _, step := range steps {
		for _, depName := range step.Dependencies {
			if _, ok := stepMap[depName]; !ok {
				return nil, fmt.Errorf("planned step '%s' has an unknown dependency step '%s' in LLM's plan", step.Name, depName)
			}
			graph[depName] = append(graph[depName], step.Name)
			inDegree[step.Name]++
		}
	}

	queue := []string{}
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var sortedOrder []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		sortedOrder = append(sortedOrder, u)

		for _, v := range graph[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	if len(sortedOrder) != len(steps) {
		return nil, fmt.Errorf("cycle detected in LLM's planned step dependencies")
	}
	return sortedOrder, nil
}
