package nopsai

import "testing"

func TestRepositoryTeamIDFromMatches(t *testing.T) {
	if got := repositoryTeamIDFromMatches(nil); got.Valid {
		t.Fatalf("repositoryTeamIDFromMatches(nil) = %#v, want invalid", got)
	}

	got := repositoryTeamIDFromMatches([]repositoryTeamMatch{
		{ID: 42, Path: "payments/acme/api", RepositoryFullName: "acme/api"},
		{ID: 7, Path: "acme/api", RepositoryFullName: "acme/api"},
	})
	if !got.Valid || got.Int32 != 42 {
		t.Fatalf("repositoryTeamIDFromMatches() = %#v, want valid id 42", got)
	}
}
