package mock

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func (s *Store) FlagContextKeySuggestions(_ context.Context, _ string) ([]repository.ContextKeySuggestion, error) {
	return nil, nil
}

func (s *Store) AnonymizeSessionGeo(_ context.Context, _ string) error { return nil }

func (s *Store) AnonymizeSessionsByIP(_ context.Context, _ string) (int64, error) { return 0, nil }

func (s *Store) GetInstanceSetting(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (s *Store) SetInstanceSetting(_ context.Context, _, _ string) error { return nil }

func (s *Store) GetAllInstanceSettings(_ context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
