package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// The columns list looks at the newest 1000 events of a name. A column only the
// newest event fills is reported, and one only events older than the sample
// fill is not: the report describes what the SDK sends now.
func TestPopulatedMetadataColumns_SamplesTheNewest1000Events(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Populated", "populated")
	require.NoError(t, err)

	const total = 1100
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	for i := 0; i < total; i++ {
		e := repository.Event{
			ID:         fmt.Sprintf("evt-%04d", i),
			ProjectID:  p.ID,
			SessionID:  "sess",
			Name:       "click",
			URL:        "https://example.com/",
			IngestID:   fmt.Sprintf("ing-%04d", i),
			OccurredAt: base.Add(time.Duration(i) * time.Second),
		}
		if i < 50 {
			e.UTMTerm = "old-term" // only events older than the newest 1000 fill this
		}
		if i == total-1 {
			e.UTMCampaign = "launch" // only the newest event fills this
		}
		require.NoError(t, s.InsertEvent(ctx, e))
	}

	got, err := s.Reader().PopulatedMetadataColumns(ctx, p.ID, "click")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"url", "utm_campaign"}, got)
}

func TestPopulatedMetadataColumns_NoEventsIsEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "PopulatedNone", "populated-none")
	require.NoError(t, err)

	got, err := s.Reader().PopulatedMetadataColumns(ctx, p.ID, "never_sent")
	require.NoError(t, err)
	require.Empty(t, got)
}
