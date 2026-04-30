package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/session"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// EventPayload is the JSON body accepted by POST /api/v1/events.
type EventPayload struct {
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	Referrer    string         `json:"referrer"`
	UTMSource   string         `json:"utm_source"`
	UTMMedium   string         `json:"utm_medium"`
	UTMCampaign string         `json:"utm_campaign"`
	UTMTerm     string         `json:"utm_term"`
	UTMContent  string         `json:"utm_content"`
	Properties  map[string]any `json:"properties"`
	SessionID   string         `json:"session_id"`
	UserID      string         `json:"user_id"`
	UserAgent   string         `json:"user_agent"`
	Timestamp   time.Time      `json:"timestamp"`
}

// ProcessRecord decodes and enriches a spool record into a repository.Event.
// It does not touch the database.
func ProcessRecord(record spool.Record) (repository.Event, error) {
	body, err := base64.StdEncoding.DecodeString(record.BodyBase64)
	if err != nil {
		return repository.Event{}, fmt.Errorf("decode body: %w", err)
	}

	var payload EventPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return repository.Event{}, fmt.Errorf("unmarshal payload: %w", err)
	}

	if payload.Name == "" {
		return repository.Event{}, fmt.Errorf("event name is required")
	}

	occurredAt := payload.Timestamp
	if occurredAt.IsZero() {
		occurredAt = record.ReceivedAt
	}

	// Derive session ID.
	sessionID := payload.SessionID
	if sessionID == "" || !session.IsValidSessionID(sessionID) {
		sessionID = session.Fingerprint(record.RemoteAddr, payload.UserAgent)
	}

	// Extract UTM from URL if not provided directly.
	utmFromURL := enrich.ExtractUTM(payload.URL)
	utmSource := coalesce(payload.UTMSource, utmFromURL.Source)
	utmMedium := coalesce(payload.UTMMedium, utmFromURL.Medium)
	utmCampaign := coalesce(payload.UTMCampaign, utmFromURL.Campaign)
	utmTerm := coalesce(payload.UTMTerm, utmFromURL.Term)
	utmContent := coalesce(payload.UTMContent, utmFromURL.Content)

	// Encode properties.
	propsJSON := ""
	if len(payload.Properties) > 0 {
		b, _ := json.Marshal(payload.Properties)
		propsJSON = string(b)
	}

	// Hash user ID for privacy-safe storage.
	userIDHash := enrich.HashUserID(payload.UserID)

	// Extract referrer domain.
	referrerDomain := enrich.ExtractReferrerDomain(payload.Referrer)

	// Parse user agent.
	ua := payload.UserAgent
	var browser, osName, deviceType string
	if ua != "" {
		uaInfo := enrich.ParseUA(ua)
		browser = uaInfo.Browser
		osName = uaInfo.OS
		deviceType = uaInfo.DeviceType
	}

	eventID, err := generateUUIDLocal()
	if err != nil {
		return repository.Event{}, fmt.Errorf("generate uuid: %w", err)
	}

	event := repository.Event{
		ID:             eventID,
		SessionID:      sessionID,
		UserIDHash:     userIDHash,
		Name:           payload.Name,
		URL:            payload.URL,
		Referrer:       payload.Referrer,
		ReferrerDomain: referrerDomain,
		UTMSource:      utmSource,
		UTMMedium:      utmMedium,
		UTMCampaign:    utmCampaign,
		UTMTerm:        utmTerm,
		UTMContent:     utmContent,
		Properties:     propsJSON,
		UserAgent:      ua,
		Browser:        browser,
		OS:             osName,
		DeviceType:     deviceType,
		IngestID:       record.IngestID,
		OccurredAt:     occurredAt,
	}

	return event, nil
}

// PersistEvent stores an event and upserts the associated session.
func PersistEvent(ctx context.Context, store *repository.Store, event repository.Event) error {
	// Check idempotency: skip if already stored.
	existing, err := store.GetEventByIngestID(ctx, event.IngestID)
	if err != nil {
		return fmt.Errorf("check idempotency: %w", err)
	}
	if existing != nil {
		slog.Debug("event already stored, skipping", "ingest_id", event.IngestID)
		return nil
	}

	// Insert the event.
	if err := store.InsertEvent(ctx, event); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	slog.Info("event stored",
		"ingest_id", event.IngestID,
		"event_id", event.ID,
		"project_id", event.ProjectID,
		"event_name", event.Name,
		"session_id", event.SessionID,
	)

	// Upsert session.
	sess := repository.Session{
		ID:          event.SessionID,
		ProjectID:   event.ProjectID,
		FirstSeenAt: event.OccurredAt,
		LastSeenAt:  event.OccurredAt,
		EntryURL:    event.URL,
		ExitURL:     event.URL,
		Referrer:    event.Referrer,
		UTMSource:   event.UTMSource,
		UTMMedium:   event.UTMMedium,
		UTMCampaign: event.UTMCampaign,
		DeviceType:  event.DeviceType,
		CountryCode: event.CountryCode,
	}
	if err := store.UpsertSession(ctx, sess); err != nil {
		// Non-fatal: log and continue.
		slog.Warn("upsert session failed", "err", err, "session_id", event.SessionID)
	}

	return nil
}

// SafeProcess wraps ProcessRecord with a panic recovery so that a panicking
// record does not crash the background worker goroutine.
func SafeProcess(rec spool.Record) (event repository.Event, err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("worker panic", "ingest_id", rec.IngestID, "panic", fmt.Sprint(p))
			err = fmt.Errorf("worker panic: %v", p)
		}
	}()
	return ProcessRecord(rec)
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
