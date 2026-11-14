package api

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestParseRecordingListOpts(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  domain.RecordingListOpts
	}{
		{name: "defaults", query: "", want: domain.RecordingListOpts{Limit: 50}},
		{name: "limit and offset", query: "limit=20&offset=40", want: domain.RecordingListOpts{Limit: 20, Offset: 40}},
		{name: "limit at the cap", query: "limit=500", want: domain.RecordingListOpts{Limit: 500}},
		{name: "limit over the cap keeps the default", query: "limit=501", want: domain.RecordingListOpts{Limit: 50}},
		{name: "zero limit keeps the default", query: "limit=0", want: domain.RecordingListOpts{Limit: 50}},
		{name: "negative offset keeps the default", query: "offset=-1", want: domain.RecordingListOpts{Limit: 50}},
		{name: "non-numeric values keep the defaults", query: "limit=ten&offset=x", want: domain.RecordingListOpts{Limit: 50}},
		{
			name:  "filters",
			query: "environment=production&device_type=mobile&page_url=%2Fcheckout&human_only=true",
			want: domain.RecordingListOpts{Limit: 50, Environment: "production", DeviceType: "mobile",
				PageURL: "/checkout", HumanOnly: true},
		},
		{name: "human_only other than true", query: "human_only=1", want: domain.RecordingListOpts{Limit: 50}},
		{
			name:  "session ids trimmed, empties dropped",
			query: "session_ids=" + url.QueryEscape(" a , ,b,"),
			want:  domain.RecordingListOpts{Limit: 50, SessionIDs: []string{"a", "b"}},
		},
		{name: "session ids all empty", query: "session_ids=" + url.QueryEscape(" , "), want: domain.RecordingListOpts{Limit: 50}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if got := parseRecordingListOpts(q); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseRecordingListOpts(%q) = %+v, want %+v", tc.query, got, tc.want)
			}
		})
	}
}
