package nopsai

import (
	"context"

	"github.com/google/uuid"

	"nopsai/services/nopsai/internal/credentials"
)

type staticCredentialResolver map[string]string

func (r staticCredentialResolver) Resolve(
	_ context.Context,
	ref credentials.Reference,
	_ credentials.Purpose,
) (credentials.Value, error) {
	value, ok := r[ref.String()]
	if !ok {
		return credentials.Value{}, credentials.ErrNotFound
	}
	return credentials.NewValue(uuid.MustParse("00000000-0000-0000-0000-000000000001"), 1, []byte(value)), nil
}
