package scheduler

import (
	"fmt"
	"sort"
	"strings"

	"nopsai/pkg/models"
)

type RunnableTask struct {
	Step      *models.PipelineStep
	Task      *models.Task
	GlobalKey string
}

func NextRunnableTasks(pipeline *models.Pipeline, completedTasks map[string]bool) []*RunnableTask {
	var runnableTasks []*RunnableTask

	completedSteps := make(map[string]bool)
	stepTaskCounts := make(map[string]int)
	completedStepTaskCounts := make(map[string]int)

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		tasks := step.GetTasks()
		if len(tasks) > 0 {
			stepTaskCounts[stepName] = len(tasks)
		} else {
			stepTaskCounts[stepName] = 1
		}
	}

	for taskKey := range completedTasks {
		parts := strings.Split(taskKey, "/")
		stepName := parts[0]
		completedStepTaskCounts[stepName]++
	}

	for stepName, totalTasks := range stepTaskCounts {
		if completedStepTaskCounts[stepName] == totalTasks {
			completedSteps[stepName] = true
		}
	}

	for i := range pipeline.Steps {
		step := &pipeline.Steps[i]
		stepName := step.GetName()

		stepDependenciesMet := true
		for _, depStepName := range step.GetDependsOn() {
			if !completedSteps[depStepName] {
				stepDependenciesMet = false
				break
			}
		}
		if !stepDependenciesMet {
			continue
		}

		tasksToCheck := []*models.Task{}
		if taskStep, ok := step.AsTaskStep(); ok && len(taskStep.Tasks) > 0 {
			for j := range taskStep.Tasks {
				tasksToCheck = append(tasksToCheck, &taskStep.Tasks[j])
			}
		} else {
			tasksToCheck = append(tasksToCheck, &models.Task{
				Name:      stepName,
				Goal:      step.GetGoal(),
				Script:    step.GetScript(),
				DependsOn: []string{},
			})
		}

		for _, task := range tasksToCheck {
			globalKey := fmt.Sprintf("%s/%s", stepName, task.Name)
			if completedTasks[globalKey] {
				continue
			}

			taskDependenciesMet := true
			for _, depTaskName := range task.DependsOn {
				depGlobalKey := fmt.Sprintf("%s/%s", stepName, depTaskName)
				if !completedTasks[depGlobalKey] {
					taskDependenciesMet = false
					break
				}
			}

			if taskDependenciesMet {
				runnableTasks = append(runnableTasks, &RunnableTask{
					Step:      step,
					Task:      task,
					GlobalKey: globalKey,
				})
			}
		}
	}

	return runnableTasks
}

func CountPipelineTasks(pipeline *models.Pipeline) int {
	totalTasks := 0
	if pipeline == nil {
		return totalTasks
	}

	for _, step := range pipeline.Steps {
		if tasks := step.GetTasks(); len(tasks) > 0 {
			totalTasks += len(tasks)
		} else {
			totalTasks++
		}
	}
	return totalTasks
}

func ImagePullQueue(pipeline *models.Pipeline, totalTasks int) []string {
	queue := make([]string, 0)
	if pipeline == nil || totalTasks == 0 {
		return queue
	}

	seen := make(map[string]bool)
	simulatedCompleted := make(map[string]bool)

	for len(simulatedCompleted) < totalTasks {
		runnable := NextRunnableTasks(pipeline, simulatedCompleted)
		if len(runnable) == 0 {
			break
		}

		for _, r := range runnable {
			if _, ok := r.Step.AsApprovalStep(); ok {
				simulatedCompleted[r.GlobalKey] = true
				continue
			}
			image := r.Step.GetImage()
			if image == "" {
				image = pipeline.ContainerImage
			}

			if image != "" && !seen[image] {
				queue = append(queue, image)
				seen[image] = true
			}
			simulatedCompleted[r.GlobalKey] = true
		}
	}

	return queue
}

func CompletedTaskKeysSnapshot(completedTasks map[string]bool) []string {
	keys := make([]string, 0, len(completedTasks))
	for key, done := range completedTasks {
		if done {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func FirstApprovalRunnable(runnableTasks []*RunnableTask) *RunnableTask {
	for _, runnable := range runnableTasks {
		if runnable == nil || runnable.Step == nil {
			continue
		}
		if _, ok := runnable.Step.AsApprovalStep(); ok {
			return runnable
		}
	}
	return nil
}
