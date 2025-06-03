// main.go
package main

import (
	"fmt"
	"log"
	"nopsai/config"
	"nopsai/executor"
	"nopsai/llm"
	"nopsai/pipeline"
	"os"
	"strings"
	"sync" // Mutex still needed for stepOutputs if accessed by LLM context prep
)

const defaultGeminiModel = "gemini-1.5-flash"

type StepState string

const (
	StatePending   StepState = "PENDING"
	StateRunning   StepState = "RUNNING"
	StateSucceeded StepState = "SUCCEEDED"
	StateFailed    StepState = "FAILED"
	StateSkipped   StepState = "SKIPPED"
)

type PipelineStepInfo struct {
	Step   *pipeline.Step
	State  StepState
	Output string
	Error  error
}

func main() {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Printf("Warning: Could not load config.yaml: %v. Using defaults and env vars.", err)
		cfg = &config.Config{Verbose: false}
	} else if cfg == nil {
		cfg = &config.Config{Verbose: false}
	}

	apiKey := cfg.GeminiAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set. Please provide it.")
	}

	llmModel := cfg.LLMModelName
	if llmModel == "" {
		llmModel = defaultGeminiModel
		if cfg.Verbose {
			log.Printf("LLM model name not set, defaulting to '%s'", llmModel)
		}
	}
	executionDir := cfg.DefaultExecutionDir
	verbose := cfg.Verbose

	if verbose {
		log.Println("Verbose mode enabled.")
		log.Printf("Using LLM Model: %s", llmModel)
		log.Printf("Default execution directory: %s (current if empty)", executionDir)
	}

	if len(os.Args) < 2 {
		log.Fatal("Usage: nopsai <pipeline_yaml_file>")
	}
	pipelineFilePath := os.Args[1]

	p, err := pipeline.LoadPipeline(pipelineFilePath)
	if err != nil {
		log.Fatalf("Error loading pipeline: %v", err)
	}

	if verbose {
		log.Printf("Successfully loaded pipeline: %s", p.Name)
		log.Printf("Found %d steps.\n", len(p.Steps))
	}

	llmClient := llm.NewClient(apiKey, llmModel)
	stepInfos := make(map[string]*PipelineStepInfo) // Map step name to its info
	stepOutputs := make(map[string]string)          // Map step name to its output
	var outputsMutex sync.Mutex                     // To protect stepOutputs

	// Initialize stepInfos
	for i := range p.Steps {
		stepPtr := &p.Steps[i]
		stepInfos[stepPtr.Name] = &PipelineStepInfo{
			Step:  stepPtr,
			State: StatePending,
		}
	}

	// Perform topological sort to get execution order
	sortedStepNames, err := TopologicalSort(p.Steps, stepInfos)
	if err != nil {
		log.Fatalf("Pipeline validation error: %v", err)
	}

	if verbose {
		log.Println("Execution order determined by topological sort:", sortedStepNames)
	}

	// Execute steps sequentially based on sorted order
	pipelineFailed := false
	for _, stepName := range sortedStepNames {
		si := stepInfos[stepName]

		// If pipeline has already failed critically, mark remaining steps as SKIPPED
		if pipelineFailed {
			si.State = StateSkipped
			si.Error = fmt.Errorf("pipeline halted due to previous critical failure")
			if verbose {
				log.Printf("Step '%s' SKIPPED because pipeline halted.", stepName)
			}
			continue
		}

		si.State = StateRunning

		fmt.Printf("\n--- Processing Step: %s ---\n", si.Step.Name)
		if verbose && si.Step.Prompt != si.Step.Name {
			log.Printf("  Prompt: %s\n", si.Step.Prompt)
		}

		outputsMutex.Lock()
		localPreviousOutputs := make(map[string]string)
		for _, depName := range si.Step.Dependencies {
			if depInfo, ok := stepInfos[depName]; ok {
				if depInfo.State == StateSucceeded {
					if output, found := stepOutputs[depName]; found {
						localPreviousOutputs[depName] = output
					}
				} else if verbose {
					log.Printf("  Dependency '%s' for step '%s' did not succeed (State: %s). Its output will not be included in context.", depName, stepName, depInfo.State)
				}
			}
		}
		outputsMutex.Unlock()

		var contextForLLM strings.Builder
		contextForLLM.WriteString(fmt.Sprintf("Task for current step ('%s'): %s\n", stepName, si.Step.Prompt))
		if len(localPreviousOutputs) > 0 {
			contextForLLM.WriteString("\nRelevant outputs from successfully completed dependent steps are:\n")
			for depName, depOutput := range localPreviousOutputs {
				shortOutput := depOutput
				if len(shortOutput) > 300 {
					shortOutput = shortOutput[:300] + "..."
				}
				contextForLLM.WriteString(fmt.Sprintf("- Output of step '%s':\n%s\n", depName, shortOutput))
			}
		}

		llmContext := llm.PromptContext{
			CurrentStepPrompt: contextForLLM.String(),
		}

		generatedCode, err := llmClient.GenerateCodeForStep(llmContext, verbose)
		if err != nil {
			si.Error = fmt.Errorf("LLM code generation failed: %w", err)
			si.State = StateFailed
			log.Printf("  Step '%s': Status: %s - %v\n", stepName, si.State, si.Error)
			if !si.Step.IgnoreFailure {
				log.Printf("Critical step '%s' failed. Stopping pipeline.", stepName)
				pipelineFailed = true
			} else {
				log.Printf("Step '%s' failed but IgnoreFailure is true. Continuing pipeline.", stepName)
			}
			continue
		}

		if verbose {
			log.Printf("  LLM Generated Code for '%s':\n%s\n", stepName, generatedCode)
		}

		var output string
		var execErr error
		if generatedCode != "" {
			output, execErr = executor.ExecuteCommand(executionDir, "bash", verbose, "-c", generatedCode)
		} else {
			execErr = fmt.Errorf("LLM returned empty code for step '%s'", stepName)
		}

		outputsMutex.Lock()
		stepOutputs[stepName] = output
		si.Output = output
		outputsMutex.Unlock()

		if execErr != nil {
			si.Error = execErr
			si.State = StateFailed
		} else {
			si.State = StateSucceeded
		}

		log.Printf("  Step '%s': Finished with Status: %s\n", stepName, si.State)
		if si.Error != nil && verbose {
			log.Printf("  Step '%s': Error details: %v\n", stepName, si.Error)
		}
		if output != "" && verbose {
			fmt.Printf("  Step '%s': Result:\n%s\n", stepName, output)
		}

		if si.State == StateFailed && !si.Step.IgnoreFailure {
			log.Printf("Critical step '%s' failed. Stopping pipeline.", stepName)
			pipelineFailed = true
		}
	}

	fmt.Println("\n--- Pipeline Execution Summary ---")
	for _, stepCfg := range p.Steps {
		si := stepInfos[stepCfg.Name]
		fmt.Printf("Step: %s, Status: %s\n", si.Step.Name, si.State)
		if si.Error != nil {
			fmt.Printf("  Error: %v\n", si.Error)
		}
		if si.Output != "" {
			summaryOutput := si.Output
			if len(summaryOutput) > 200 {
				summaryOutput = summaryOutput[:200] + "..."
			}
			fmt.Printf("  Output Preview: %s\n", summaryOutput)
		}
	}
	fmt.Println("\nPipeline processing finished.")
}

func TopologicalSort(steps []pipeline.Step, stepInfos map[string]*PipelineStepInfo) ([]string, error) {
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	for _, step := range steps {
		if _, ok := inDegree[step.Name]; !ok {
			inDegree[step.Name] = 0
		}
		graph[step.Name] = []string{}
	}

	for _, step := range steps {
		for _, depName := range step.Dependencies {
			if _, ok := stepInfos[depName]; !ok {
				return nil, fmt.Errorf("step '%s' has an unknown dependency '%s'", step.Name, depName)
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
		var cyclePath []string
		visited := make(map[string]bool)
		recursionStack := make(map[string]bool)
		var findCycle func(node string) bool
		findCycle = func(node string) bool {
			visited[node] = true
			recursionStack[node] = true
			cyclePath = append(cyclePath, node)
			for _, neighbor := range graph[node] {
				// Check if neighbor is part of the problem (still has in-degree in the context of cycle detection)
				// or if it leads to a cycle via unvisited path or is already in recursion stack.
				if inDegree[neighbor] > 0 || (!visited[neighbor] && findCycle(neighbor)) || recursionStack[neighbor] {
					if recursionStack[neighbor] { // Cycle detected
						// Find the start of the cycle in cyclePath
						for i, pNode := range cyclePath {
							if pNode == neighbor {
								cyclePath = cyclePath[i:]
								return true
							}
						}
						return true // Should have found it
					}
					if !visited[neighbor] { // If not visited, explore
						if findCycle(neighbor) {
							return true
						}
					}
				}
			}
			// Backtrack: remove node from current path and recursion stack
			idx := -1
			for i, pNode := range cyclePath {
				if pNode == node {
					idx = i
					break
				}
			}
			if idx != -1 {
				cyclePath = cyclePath[:idx]
			}
			recursionStack[node] = false
			return false
		}

		// Attempt to find a cycle starting from nodes that couldn't be processed
		for nodeName := range graph { // Iterate all nodes to ensure all components are checked
			if !visited[nodeName] && inDegree[nodeName] > 0 { // Start DFS from unvisited nodes that are part of the remaining graph
				cyclePath = []string{} // Reset for each new DFS traversal
				if findCycle(nodeName) {
					return nil, fmt.Errorf("cycle detected in pipeline dependencies. Involved steps might include: %v", cyclePath)
				}
			}
		}
		// If no specific cycle path was found but sort failed, it's still a cycle.
		return nil, fmt.Errorf("cycle detected in pipeline dependencies (could not sort all steps). Please check your 'dependencies' fields.")
	}
	return sortedOrder, nil
}
