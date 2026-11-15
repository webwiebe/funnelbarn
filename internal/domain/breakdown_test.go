package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestIsMetadataColumn(t *testing.T) {
	assert.True(t, domain.IsMetadataColumn("url"))
	assert.True(t, domain.IsMetadataColumn("browser"))
	assert.False(t, domain.IsMetadataColumn("plan"))
	assert.NotEmpty(t, domain.MetadataColumns())
}
