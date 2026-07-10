package nopsai

import "testing"

func TestRepositoryGroupIDFromMatches(t *testing.T) {
	if got := repositoryGroupIDFromMatches(nil); got.Valid {
		t.Fatalf("repositoryGroupIDFromMatches(nil) = %#v, want invalid", got)
	}

	got := repositoryGroupIDFromMatches([]repositoryGroupMatch{
		{ID: 42, Path: "payments/acme/api", RepositoryFullName: "acme/api"},
		{ID: 7, Path: "acme/api", RepositoryFullName: "acme/api"},
	})
	if !got.Valid || got.Int32 != 42 {
		t.Fatalf("repositoryGroupIDFromMatches() = %#v, want valid id 42", got)
	}
}
