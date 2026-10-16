package repository

import (
	"reflect"
	"testing"
)

func TestSortedSessionIDs(t *testing.T) {
	ids := map[string]bool{"c": true, "a": true, "d": true, "b": true}
	for i := 0; i < 20; i++ {
		if got := sortedSessionIDs(ids, 3); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
			t.Fatalf("limit 3: got %v", got)
		}
	}
	if got := sortedSessionIDs(ids, 0); len(got) != 4 || got[0] != "a" {
		t.Fatalf("no limit: got %v", got)
	}
	if got := sortedSessionIDs(nil, 5); got == nil || len(got) != 0 {
		t.Fatalf("empty input must give an empty, non-nil slice, got %#v", got)
	}
}

func TestBuildFlowResultOrderIsStable(t *testing.T) {
	transitions := []flowTransition{
		{SourceURL: "/", SourceDepth: 0, TargetURL: "/b", TargetDepth: 1, Sessions: 5},
		{SourceURL: "/", SourceDepth: 0, TargetURL: "/a", TargetDepth: 1, Sessions: 5},
		{SourceURL: "/", SourceDepth: 0, TargetURL: "/c", TargetDepth: 1, Sessions: 9},
	}
	want := buildFlowResult("/", 20, 1, transitions, nil)
	for i := 0; i < 20; i++ {
		got := buildFlowResult("/", 20, 1, transitions, nil)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d differs:\n%+v\n%+v", i, got, want)
		}
	}
	// Heaviest first; equal weights break ties by source then target.
	if want.Links[0].Target != flowNodeID("/c", 1) || want.Links[1].Target != flowNodeID("/a", 1) {
		t.Fatalf("links not ordered by weight then key: %+v", want.Links)
	}
	if want.Nodes[0].Sessions != 20 {
		t.Fatalf("nodes not ordered by sessions: %+v", want.Nodes)
	}
}
