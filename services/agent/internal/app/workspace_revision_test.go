package app

import (
	"testing"

	"nopsai/pkg/proto"
)

func TestActionMayMutateWorkspace(t *testing.T) {
	tests := []struct {
		name   string
		action *proto.Action
		want   bool
	}{
		{name: "nil", action: nil, want: false},
		{name: "answer", action: &proto.Action{Type: "RETURN_ANSWER", Payload: &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{Answer: "ok"}}}, want: false},
		{name: "command", action: &proto.Action{Type: "EXECUTE_COMMAND", Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: "touch file"}}}, want: true},
		{name: "file", action: &proto.Action{Type: "REPLACE_FILE", Payload: &proto.Action_FileAction{FileAction: &proto.FileAction{Path: "file", Content: "ok"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := actionMayMutateWorkspace(tt.action); got != tt.want {
				t.Fatalf("actionMayMutateWorkspace() = %t, want %t", got, tt.want)
			}
		})
	}
}
