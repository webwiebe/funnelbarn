package service

import (
	"reflect"
	"testing"
)

func TestContextKeyNamesAreSorted(t *testing.T) {
	ctx := map[string]any{"plan": "pro", "country": "NL", "user_id": "u1", "beta": true}
	want := []string{"beta", "country", "plan", "user_id"}
	for i := 0; i < 20; i++ {
		if got := contextKeyNames(ctx); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if contextKeyNames(nil) != nil {
		t.Fatal("empty context must give nil")
	}
}
