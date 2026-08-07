package nopsai

import "testing"

func TestNormalizeApprovalDecisionCommentRequiresRejectComment(t *testing.T) {
	tests := []struct {
		name     string
		approved bool
		raw      string
		want     string
		wantErr  bool
	}{
		{name: "approve without comment", approved: true, raw: "", want: ""},
		{name: "approve trims comment", approved: true, raw: "  ready  ", want: "ready"},
		{name: "reject with comment", approved: false, raw: "  window closed  ", want: "window closed"},
		{name: "reject without comment", approved: false, raw: "  ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeApprovalDecisionComment(tt.approved, tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if err.Error() != approvalRejectCommentText {
					t.Fatalf("error = %q, want %q", err.Error(), approvalRejectCommentText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("comment = %q, want %q", got, tt.want)
			}
		})
	}
}
