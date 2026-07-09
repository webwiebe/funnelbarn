package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestIsMetadataColumn(t *testing.T) {
	assert.True(t, repository.IsMetadataColumn("url"))
	assert.True(t, repository.IsMetadataColumn("browser"))
	assert.False(t, repository.IsMetadataColumn("plan"))
	assert.NotEmpty(t, repository.MetadataColumns())
}

func TestWidget_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "Widgets", "widgets-crud")

	// Create — size out of range gets clamped to 1.
	w, err := s.CreateWidget(ctx, repository.DashboardWidget{
		ProjectID: p.ID,
		EventName: "page_view",
		Property:  "browser",
		Title:     "Browsers",
		Position:  0,
		Size:      99,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, w.ID)
	assert.Equal(t, "Browsers", w.Title)
	assert.Equal(t, 1, w.Size)
	assert.False(t, w.CreatedAt.IsZero())

	// Read by ID.
	got, err := s.WidgetByID(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, w.ID, got.ID)
	assert.Equal(t, "browser", got.Property)

	// List.
	list, err := s.ListWidgets(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, w.ID, list[0].ID)

	// Update.
	w.Title = "Updated"
	w.Property = "os"
	w.Size = 2
	updated, err := s.UpdateWidget(ctx, w)
	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Title)
	assert.Equal(t, "os", updated.Property)
	assert.Equal(t, 2, updated.Size)

	// Delete.
	require.NoError(t, s.DeleteWidget(ctx, w.ID))
	_, err = s.WidgetByID(ctx, w.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	empty, err := s.ListWidgets(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestWidgetBreakdown_TotalCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "WB", "wb-total")

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		e := repository.Event{
			ID:         randomHex(t) + string(rune('a'+i)),
			ProjectID:  p.ID,
			SessionID:  "s1",
			Name:       "signup",
			IngestID:   "ing-" + randomHex(t) + string(rune('a'+i)),
			OccurredAt: now,
		}
		require.NoError(t, s.InsertEvent(ctx, e))
	}

	// Empty property → total count only.
	res, err := s.WidgetBreakdown(ctx, p.ID, "signup", "", 100, 10)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "_total", res[0].Value)
	assert.EqualValues(t, 3, res[0].Count)
}

func TestWidgetBreakdown_MetadataColumn(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "WB", "wb-meta")

	now := time.Now().UTC().Truncate(time.Second)
	for _, browser := range []string{"Chrome", "Chrome", "Firefox"} {
		e := repository.Event{
			ID:         randomHex(t) + browser,
			ProjectID:  p.ID,
			SessionID:  "s1",
			Name:       "page_view",
			Browser:    browser,
			IngestID:   "ing-" + randomHex(t) + browser,
			OccurredAt: now,
		}
		require.NoError(t, s.InsertEvent(ctx, e))
	}

	res, err := s.WidgetBreakdown(ctx, p.ID, "page_view", "browser", 100, 10)
	require.NoError(t, err)
	require.NotEmpty(t, res)
	// Chrome (2) should rank first.
	assert.Equal(t, "Chrome", res[0].Value)
	assert.EqualValues(t, 2, res[0].Count)
}

func TestWidgetBreakdown_JSONProperty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "WB", "wb-json")

	now := time.Now().UTC().Truncate(time.Second)
	for _, plan := range []string{"pro", "pro", "free"} {
		e := repository.Event{
			ID:         randomHex(t) + plan,
			ProjectID:  p.ID,
			SessionID:  "s1",
			Name:       "signup",
			Properties: `{"plan":"` + plan + `"}`,
			IngestID:   "ing-" + randomHex(t) + plan,
			OccurredAt: now,
		}
		require.NoError(t, s.InsertEvent(ctx, e))
	}

	res, err := s.WidgetBreakdown(ctx, p.ID, "signup", "plan", 100, 10)
	require.NoError(t, err)
	require.NotEmpty(t, res)
	assert.Equal(t, "pro", res[0].Value)
	assert.EqualValues(t, 2, res[0].Count)
}

func TestWidgetBreakdown_InvalidPropertyName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "WB", "wb-invalid")

	_, err := s.WidgetBreakdown(ctx, p.ID, "signup", "bad name!", 100, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid property name")
}
