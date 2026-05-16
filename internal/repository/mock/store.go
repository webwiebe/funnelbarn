package mock

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func (s *Store) FlagContextKeySuggestions(_ context.Context, _ string) ([]repository.ContextKeySuggestion, error) {
	return nil, nil
}
