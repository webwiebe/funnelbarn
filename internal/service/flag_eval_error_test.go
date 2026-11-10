package service_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func TestEvaluateFlag_MissingFlagIsErrFlagNotFound(t *testing.T) {
	svc := service.NewFlagService(mock.New())

	_, err := svc.EvaluateFlag(context.Background(), "proj-1", "absent", map[string]any{})
	require.ErrorIs(t, err, domain.ErrFlagNotFound)
	// A missing flag is a not-found for every caller that maps domain.ErrNotFound.
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestEvalErrorCodeOf(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		code  service.EvalErrorCode
		level slog.Level
		fault bool
	}{
		{"flag not found", fmt.Errorf("read: %w", domain.ErrFlagNotFound), service.EvalFlagNotFound, slog.LevelDebug, false},
		{"auto register limit", fmt.Errorf("project p: %w", domain.ErrAutoRegisterLimit), service.EvalAutoRegisterLimit, slog.LevelWarn, false},
		{"database failure", errors.New("disk I/O error"), service.EvalGeneral, slog.LevelError, true},
		{"text that says flag not found is still general", errors.New("flag not found in cache layer"), service.EvalGeneral, slog.LevelError, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := service.EvalErrorCodeOf(tc.err)
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.level, code.LogLevel())
			require.Equal(t, tc.fault, code.IsFault())
		})
	}
	require.Equal(t, "VARIANT_NOT_FOUND", string(service.EvalVariantNotFound))
	require.True(t, service.EvalVariantNotFound.IsFault())
	require.Equal(t, slog.LevelError, service.EvalVariantNotFound.LogLevel())
}
