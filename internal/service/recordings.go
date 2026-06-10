package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// RecordingChunk is a chunk of rrweb events sent from the SDK.
type RecordingChunk struct {
	RecordingID string          `json:"recording_id"`
	SessionID   string          `json:"session_id"`
	ChunkIndex  int             `json:"chunk_index"`
	Events      json.RawMessage `json:"events"`
	StartedAt   time.Time       `json:"started_at"`
	DurationMs  int64           `json:"duration_ms"`
	PageURL     string          `json:"page_url"`
	Environment string          `json:"-"` // set by the handler from the resolved project
	ProjectSlug string          `json:"-"` // set by the handler
	ProjectID   string          `json:"-"` // set by the handler after slug lookup
	UserAgent   string          `json:"-"` // set by the handler from the request UA header
}

// RecordingStorage is the interface for chunk blob storage (R2).
type RecordingStorage interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// RecordingService handles session recording business logic.
type RecordingService struct {
	store   ports.RecordingRepo
	funnels ports.FunnelRepo
	events  ports.EventRepo
	storage RecordingStorage
}

// NewRecordingService creates a new RecordingService.
func NewRecordingService(store ports.RecordingRepo, funnels ports.FunnelRepo, events ports.EventRepo, storage RecordingStorage) *RecordingService {
	return &RecordingService{store: store, funnels: funnels, events: events, storage: storage}
}

// IngestChunk compresses the rrweb event chunk, uploads it to R2, and
// upserts the recording metadata row in SQLite.
func (svc *RecordingService) IngestChunk(ctx context.Context, chunk RecordingChunk) error {
	// Compress events.
	compressed, err := gzipJSON(chunk.Events)
	if err != nil {
		return fmt.Errorf("recordings: compress chunk: %w", err)
	}

	key := chunkKey(chunk.ProjectID, chunk.RecordingID, chunk.ChunkIndex)
	if err := svc.storage.Put(ctx, key, compressed); err != nil {
		return fmt.Errorf("recordings: upload chunk: %w", err)
	}

	endedAt := chunk.StartedAt.Add(time.Duration(chunk.DurationMs) * time.Millisecond)
	rec := repository.Recording{
		ID:              chunk.RecordingID,
		ProjectID:       chunk.ProjectID,
		SessionID:       chunk.SessionID,
		Environment:     chunk.Environment,
		FirstChunkIndex: chunk.ChunkIndex,
		ChunkCount:      1,
		DurationMs:      chunk.DurationMs,
		StartedAt:       chunk.StartedAt,
		EndedAt:         &endedAt,
		UserAgent:       chunk.UserAgent,
		DeviceType:      DetectDeviceType(chunk.UserAgent),
		IsBot:           DetectBot(chunk.UserAgent),
		PageURL:         chunk.PageURL,
	}
	return svc.store.UpsertRecording(ctx, rec)
}

// PurgeOldRecordings deletes recordings older than retentionDays from both R2 and SQLite.
func (svc *RecordingService) PurgeOldRecordings(ctx context.Context, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	before := time.Now().AddDate(0, 0, -retentionDays)
	recs, err := svc.store.ListOldRecordings(ctx, before)
	if err != nil {
		return fmt.Errorf("recordings: list old: %w", err)
	}
	for _, rec := range recs {
		for i := 0; i < rec.ChunkCount; i++ {
			key := chunkKey(rec.ProjectID, rec.ID, i)
			if delErr := svc.storage.Delete(ctx, key); delErr != nil {
				// Don't abort the purge — a failed chunk delete leaves an
				// R2 orphan but the row will be removed below. Warn so we
				// can correlate orphans with storage outages later.
				slog.WarnContext(ctx, "recordings: purge chunk delete failed",
					"err", delErr, "handled", true,
					"recording_id", rec.ID, "chunk_index", i, "key", key)
			}
		}
		if err := svc.store.DeleteRecording(ctx, rec.ID); err != nil {
			return fmt.Errorf("recordings: delete row %s: %w", rec.ID, err)
		}
	}
	return nil
}

// ListRecordings returns recordings for a project with optional filters.
func (svc *RecordingService) ListRecordings(ctx context.Context, projectID string, opts repository.RecordingListOpts) ([]repository.Recording, error) {
	return svc.store.ListRecordings(ctx, projectID, opts)
}

// GetChunk fetches, decompresses, and returns the raw JSON event array for one chunk.
func (svc *RecordingService) GetChunk(ctx context.Context, projectID, recordingID string, index int) ([]byte, error) {
	rec, err := svc.store.GetRecording(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("recordings: get recording: %w", err)
	}
	if rec.ProjectID != projectID {
		return nil, fmt.Errorf("recordings: not found")
	}

	key := chunkKey(rec.ProjectID, recordingID, index)
	compressed, err := svc.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("recordings: download chunk: %w", err)
	}
	return gunzip(compressed)
}

// GetRecordingSessionID returns the session_id for a recording, verifying project ownership.
func (svc *RecordingService) GetRecordingSessionID(ctx context.Context, projectID, recordingID string) (string, error) {
	rec, err := svc.store.GetRecording(ctx, recordingID)
	if err != nil {
		return "", fmt.Errorf("recordings: get recording: %w", err)
	}
	if rec.ProjectID != projectID {
		return "", fmt.Errorf("recordings: not found")
	}
	return rec.SessionID, nil
}

// FlagEvaluationsForSession returns flag evaluations for the session linked to a recording.
func (svc *RecordingService) FlagEvaluationsForSession(ctx context.Context, projectID, sessionID string) ([]repository.FlagEvaluationEntry, error) {
	return svc.store.FlagEvaluationsForSession(ctx, sessionID, projectID)
}

// SessionsAtStep returns session IDs that dropped off at the given funnel step.
func (svc *RecordingService) SessionsAtStep(ctx context.Context, funnelID, projectID string, stepOrder int, from, to time.Time) ([]string, error) {
	f, err := svc.funnels.FunnelByID(ctx, funnelID)
	if err != nil {
		return nil, err
	}
	if f.ProjectID != projectID {
		return nil, fmt.Errorf("recordings: funnel not found")
	}
	return svc.funnels.SessionsAtStep(ctx, f, stepOrder, from, to, 100)
}

// SessionsForPage returns session IDs that visited the given page URL.
func (svc *RecordingService) SessionsForPage(ctx context.Context, projectID, page string, from, to time.Time) ([]string, error) {
	return svc.events.SessionsForPage(ctx, projectID, page, from, to, 100)
}

func chunkKey(projectSlug, recordingID string, index int) string {
	return fmt.Sprintf("recordings/%s/%s/%05d.json.gz", projectSlug, recordingID, index)
}

// DetectDeviceType returns "mobile", "tablet", or "desktop" from a User-Agent string.
func DetectDeviceType(ua string) string {
	ua = strings.ToLower(ua)
	if strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") ||
		(strings.Contains(ua, "android") && !strings.Contains(ua, "mobile")) {
		return "tablet"
	}
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "iphone") ||
		strings.Contains(ua, "ipod") || strings.Contains(ua, "android") ||
		strings.Contains(ua, "blackberry") || strings.Contains(ua, "windows phone") {
		return "mobile"
	}
	return "desktop"
}

// DetectBot returns true when the User-Agent looks like a bot/crawler.
func DetectBot(ua string) bool {
	if ua == "" {
		return false
	}
	ua = strings.ToLower(ua)
	botSignals := []string{
		"bot", "crawler", "spider", "slurp", "baiduspider", "yandex",
		"facebookexternalhit", "twitterbot", "linkedinbot", "whatsapp",
		"googlebot", "bingbot", "duckduckbot", "sogou", "exabot",
		"semrushbot", "ahrefsbot", "mj12bot", "dotbot", "uptimerobot",
		"pingdom", "statuscake", "headlesschrome", "phantomjs", "puppeteer",
		"selenium", "python-requests", "go-http-client", "java/", "curl/",
		"wget/", "libwww-perl", "python/",
	}
	for _, s := range botSignals {
		if strings.Contains(ua, s) {
			return true
		}
	}
	return false
}

func gzipJSON(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
