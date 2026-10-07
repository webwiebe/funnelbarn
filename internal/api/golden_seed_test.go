package api

// Deterministic seed for the golden suite: projects, API keys, and the event
// and session timeline. The remaining entities (funnels, flags, widgets, ...)
// are in golden_seed_more_test.go. The repository generates random UUIDs for
// most rows, so after seeding every generated id is rewritten to a fixed value
// (goldenRemap) and every created_at/updated_at to a fixed timestamp
// (goldenNormalizeTimes). Responses then repeat byte for byte across runs.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// goldenIDPrefix marks every fixed id. The masker leaves ids with this prefix
// alone and replaces all other UUIDs (those were generated while serving).
const goldenIDPrefix = "f0000000-"

// goldenID builds the fixed id for the n-th row of a kind.
func goldenID(kind, n int) string {
	return fmt.Sprintf("%s%04x-4000-8000-%012x", goldenIDPrefix, kind, n)
}

// Kinds of fixed ids.
const (
	kindProject = iota + 1
	kindAPIKey
	kindFunnel
	kindFunnelStep
	kindFlag
	kindABTest
	kindWidget
	kindSegment
	kindCanonicalFunnel
)

// goldenSeed records the ids and credentials of the seeded dataset so cases can
// fill path parameters and authenticate.
type goldenSeed struct {
	ProjectA, ProjectB string // fixed project ids
	SlugA, SlugB       string

	// Plaintext API keys.
	KeyAFull, KeyAIngest, KeyAAnalytics, KeyAFlags, KeyBFull string

	APIKeyIDs []string // fixed ids of the keys above, same order

	FunnelA, FunnelAPage, FunnelB string
	CanonicalFunnel               string
	FlagExperiment, FlagTargeted  string // ids in project A
	FlagConfig, FlagInactive      string
	FlagAuto, FlagB               string
	ABTestA                       string
	WidgetA1, WidgetA2, WidgetB   string
	SegmentMobile                 string
	RecordingA1, RecordingA2      string // recording ids (fixed by the seed)
	RecordingB                    string
	TraceA                        string // a trace id linked to RecordingA1

	// Window is the explicit from/to every time-relative route is called with.
	From, To string
}

// goldenBase is day 0 of the dataset. Everything is anchored here, never to
// time.Now(), so goldens do not age.
var goldenBase = time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)

const goldenDays = 5

func goldenSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// seedGolden builds the whole dataset into store and returns its handles.
func seedGolden(t *testing.T, store *repository.Store, storage *memStorage) *goldenSeed {
	t.Helper()
	ctx := context.Background()
	must := func(err error, what string) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	s := &goldenSeed{
		SlugA: "acme-shop", SlugB: "beta-blog",
		KeyAFull: "gk-a-full", KeyAIngest: "gk-a-ingest", KeyAAnalytics: "gk-a-analytics",
		KeyAFlags: "gk-a-flags", KeyBFull: "gk-b-full",
		From: goldenBase.Format(time.RFC3339),
		To:   goldenBase.AddDate(0, 0, goldenDays+1).Format(time.RFC3339),
	}

	// Projects first: remapping their ids immediately means every child row
	// is created against the fixed id.
	pa, err := store.CreateProject(ctx, "Acme Shop", s.SlugA)
	must(err, "project A")
	pb, err := store.CreateProject(ctx, "Beta Blog", s.SlugB)
	must(err, "project B")
	s.ProjectA, s.ProjectB = goldenID(kindProject, 1), goldenID(kindProject, 2)
	goldenRemap(t, store, map[string]string{pa.ID: s.ProjectA, pb.ID: s.ProjectB})
	_, err = store.UpdateProject(ctx, s.ProjectA, "Acme Shop", "acme.example")
	must(err, "project A domain")
	_, err = store.UpdateProject(ctx, s.ProjectB, "Beta Blog", "beta.example")
	must(err, "project B domain")

	ids := map[string]string{}
	keys := []struct{ name, plain, scope, project string }{
		{"a-full", s.KeyAFull, repository.APIKeyScopeFull, s.ProjectA},
		{"a-ingest", s.KeyAIngest, repository.APIKeyScopeIngest, s.ProjectA},
		{"a-analytics", s.KeyAAnalytics, repository.APIKeyScopeAnalyticsRead, s.ProjectA},
		{"a-flags", s.KeyAFlags, repository.APIKeyScopeFlagsRead, s.ProjectA},
		{"b-full", s.KeyBFull, repository.APIKeyScopeFull, s.ProjectB},
	}
	for i, k := range keys {
		ak, err := store.CreateAPIKey(ctx, k.name, k.project, goldenSHA256(k.plain), k.scope)
		must(err, "api key "+k.name)
		fixed := goldenID(kindAPIKey, i+1)
		ids[ak.ID] = fixed
		s.APIKeyIDs = append(s.APIKeyIDs, fixed)
	}
	goldenRemap(t, store, ids)

	seedEventsA(t, store, s)
	seedEventsB(t, store, s)
	seedGoldenEntities(t, store, storage, s)
	goldenNormalizeTimes(t, store)
	seedGoldenEvaluations(t, store, s)
	return s
}

var (
	goldenCountries = []string{"NL", "DE", "US", "FR", "GB"}
	goldenDevices   = []string{"desktop", "mobile", "tablet"}
	goldenBrowsers  = []string{"Chrome", "Safari", "Firefox"}
	goldenOSes      = []string{"macOS", "iOS", "Windows", "Android"}
	goldenRefs      = []string{"", "google.com", "twitter.com", "news.ycombinator.com"}
	goldenPlans     = []string{"free", "pro", "team"}
	goldenCities    = []string{"Amsterdam", "Berlin", "Austin", "Paris", "London"}
)

// goldenEvent is a compact description of one event to insert.
type goldenEvent struct {
	name, path, props, pageView string
	offset                      time.Duration
}

// insertSession inserts a session's events the way the worker does: one
// InsertEvent plus one UpsertSession per event.
func insertSession(t *testing.T, store *repository.Store, project, host, tag string, i int, evs []goldenEvent) string {
	t.Helper()
	ctx := context.Background()
	sid := fmt.Sprintf("sess-%s-%02d", tag, i)
	start := goldenBase.AddDate(0, 0, i%goldenDays).Add(time.Duration(8+i%10) * time.Hour).Add(time.Duration(i%7) * time.Minute)
	country := goldenCountries[i%len(goldenCountries)]
	ref := goldenRefs[i%len(goldenRefs)]
	refDomain := ref
	if ref != "" {
		ref = "https://" + ref + "/"
	}
	var utmSource, utmMedium, utmCampaign string
	if i%5 == 1 {
		utmSource, utmMedium, utmCampaign = "newsletter", "email", "spring-"+tag
	}
	env := "production"
	if i%7 == 0 {
		env = "staging"
	}
	for k, ev := range evs {
		at := start.Add(ev.offset)
		e := repository.Event{
			ID:         fmt.Sprintf("ev-%s-%02d-%02d", tag, i, k),
			ProjectID:  project,
			SessionID:  sid,
			UserIDHash: fmt.Sprintf("u-%s-%02d", tag, i),
			Name:       ev.name,
			URL:        "https://" + host + ev.path,
			Referrer:   ref, ReferrerDomain: refDomain,
			UTMSource: utmSource, UTMMedium: utmMedium, UTMCampaign: utmCampaign,
			Properties:  ev.props,
			UserAgent:   "Mozilla/5.0 golden",
			Browser:     goldenBrowsers[i%len(goldenBrowsers)],
			OS:          goldenOSes[i%len(goldenOSes)],
			DeviceType:  goldenDevices[i%len(goldenDevices)],
			CountryCode: country, PageViewID: ev.pageView,
			IngestID: fmt.Sprintf("ing-%s-%02d-%02d", tag, i, k), OccurredAt: at, Environment: env,
		}
		if err := store.InsertEvent(ctx, e); err != nil {
			t.Fatalf("seed event: %v", err)
		}
		sess := repository.Session{
			ID: sid, ProjectID: project, FirstSeenAt: start, LastSeenAt: at,
			EntryURL: "https://" + host + evs[0].path, ExitURL: e.URL,
			Referrer: ref, UTMSource: utmSource, UTMMedium: utmMedium, UTMCampaign: utmCampaign,
			DeviceType: e.DeviceType, CountryCode: country, Environment: env,
			IP: fmt.Sprintf("192.0.2.%d", 10+i), City: goldenCities[i%len(goldenCities)],
			Region: "R" + country, Latitude: 52.0 + float64(i%5), Longitude: 4.0 + float64(i%3),
			Timezone: "Europe/Amsterdam", ConnectionClass: "broadband",
		}
		if err := store.UpsertSession(ctx, sess); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	if i%3 == 0 {
		w, h, dark, touch := 1440+i, 900, i%2 == 0, i%2 == 1
		err := store.UpsertSessionSignals(ctx, project, sid, repository.SessionSignals{
			ScreenWidth: &w, ScreenHeight: &h, DarkMode: &dark, Touch: &touch, BrowserTimezone: "Europe/Amsterdam",
		})
		if err != nil {
			t.Fatalf("seed signals: %v", err)
		}
	}
	return sid
}

// seedEventsA builds 40 sessions for the shop: a purchase funnel with
// drop-off at each step, A/B variant properties, signups and CTA clicks.
func seedEventsA(t *testing.T, store *repository.Store, s *goldenSeed) {
	t.Helper()
	for i := 0; i < 40; i++ {
		variant := []string{"a", "b"}[i%2]
		plan := goldenPlans[i%len(goldenPlans)]
		vp := fmt.Sprintf(`{"variant":%q,"plan":%q}`, variant, plan)
		pv := func(n int) string { return fmt.Sprintf("pv-A-%02d-%d", i, n) }
		evs := []goldenEvent{{"page_view", "/", vp, pv(0), 0}}
		if i%2 == 0 {
			evs = append(evs, goldenEvent{"page_view", "/pricing", vp, pv(1), 30 * time.Second})
		}
		if i%4 == 1 {
			evs = append(evs, goldenEvent{"click_cta", "/", vp, pv(0), 45 * time.Second})
		}
		if i%3 == 0 {
			evs = append(evs, goldenEvent{"add_to_cart", "/products/widget", vp, pv(2), time.Minute})
			if i%2 == 0 {
				evs = append(evs, goldenEvent{"checkout_start", "/checkout", vp, pv(3), 2 * time.Minute})
				if i%6 == 0 {
					evs = append(evs, goldenEvent{"purchase", "/thanks",
						fmt.Sprintf(`{"variant":%q,"plan":%q,"amount":%d}`, variant, plan, 20+i), pv(4), 3 * time.Minute})
				}
			}
		}
		if i%5 == 0 {
			evs = append(evs, goldenEvent{"signup", "/signup", vp, pv(5), 90 * time.Second})
		}
		insertSession(t, store, s.ProjectA, "acme.example", "A", i, evs)
	}
}

// seedEventsB builds 12 sessions for the blog.
func seedEventsB(t *testing.T, store *repository.Store, s *goldenSeed) {
	t.Helper()
	for i := 0; i < 12; i++ {
		plan := goldenPlans[i%len(goldenPlans)]
		props := fmt.Sprintf(`{"plan":%q}`, plan)
		article := fmt.Sprintf("/blog/post-%d", i%4)
		evs := []goldenEvent{{"page_view", article, props, fmt.Sprintf("pv-B-%02d-0", i), 0}}
		if i%2 == 0 {
			evs = append(evs, goldenEvent{"page_view", "/blog/", props, fmt.Sprintf("pv-B-%02d-1", i), time.Minute})
		}
		if i%3 == 0 {
			evs = append(evs, goldenEvent{"signup", "/signup", props, fmt.Sprintf("pv-B-%02d-2", i), 2 * time.Minute})
		}
		insertSession(t, store, s.ProjectB, "beta.example", "B", i, evs)
	}
}

// goldenQuery appends the explicit window to a path.
func (s *goldenSeed) window(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "from=" + s.From + "&to=" + s.To
}
