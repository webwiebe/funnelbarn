package api

// Golden cases for the mutating routes. Every mutation is followed by a
// read-back GET that pins the persisted effect, so a refactor that moves writes
// behind a command dispatcher or a second connection must still make the write
// visible to the next read. Created rows get random ids; those are masked in
// the responses and carried between cases as $variables (goldenCase.Save).
//
// Mutations with no persisted effect that an HTTP read can see are noted at
// the case: telemetry and client-error reports are relayed or logged only, and
// the accepted ingest event lands in the on-disk spool, which only the worker
// reads (the state snapshot suite in statesnap_*_test.go covers the spool).

import "net/http"

// mutBuilder collects cases in order, all in phaseMutate.
type mutBuilder struct {
	cs []goldenCase
}

func (b *mutBuilder) add(c goldenCase) {
	c.Phase = phaseMutate
	b.cs = append(b.cs, c)
}

// write adds a session-authed mutation expecting status.
func (b *mutBuilder) write(name, method, path string, body any, status int, save map[string]string) {
	b.add(goldenCase{Name: name, Method: method, Path: path, Auth: authSession, Body: body, WantStatus: status, Save: save})
}

// read adds a session-authed GET expecting 200.
func (b *mutBuilder) read(name, path string) {
	b.add(goldenCase{Name: name, Method: http.MethodGet, Path: path, Auth: authSession})
}

// readStatus adds a session-authed GET expecting status.
func (b *mutBuilder) readStatus(name, path string, status int) {
	b.add(goldenCase{Name: name, Method: http.MethodGet, Path: path, Auth: authSession, WantStatus: status})
}

// goldenMutationCases returns every mutating case after the evaluate phase.
func goldenMutationCases(s *goldenSeed) []goldenCase {
	b := &mutBuilder{}
	goldenAuthMutations(b)
	goldenIngestMutations(b, s)
	goldenProjectMutations(b)
	goldenAPIKeyMutations(b, s)
	goldenSettingsMutations(b, s)
	goldenMiscMutations(b, s)
	goldenFunnelMutations(b, s)
	goldenFlagMutations(b, s)
	goldenSegmentWidgetMutations(b, s)
	goldenABTestMutations(b, s)
	goldenCanonicalMutations(b, s)
	return b.cs
}

func goldenAuthMutations(b *mutBuilder) {
	creds := map[string]any{"username": "admin", "password": "password"}
	b.add(goldenCase{Name: "login", Method: "POST", Path: "/api/v1/login", Auth: authNone, Body: creds, WantStatus: http.StatusOK})
	b.add(goldenCase{Name: "login_wrong_password", Method: "POST", Path: "/api/v1/login", Auth: authNone,
		Body: map[string]any{"username": "admin", "password": "nope"}, WantStatus: http.StatusUnauthorized})
	// The session row a login creates is only usable through the cookie the
	// response sets; the harness keeps its own sessions, so the read-back
	// confirms the dashboard session still resolves after a second login.
	b.read("me_after_login", "/api/v1/me")

	// Logout revokes the spare session; the read-back with it is a 401.
	b.add(goldenCase{Name: "me_spare_session_before_logout", Method: "GET", Path: "/api/v1/me", Auth: authSpare})
	b.add(goldenCase{Name: "logout", Method: "POST", Path: "/api/v1/logout", Auth: authSpare, WantStatus: http.StatusOK})
	b.add(goldenCase{Name: "me_spare_session_after_logout", Method: "GET", Path: "/api/v1/me", Auth: authSpare, WantStatus: http.StatusUnauthorized})
	b.add(goldenCase{Name: "backchannel_logout_without_oidc", Method: "POST", Path: "/api/v1/oidc/backchannel-logout", Auth: authNone, WantStatus: http.StatusNotFound})
}

func goldenIngestMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	b.add(goldenCase{Name: "ingest_event", Method: "POST", Path: "/api/v1/events", Auth: authKey, Key: s.KeyAIngest, WantStatus: http.StatusAccepted,
		Body: map[string]any{"name": "golden_ingest", "url": "https://acme.example/pricing", "properties": map[string]any{"plan": "pro"}}})
	b.add(goldenCase{Name: "ingest_event_bad_key", Method: "POST", Path: "/api/v1/events", Auth: authKey, Key: "gk-wrong", WantStatus: http.StatusUnauthorized,
		Body: map[string]any{"name": "golden_ingest"}})
	chunk := map[string]any{"recording_id": "rec-A-03", "session_id": "sess-A-03", "chunk_index": 0,
		"events":     []any{map[string]any{"type": 4, "data": map[string]any{"href": "https://acme.example/"}, "timestamp": 1772452800000}},
		"started_at": "2026-03-02T12:00:00Z", "duration_ms": 4000, "page_url": "https://acme.example/"}
	b.add(goldenCase{Name: "ingest_recording_chunk", Method: "POST", Path: "/api/v1/recordings/chunk", Auth: authKey, Key: s.KeyAFull, Body: chunk, WantStatus: http.StatusAccepted})
	b.add(goldenCase{Name: "ingest_recording_chunk_bad_id", Method: "POST", Path: "/api/v1/recordings/chunk", Auth: authKey, Key: s.KeyAFull, WantStatus: http.StatusUnprocessableEntity,
		Body: map[string]any{"recording_id": "../escape", "session_id": "sess-A-03", "events": []any{1}}})
	b.read("recordings_a_after_chunk", pa+"/recordings")
	b.read("recording_chunk_after_ingest", pa+"/recordings/rec-A-03/chunks/0")
	// Ingest and chunk flip health flags off the request goroutine.
	b.add(goldenCase{Name: "project_health_a_after_ingest", Method: "GET", Path: pa + "/health", Auth: authSession, Await: awaitIngest})
	b.write("project_health_a_reset", "POST", pa+"/health/reset", nil, http.StatusNoContent, nil)
	b.read("project_health_a_after_reset", pa+"/health")
}

func goldenProjectMutations(b *mutBuilder) {
	b.write("project_create", "POST", "/api/v1/projects", map[string]any{"name": "Gamma Shop", "domain": "gamma.example"},
		http.StatusCreated, map[string]string{"newProject": "id"})
	b.read("projects_after_create", "/api/v1/projects")
	b.write("project_update", "PUT", "/api/v1/projects/$newProject", map[string]any{"name": "Gamma Store", "domain": "store.gamma.example"}, http.StatusOK, nil)
	b.read("projects_after_update", "/api/v1/projects")
	b.write("project_approve", "POST", "/api/v1/projects/$newProject/approve", nil, http.StatusOK, nil)
	b.read("projects_after_approve", "/api/v1/projects")
	b.write("project_update_missing_name", "PUT", "/api/v1/projects/$newProject", map[string]any{"domain": "x.example"}, http.StatusBadRequest, nil)
	b.write("project_delete", "DELETE", "/api/v1/projects/$newProject", nil, http.StatusNoContent, nil)
	b.read("projects_after_delete", "/api/v1/projects")
}

func goldenAPIKeyMutations(b *mutBuilder, s *goldenSeed) {
	b.write("apikey_create", "POST", "/api/v1/apikeys", map[string]any{"project_id": s.ProjectA, "name": "golden-new", "scope": "ingest"},
		http.StatusCreated, map[string]string{"newKeyID": "api_key.id", "newKey": "key"})
	b.write("apikey_create_missing_project", "POST", "/api/v1/apikeys", map[string]any{"name": "orphan"}, http.StatusBadRequest, nil)
	b.add(goldenCase{Name: "apikeys_after_create", Method: http.MethodGet, Path: "/api/v1/apikeys?project_id=" + s.ProjectA, Auth: authSession, Await: awaitAPIKeys})
	eval := map[string]any{"flag_key": "legacy_search", "default_value": true, "context": map[string]any{}}
	b.add(goldenCase{Name: "apikey_new_key_works", Method: "POST", Path: "/api/v1/evaluate", Auth: authKey, Key: "$newKey", Body: eval})
	b.write("apikey_delete", "DELETE", "/api/v1/apikeys/$newKeyID", nil, http.StatusNoContent, nil)
	b.add(goldenCase{Name: "apikeys_after_delete", Method: http.MethodGet, Path: "/api/v1/apikeys?project_id=" + s.ProjectA, Auth: authSession, Await: awaitAPIKeys})
	b.add(goldenCase{Name: "apikey_deleted_key_rejected", Method: "POST", Path: "/api/v1/evaluate", Auth: authKey, Key: "$newKey", Body: eval, WantStatus: http.StatusUnauthorized})
}

func goldenSettingsMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	b.write("instance_settings_update", "PUT", "/api/v1/instance-settings", map[string]any{"retention_days": "60"}, http.StatusOK, nil)
	b.read("instance_settings_after_update", "/api/v1/instance-settings")
	b.write("recording_settings_update", "PUT", pa+"/recording-settings",
		map[string]any{"enabled": false, "sample_rate": 0.1, "rules": []any{map[string]any{"pattern": "/admin*", "action": "ignore"}}}, http.StatusNoContent, nil)
	b.write("recording_settings_bad_rate", "PUT", pa+"/recording-settings", map[string]any{"sample_rate": 2}, http.StatusBadRequest, nil)
	b.read("recording_settings_after_update", pa+"/recording-settings")
	b.add(goldenCase{Name: "recording_config_after_settings", Method: "GET", Path: "/api/v1/recording-config", Auth: authKey, Key: s.KeyAFull})
}

func goldenMiscMutations(b *mutBuilder, s *goldenSeed) {
	pa := "/api/v1/projects/" + s.ProjectA
	b.write("anonymize_geo_session", "POST", "/api/v1/admin/anonymize-geo", map[string]any{"session_id": "sess-A-00"}, http.StatusOK, nil)
	b.write("anonymize_geo_empty", "POST", "/api/v1/admin/anonymize-geo", map[string]any{}, http.StatusUnprocessableEntity, nil)
	b.read("sessions_a_after_anonymize", pa+"/sessions?limit=5")
	b.write("recording_delete", "DELETE", pa+"/recordings/"+s.RecordingA2, nil, http.StatusNoContent, nil)
	b.read("recordings_a_after_delete", pa+"/recordings")
	// No persisted effect an HTTP read can see: spans go to the SpanBarn relay
	// and client errors to the error log. The status is the contract.
	b.write("telemetry_empty_batch", "POST", "/api/v1/telemetry", map[string]any{"spans": []any{}}, http.StatusAccepted, nil)
	b.write("client_error_report", "POST", "/api/v1/client-errors", map[string]any{"message": "golden client error", "type": "TypeError"}, http.StatusAccepted, nil)
	b.write("client_error_missing_message", "POST", "/api/v1/client-errors", map[string]any{}, http.StatusUnprocessableEntity, nil)
}
