package command_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

func fullFlag() repository.FeatureFlag {
	evaluated := time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC)
	return repository.FeatureFlag{
		ID:              "flag-1",
		ProjectID:       "proj-1",
		FlagKey:         "new-checkout",
		Name:            "New checkout",
		FlagType:        "multivariate",
		Variants:        `["a","b"]`,
		DefaultVariant:  "a",
		Split:           `{"a":50,"b":50}`,
		ConversionEvent: "purchase",
		TargetingRules:  `[{"key":"plan"}]`,
		Status:          "active",
		CreatedAt:       time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
		Origin:          "auto",
		LastEvaluatedAt: &evaluated,
		Kind:            "experiment",
	}
}

func fullEvaluation() repository.FlagEvaluation {
	return repository.FlagEvaluation{
		ID:          "eval-1",
		FlagID:      "flag-1",
		ProjectID:   "proj-1",
		Variant:     "b",
		ContextHash: "abc123",
		SessionID:   "sess-1",
		ContextKeys: []string{"plan", "country"},
		CreatedAt:   time.Date(2026, 10, 6, 12, 31, 0, 0, time.UTC),
	}
}

func fullRecordingChunk() command.ApplyRecordingChunk {
	started := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	ended := started.Add(5 * time.Second)
	return command.ApplyRecordingChunk{
		Recording: repository.Recording{
			ID:              "rec-1",
			ProjectID:       "proj-1",
			SessionID:       "sess-1",
			Environment:     "production",
			FirstChunkIndex: 2,
			LastChunkIndex:  2,
			ChunkCount:      1,
			HasSnapshot:     true,
			DurationMs:      5000,
			StartedAt:       started,
			EndedAt:         &ended,
			CreatedAt:       started,
			DeviceType:      "desktop",
			UserAgent:       "Mozilla/5.0",
			PageURL:         "https://example.com/checkout",
		},
		ChunkIndex: 2,
		Traces: []repository.TraceLink{{
			TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
			SpanID:     "00f067aa0ba902b7",
			URL:        "/api/cart",
			OccurredAt: started.Add(time.Second),
		}},
	}
}

func TestCodecRoundTripsEveryKind(t *testing.T) {
	cmds := []command.Command{
		command.RecordEvaluation{Eval: fullEvaluation()},
		command.TouchAPIKey{KeyHash: "deadbeef"},
		command.TouchFlagEvaluated{ProjectID: "proj-1", FlagKey: "new-checkout"},
		command.MarkFlagsEvaluated{ProjectID: "proj-1"},
		command.EnsureAutoFlag{Flag: fullFlag(), Max: 50},
		command.IngestRecord{Record: spool.Record{
			IngestID:      "ing-1",
			ReceivedAt:    time.Date(2026, 10, 7, 8, 59, 0, 0, time.UTC),
			ContentType:   "application/json",
			RemoteAddr:    "10.0.0.1:5555",
			ClientIP:      "203.0.113.9",
			ContentLength: 42,
			BodyBase64:    "eyJuYW1lIjoicGFnZXZpZXcifQ==",
			ProjectSlug:   "site",
			UserAgent:     "Mozilla/5.0",
		}},
		fullRecordingChunk(),
	}
	for _, want := range cmds {
		t.Run(want.Kind(), func(t *testing.T) {
			at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
			b, err := command.Encode(want, at)
			require.NoError(t, err)

			got, env, err := command.Decode(b)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.Equal(t, want.Kind(), env.Kind)
			require.Equal(t, want.Project(), env.ProjectID)
			require.True(t, at.Equal(env.ReceivedAt))
		})
	}
}

func TestEncodeSetsEnvelopeProjectID(t *testing.T) {
	b, err := command.Encode(command.MarkFlagsEvaluated{ProjectID: "proj-9"}, time.Now())
	require.NoError(t, err)
	_, env, err := command.Decode(b)
	require.NoError(t, err)
	require.Equal(t, "proj-9", env.ProjectID)
}

func TestEncodeAssignsEvaluationID(t *testing.T) {
	eval := fullEvaluation()
	eval.ID = ""
	b, err := command.Encode(command.RecordEvaluation{Eval: eval}, time.Now())
	require.NoError(t, err)
	got, _, err := command.Decode(b)
	require.NoError(t, err)
	require.NotEmpty(t, got.(command.RecordEvaluation).Eval.ID)
}

func TestDecodeUnknownKind(t *testing.T) {
	_, _, err := command.Decode([]byte(`{"kind":"nope","payload":{}}`))
	require.ErrorContains(t, err, "unknown kind")
}

func TestDecodeMalformed(t *testing.T) {
	_, _, err := command.Decode([]byte(`{not json`))
	require.Error(t, err)

	_, _, err = command.Decode([]byte(`{"kind":"touch_api_key","payload":"a string"}`))
	require.Error(t, err)
}
