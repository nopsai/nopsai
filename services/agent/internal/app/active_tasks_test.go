package app

import "testing"

func TestActiveTaskTrackerTracksAndClearsTasks(t *testing.T) {
	tracker := NewActiveTaskTracker()
	tracker.Add("deploy", "restart")
	tracker.Add("build", "test")
	tracker.Add("deploy", "restart")
	tracker.Remove("build", "test")

	got := tracker.Clear()
	if len(got) != 1 {
		t.Fatalf("tasks = %#v, want one active task", got)
	}
	if got[0].StepName != "deploy" || got[0].TaskName != "restart" {
		t.Fatalf("task = %#v, want deploy/restart", got[0])
	}
	if remaining := tracker.Clear(); len(remaining) != 0 {
		t.Fatalf("remaining = %#v, want empty after clear", remaining)
	}
}

func TestActiveTaskTrackerClearSortsTasks(t *testing.T) {
	tracker := NewActiveTaskTracker()
	tracker.Add("deploy", "restart")
	tracker.Add("build", "lint")
	tracker.Add("build", "test")

	got := tracker.Clear()
	want := []ActiveTask{
		{StepName: "build", TaskName: "lint"},
		{StepName: "build", TaskName: "test"},
		{StepName: "deploy", TaskName: "restart"},
	}
	if len(got) != len(want) {
		t.Fatalf("tasks = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("task[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
