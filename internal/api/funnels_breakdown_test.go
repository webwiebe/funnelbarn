package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestHandleFunnelAnalysis_Breakdown(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FBreak", "fbreak")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Surface Funnel",
		Steps:     []repository.FunnelStep{{EventName: "visit"}, {EventName: "signup"}},
	})
	at := time.Now().UTC().Add(-time.Hour)
	insert := func(id, session, name, props string) {
		t.Helper()
		if err := store.InsertEvent(ctx, repository.Event{
			ID: "ev-" + id, ProjectID: p.ID, SessionID: session, Name: name,
			Properties: props, IngestID: "ing-" + id, OccurredAt: at, Environment: "production",
		}); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}
	insert("a", "s1", "visit", `{"surface":"wordpress"}`)
	insert("b", "s1", "signup", `{}`)
	insert("c", "s2", "visit", `{"surface":"lightroom"}`)
	insert("d", "s3", "visit", `{}`)

	base := "/api/v1/projects/" + p.ID + "/funnels/" + f.ID + "/analysis"

	var plain map[string]json.RawMessage
	w := getJSON(t, srv, base, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("plain: want 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &plain); err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["breakdown"]; ok {
		t.Errorf("breakdown present without ?breakdown=")
	}

	w = getJSON(t, srv, base+"?breakdown=surface", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("breakdown: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Breakdown struct {
			Property string `json:"property"`
			Groups   []struct {
				Value   string                        `json:"value"`
				Label   string                        `json:"label"`
				Entries int64                         `json:"entries"`
				Results []repository.FunnelStepResult `json:"results"`
			} `json:"groups"`
		} `json:"breakdown"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Breakdown.Property != "surface" || len(resp.Breakdown.Groups) != 3 {
		t.Fatalf("breakdown = %+v", resp.Breakdown)
	}
	got := map[string][2]int64{}
	for _, g := range resp.Breakdown.Groups {
		got[g.Label] = [2]int64{g.Results[0].Count, g.Results[1].Count}
	}
	want := map[string][2]int64{"wordpress": {1, 1}, "lightroom": {1, 0}, "(none)": {1, 0}}
	for label, counts := range want {
		if got[label] != counts {
			t.Errorf("group %q = %v, want %v", label, got[label], counts)
		}
	}
}

func TestHandleFunnelAnalysis_BreakdownRejectsBadProperty(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "FBreakBad", "fbreakbad")
	f, _ := store.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID, Name: "F", Steps: []repository.FunnelStep{{EventName: "visit"}},
	})
	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/funnels/"+f.ID+"/analysis?breakdown=a%27)%20OR%201=1", nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d (body: %s)", w.Code, w.Body.String())
	}
}
