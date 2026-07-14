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

func TestRepositoryTeamIDFromMatchesUnderPath(t *testing.T) {
	matches := []repositoryTeamMatch{
		{ID: 42, Path: "team-1/nopsai-config-gitlab", RepositoryFullName: "acme/api"},
		{ID: 7, Path: "team-2/nopsai-config-gitlab", RepositoryFullName: "acme/api"},
	}

	got, found := repositoryTeamIDFromMatchesUnderPath(matches, "team-1")
	if !found || !got.Valid || got.Int32 != 42 {
		t.Fatalf("repositoryTeamIDFromMatchesUnderPath(team-1) = (%#v, %t), want id 42", got, found)
	}

	got, found = repositoryTeamIDFromMatchesUnderPath(matches, "team-3")
	if found || got.Valid {
		t.Fatalf("repositoryTeamIDFromMatchesUnderPath(team-3) = (%#v, %t), want not found", got, found)
	}
}
