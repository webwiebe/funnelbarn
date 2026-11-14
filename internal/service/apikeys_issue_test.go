package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
)

func TestAPIKeyService_IssueAPIKey_StoresTheHashOfTheReturnedKey(t *testing.T) {
	ctx := context.Background()
	store := mock.New()
	projects := NewProjectService(store)
	svc := NewAPIKeyService(store)
	p, err := projects.CreateProject(ctx, "Issue", "issue")
	require.NoError(t, err)

	key, plaintext, err := svc.IssueAPIKey(ctx, "ci", p.ID, domain.APIKeyScopeIngest)
	require.NoError(t, err)
	assert.Len(t, plaintext, 64, "32 random bytes, hex encoded")
	assert.NotEmpty(t, key.ID)
	assert.Equal(t, "ci", key.Name)
	assert.Equal(t, domain.APIKeyScopeIngest, key.Scope)

	sum := sha256.Sum256([]byte(plaintext))
	projectID, scope, found, err := svc.ValidAPIKeySHA256(ctx, hex.EncodeToString(sum[:]))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, p.ID, projectID)
	assert.Equal(t, domain.APIKeyScopeIngest, scope)

	_, second, err := svc.IssueAPIKey(ctx, "ci2", p.ID, domain.APIKeyScopeIngest)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, second)
}

func TestAPIKeyService_IssueAPIKey_RejectsInvalidInput(t *testing.T) {
	svc := NewAPIKeyService(mock.New())
	for name, args := range map[string][3]string{
		"empty name":    {"", "p", "ingest"},
		"empty project": {"n", "", "ingest"},
		"bad scope":     {"n", "p", "superadmin"},
	} {
		_, plaintext, err := svc.IssueAPIKey(context.Background(), args[0], args[1], args[2])
		require.Error(t, err, name)
		assert.True(t, domain.IsValidation(err), name)
		assert.Empty(t, plaintext, name)
	}
}

func TestAPIKeyService_IssueAPIKey_ReportsARandomSourceFailure(t *testing.T) {
	boom := errors.New("entropy exhausted")
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, boom }
	t.Cleanup(func() { randRead = orig })

	_, plaintext, err := NewAPIKeyService(mock.New()).IssueAPIKey(context.Background(), "n", "p", "ingest")
	var genErr *KeyGenerationError
	require.ErrorAs(t, err, &genErr)
	assert.Same(t, boom, genErr.Err)
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, boom.Error(), err.Error())
	assert.Empty(t, plaintext)
}
