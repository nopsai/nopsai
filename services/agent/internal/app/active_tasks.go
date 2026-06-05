package app

import (
	"sort"
	"strings"
	"sync"
)

type ActiveTask struct {
	StepName string
	TaskName string
}

type ActiveTaskTracker struct {
	mu    sync.Mutex
	tasks map[string]ActiveTask
}

func NewActiveTaskTracker() *ActiveTaskTracker {
	return &ActiveTaskTracker{tasks: make(map[string]ActiveTask)}
}

func (t *ActiveTaskTracker) Add(stepName, taskName string) {
	if t == nil {
		return
	}
	stepName = strings.TrimSpace(stepName)
	taskName = strings.TrimSpace(taskName)
	if stepName == "" || taskName == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tasks[activeTaskKey(stepName, taskName)] = ActiveTask{StepName: stepName, TaskName: taskName}
}

func (t *ActiveTaskTracker) Remove(stepName, taskName string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tasks, activeTaskKey(stepName, taskName))
}

func (t *ActiveTaskTracker) Clear() []ActiveTask {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	tasks := make([]ActiveTask, 0, len(t.tasks))
	for _, task := range t.tasks {
		tasks = append(tasks, task)
	}
	t.tasks = make(map[string]ActiveTask)

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].StepName == tasks[j].StepName {
			return tasks[i].TaskName < tasks[j].TaskName
		}
		return tasks[i].StepName < tasks[j].StepName
	})
	return tasks
}

func activeTaskKey(stepName, taskName string) string {
	return strings.TrimSpace(stepName) + "/" + strings.TrimSpace(taskName)
}
