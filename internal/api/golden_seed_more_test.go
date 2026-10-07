package api

// Second half of the golden seed: the configured entities (funnels, flags,
// widgets, segments, recordings, settings) and the two normalisation passes
// that make generated ids and timestamps fixed.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

const (
	goldenDesktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0 Safari/537.36"
	goldenMobileUA  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Safari/604.1"
)

func seedGoldenEntities(t *testing.T, store *repository.Store, storage *memStorage, s *goldenSeed) {
	t.Helper()
	ctx := context.Background()
	must := func(err error, what string) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	ids := map[string]string{}
	fix := func(old string, kind, n int) string {
		fixed := goldenID(kind, n)
		ids[old] = fixed
		return fixed
	}

	// Funnels. The session-scoped purchase funnel has drop-off at every step.
	fa, err := store.CreateFunnel(ctx, repository.Funnel{ProjectID: s.ProjectA, Name: "Purchase", Description: "Visit to purchase",
		Steps: []repository.FunnelStep{{EventName: "page_view"}, {EventName: "add_to_cart"}, {EventName: "checkout_start"}, {EventName: "purchase"}}})
	must(err, "funnel A")
	fp, err := store.CreateFunnel(ctx, repository.Funnel{ProjectID: s.ProjectA, Name: "Pricing to cart", Scope: "page_view",
		Steps: []repository.FunnelStep{{EventName: "page_view"}, {EventName: "add_to_cart"}}})
	must(err, "funnel A page")
	fb, err := store.CreateFunnel(ctx, repository.Funnel{ProjectID: s.ProjectB, Name: "Blog signup",
		Steps: []repository.FunnelStep{{EventName: "page_view"}, {EventName: "signup", Filters: []repository.FunnelFilter{{Property: "plan", Value: "pro"}}}}})
	must(err, "funnel B")
	s.FunnelA, s.FunnelAPage, s.FunnelB = fix(fa.ID, kindFunnel, 1), fix(fp.ID, kindFunnel, 2), fix(fb.ID, kindFunnel, 3)
	n := 0
	for _, f := range []repository.Funnel{fa, fp, fb} {
		for _, st := range f.Steps {
			n++
			fix(st.ID, kindFunnelStep, n)
		}
	}

	// Canonical vocabulary: mappings for both projects, one unmapped name left
	// over so the suggestions endpoint has something to say.
	for _, m := range [][3]string{
		{s.ProjectA, "page_view", "page_view"}, {s.ProjectA, "add_to_cart", "add_to_cart"},
		{s.ProjectA, "checkout_start", "checkout_start"}, {s.ProjectA, "purchase", "purchase"},
		{s.ProjectA, "signup", "sign_up"}, {s.ProjectB, "page_view", "page_view"}, {s.ProjectB, "signup", "sign_up"},
	} {
		must(store.UpsertMapping(ctx, m[0], m[1], m[2]), "mapping "+m[1])
	}
	cf, err := store.CreateCanonicalFunnel(ctx, repository.CanonicalFunnel{Name: "Visit to signup", Description: "Across projects",
		ProjectIDs: []string{s.ProjectA, s.ProjectB},
		Steps:      []repository.CanonicalFunnelStep{{CanonicalKey: "page_view", Label: "Visit"}, {CanonicalKey: "sign_up", Label: "Sign up"}}})
	must(err, "canonical funnel")
	s.CanonicalFunnel = fix(cf.ID, kindCanonicalFunnel, 1)

	// Flags: experiment with a split, experiment with targeting rules, config,
	// inactive manual, auto-registered, plus one in project B.
	flags := service.NewFlagService(store)
	mk := func(f repository.FeatureFlag, nth int) string {
		t.Helper()
		got, err := flags.CreateFlag(ctx, f)
		must(err, "flag "+f.FlagKey)
		return fix(got.ID, kindFlag, nth)
	}
	s.FlagExperiment = mk(repository.FeatureFlag{ProjectID: s.ProjectA, FlagKey: "checkout_redesign", Name: "Checkout redesign",
		FlagType: "experiment", Variants: `{"control":false,"treatment":true}`, DefaultVariant: "control",
		Split: `{"control":50,"treatment":50}`, ConversionEvent: "purchase"}, 1)
	s.FlagTargeted = mk(repository.FeatureFlag{ProjectID: s.ProjectA, FlagKey: "pricing_banner", Name: "Pricing banner",
		FlagType: "string", Variants: `{"a":"standard","b":"dutch-promo"}`, DefaultVariant: "a", Split: `{"a":100,"b":0}`,
		TargetingRules: `[{"name":"dutch visitors","variant":"b","match":"all","conditions":[{"context_key":"country","operator":"eq","value":"NL"}]}]`}, 2)
	s.FlagConfig = mk(repository.FeatureFlag{ProjectID: s.ProjectA, FlagKey: "max_items", Name: "Max items per page",
		FlagType: "number", Kind: repository.FlagKindConfig, Variants: `{"default":25,"high":100}`, DefaultVariant: "default", Split: `{}`}, 3)
	s.FlagInactive = mk(repository.FeatureFlag{ProjectID: s.ProjectA, FlagKey: "legacy_search", Name: "Legacy search",
		FlagType: "boolean", Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":0,"off":100}`, Status: "paused"}, 4)
	auto, err := store.CreateFlag(ctx, repository.FeatureFlag{ProjectID: s.ProjectA, FlagKey: "sdk_discovered", Name: "sdk_discovered",
		FlagType: "boolean", Variants: `{"default":false}`, DefaultVariant: "default", Split: `{}`, Status: "inactive", Origin: "auto"})
	must(err, "auto flag")
	s.FlagAuto = fix(auto.ID, kindFlag, 5)
	s.FlagB = mk(repository.FeatureFlag{ProjectID: s.ProjectB, FlagKey: "beta_banner", Name: "Beta banner",
		FlagType: "boolean", Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":30,"off":70}`}, 6)

	ab, err := store.CreateABTest(ctx, repository.ABTest{ProjectID: s.ProjectA, Name: "Variant a vs b", Status: "running",
		ControlFilter: `{"property":"variant","value":"a"}`, VariantFilter: `{"property":"variant","value":"b"}`, ConversionEvent: "purchase"})
	must(err, "abtest")
	s.ABTestA = fix(ab.ID, kindABTest, 1)

	for i, w := range []repository.DashboardWidget{
		{ProjectID: s.ProjectA, EventName: "purchase", Property: "plan", Title: "Purchases by plan", Position: 0, Size: 2},
		{ProjectID: s.ProjectA, EventName: "signup", Property: "plan", Title: "Signups by plan", Position: 1, Size: 1},
		{ProjectID: s.ProjectB, EventName: "page_view", Property: "plan", Title: "Views by plan", Position: 0, Size: 1},
	} {
		got, err := store.CreateWidget(ctx, w)
		must(err, "widget")
		fixed := fix(got.ID, kindWidget, i+1)
		switch i {
		case 0:
			s.WidgetA1 = fixed
		case 1:
			s.WidgetA2 = fixed
		default:
			s.WidgetB = fixed
		}
	}

	seg, err := store.CreateSegment(ctx, repository.Segment{ProjectID: s.ProjectA, Name: "Mobile visitors",
		Rules: []repository.SegmentRule{{Field: "device_type", Operator: "eq", Value: "mobile"}}})
	must(err, "segment")
	s.SegmentMobile = fix(seg.ID, kindSegment, 1)
	_, err = store.CreateSegment(ctx, repository.Segment{ProjectID: s.ProjectA, Name: "Dutch visitors",
		Rules: []repository.SegmentRule{{Field: "country_code", Operator: "eq", Value: "NL"}}})
	must(err, "segment 2")
	if segs, err := store.ListSegments(ctx, s.ProjectA); err == nil {
		for i, sg := range segs {
			if sg.ID != s.SegmentMobile {
				fix(sg.ID, kindSegment, 100+i)
			}
		}
	}

	seedGoldenRecordings(t, store, storage, s)

	enabled, rate := true, 0.5
	must(store.UpsertProjectRecordingSettings(ctx, &repository.ProjectRecordingSettings{ProjectID: s.ProjectA,
		Enabled: &enabled, SampleRate: &rate, Rules: []repository.RecordingRule{{Pattern: "/checkout*", Action: "capture"}}}), "recording settings")
	for k, v := range map[string]string{"recording_enabled": "true", "recording_sample_rate": "0.25", "retention_days": "90"} {
		must(store.SetInstanceSetting(ctx, k, v), "instance setting "+k)
	}

	goldenRemap(t, store, ids)
}

func seedGoldenRecordings(t *testing.T, store *repository.Store, storage *memStorage, s *goldenSeed) {
	t.Helper()
	ctx := context.Background()
	svc := service.NewRecordingService(store, store, store, storage)
	s.RecordingA1, s.RecordingA2, s.RecordingB = "rec-A-01", "rec-A-02", "rec-B-01"
	s.TraceA = "4bf92f3577b34da6a3ce929d0e0e4736"
	ts := goldenBase.Add(9 * time.Hour)
	snapshot := json.RawMessage(`[{"type":4,"data":{"href":"https://acme.example/"},"timestamp":1772442000000},{"type":2,"data":{"node":{}},"timestamp":1772442000100}]`)
	incr := json.RawMessage(`[{"type":3,"data":{"source":1},"timestamp":1772442005000}]`)
	chunks := []service.RecordingChunk{
		{RecordingID: s.RecordingA1, SessionID: "sess-A-00", ChunkIndex: 0, Events: snapshot, StartedAt: ts, DurationMs: 5000,
			PageURL: "https://acme.example/", ProjectID: s.ProjectA, Environment: "production", UserAgent: goldenDesktopUA,
			Traces: []repository.TraceLink{{TraceID: s.TraceA, SpanID: "00f067aa0ba902b7", URL: "https://acme.example/api/cart", OccurredAt: ts.Add(2 * time.Second)}}},
		{RecordingID: s.RecordingA1, SessionID: "sess-A-00", ChunkIndex: 1, Events: incr, StartedAt: ts.Add(5 * time.Second), DurationMs: 4000,
			PageURL: "https://acme.example/", ProjectID: s.ProjectA, Environment: "production", UserAgent: goldenDesktopUA},
		{RecordingID: s.RecordingA2, SessionID: "sess-A-02", ChunkIndex: 0, Events: snapshot, StartedAt: ts.Add(time.Hour), DurationMs: 7000,
			PageURL: "https://acme.example/pricing", ProjectID: s.ProjectA, Environment: "production", UserAgent: goldenMobileUA},
		{RecordingID: s.RecordingB, SessionID: "sess-B-00", ChunkIndex: 0, Events: snapshot, StartedAt: ts.Add(2 * time.Hour), DurationMs: 3000,
			PageURL: "https://beta.example/blog/", ProjectID: s.ProjectB, Environment: "production", UserAgent: goldenDesktopUA},
	}
	for _, c := range chunks {
		if err := svc.IngestChunk(ctx, c); err != nil {
			t.Fatalf("seed recording chunk: %v", err)
		}
	}
}

// seedGoldenEvaluations records flag evaluations through the real service so
// the flag analysis has variants and conversions to report. It runs after the
// time normalisation on purpose: evaluations carry the insert time and the
// analysis routes look back from "now", so counts stay stable for as long as a
// run lasts and the timestamps themselves never reach a response.
func seedGoldenEvaluations(t *testing.T, store *repository.Store, s *goldenSeed) {
	t.Helper()
	ctx := context.Background()
	flags := service.NewFlagService(store)
	for i := 0; i < 40; i++ {
		ec := map[string]any{"targeting_key": fmt.Sprintf("u-A-%02d", i), "session_id": fmt.Sprintf("sess-A-%02d", i), "country": goldenCountries[i%5]}
		for _, key := range []string{"checkout_redesign", "pricing_banner", "legacy_search"} {
			if _, err := flags.EvaluateFlag(ctx, s.ProjectA, key, ec); err != nil {
				t.Fatalf("seed evaluation %s: %v", key, err)
			}
		}
	}
}

// goldenRemap rewrites every generated id to its fixed value in every table
// and text column. Foreign keys are switched off on the one pooled connection
// for the duration and re-checked afterwards, so a dangling reference fails
// the seed instead of the goldens.
func goldenRemap(t *testing.T, store *repository.Store, ids map[string]string) {
	t.Helper()
	if len(ids) == 0 {
		return
	}
	ctx := context.Background()
	conn, err := store.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("remap: conn: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatalf("remap: fk off: %v", err)
	}
	for _, tc := range goldenTextColumns(t, ctx, conn) {
		for from, to := range ids {
			q := fmt.Sprintf(`UPDATE "%s" SET "%s" = ? WHERE "%s" = ?`, tc[0], tc[1], tc[1])
			if _, err := conn.ExecContext(ctx, q, to, from); err != nil {
				t.Fatalf("remap %s.%s: %v", tc[0], tc[1], err)
			}
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("remap: fk on: %v", err)
	}
	rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("remap: fk check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("remap left a dangling foreign key")
	}
}

// goldenTextColumns lists [table, column] for every text-like column.
func goldenTextColumns(t *testing.T, ctx context.Context, conn *sql.Conn) [][2]string {
	t.Helper()
	tables, err := conn.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var names []string
	for tables.Next() {
		var n string
		if err := tables.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	tables.Close()
	var out [][2]string
	for _, tbl := range names {
		cols, err := conn.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, tbl))
		if err != nil {
			t.Fatalf("table_info: %v", err)
		}
		for cols.Next() {
			var cid, notNull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := cols.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				t.Fatalf("scan: %v", err)
			}
			if u := strings.ToUpper(typ); strings.Contains(u, "TEXT") || strings.Contains(u, "CHAR") || u == "" {
				out = append(out, [2]string{tbl, name})
			}
		}
		cols.Close()
	}
	return out
}

// goldenNormalizeTimes replaces every created_at/updated_at (SQL defaults,
// CURRENT_TIMESTAMP) with a fixed timestamp that grows with the row order, so
// ORDER BY created_at stays stable and nothing carries the wall clock.
func goldenNormalizeTimes(t *testing.T, store *repository.Store) {
	t.Helper()
	ctx := context.Background()
	conn, err := store.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("normalize: conn: %v", err)
	}
	defer conn.Close()
	base := goldenBase.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	for _, tc := range goldenTextTimeColumns(t, ctx, conn) {
		q := fmt.Sprintf(`UPDATE "%s" SET "%s" = datetime(?, '+' || rowid || ' seconds') WHERE "%s" IS NOT NULL`, tc[0], tc[1], tc[1])
		if _, err := conn.ExecContext(ctx, q, base); err != nil {
			t.Fatalf("normalize %s.%s: %v", tc[0], tc[1], err)
		}
	}
}

// goldenTextTimeColumns lists [table, column] for created_at and updated_at,
// whatever their declared type (DATETIME, TIMESTAMP, TEXT).
func goldenTextTimeColumns(t *testing.T, ctx context.Context, conn *sql.Conn) [][2]string {
	t.Helper()
	var out [][2]string
	rows, err := conn.QueryContext(ctx, `SELECT m.name, p.name FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type='table' AND p.name IN ('created_at','updated_at') AND m.name NOT LIKE 'sqlite_%' AND m.name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatalf("time columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, [2]string{tbl, col})
	}
	return out
}
