package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

var errBoom = errors.New("boom")

type fakeInstanceSettings struct {
	values map[string]string
	err    error
}

func (f *fakeInstanceSettings) GetInstanceSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := f.values[key]
	return v, ok, f.err
}

func (f *fakeInstanceSettings) GetAllInstanceSettings(context.Context) (map[string]string, error) {
	return f.values, f.err
}

func (f *fakeInstanceSettings) SetInstanceSetting(_ context.Context, key, value string) error {
	if f.err != nil {
		return f.err
	}
	f.values[key] = value
	return nil
}

func TestInstanceSettingsService(t *testing.T) {
	ctx := context.Background()
	fake := &fakeInstanceSettings{values: map[string]string{}}
	svc := service.NewInstanceSettingsService(fake)

	require.NoError(t, svc.SetInstanceSetting(ctx, "recording_enabled", "true"))
	got, err := svc.GetAllInstanceSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"recording_enabled": "true"}, got)

	fake.err = errBoom
	assert.ErrorIs(t, svc.SetInstanceSetting(ctx, "k", "v"), errBoom)
	_, err = svc.GetAllInstanceSettings(ctx)
	assert.ErrorIs(t, err, errBoom)
}

type fakeRecordingSettings struct {
	stored *repository.ProjectRecordingSettings
	err    error
}

func (f *fakeRecordingSettings) GetProjectRecordingSettings(context.Context, string) (*repository.ProjectRecordingSettings, error) {
	return f.stored, f.err
}

func (f *fakeRecordingSettings) UpsertProjectRecordingSettings(_ context.Context, s *repository.ProjectRecordingSettings) error {
	if f.err != nil {
		return f.err
	}
	f.stored = s
	return nil
}

func TestRecordingSettingsService(t *testing.T) {
	ctx := context.Background()
	fake := &fakeRecordingSettings{}
	svc := service.NewRecordingSettingsService(fake)

	enabled, rate := true, 0.5
	in := &repository.ProjectRecordingSettings{ProjectID: "p1", Enabled: &enabled, SampleRate: &rate}
	require.NoError(t, svc.UpsertProjectRecordingSettings(ctx, in))
	got, err := svc.GetProjectRecordingSettings(ctx, "p1")
	require.NoError(t, err)
	assert.Same(t, in, got)

	fake.err = errBoom
	assert.ErrorIs(t, svc.UpsertProjectRecordingSettings(ctx, in), errBoom)
	_, err = svc.GetProjectRecordingSettings(ctx, "p1")
	assert.ErrorIs(t, err, errBoom)
}

type fakeGeoAnonymizer struct {
	sessions []string
	ips      []string
	count    int64
	err      error
}

func (f *fakeGeoAnonymizer) AnonymizeSessionGeo(_ context.Context, id string) error {
	f.sessions = append(f.sessions, id)
	return f.err
}

func (f *fakeGeoAnonymizer) AnonymizeSessionsByIP(_ context.Context, ip string) (int64, error) {
	f.ips = append(f.ips, ip)
	return f.count, f.err
}

func TestPrivacyService(t *testing.T) {
	ctx := context.Background()
	fake := &fakeGeoAnonymizer{count: 3}
	svc := service.NewPrivacyService(fake)

	require.NoError(t, svc.AnonymizeSessionGeo(ctx, "s1"))
	n, err := svc.AnonymizeSessionsByIP(ctx, "1.2.3.4")
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)
	assert.Equal(t, []string{"s1"}, fake.sessions)
	assert.Equal(t, []string{"1.2.3.4"}, fake.ips)

	fake.err = errBoom
	assert.ErrorIs(t, svc.AnonymizeSessionGeo(ctx, "s2"), errBoom)
	_, err = svc.AnonymizeSessionsByIP(ctx, "5.6.7.8")
	assert.ErrorIs(t, err, errBoom)
}

type fakeWebSessions struct {
	calls []string
	ws    repository.WebSession
	n     int64
	err   error
}

func (f *fakeWebSessions) rec(name string) { f.calls = append(f.calls, name) }

func (f *fakeWebSessions) CreateWebSession(_ context.Context, ws repository.WebSession) error {
	f.rec("create")
	f.ws = ws
	return f.err
}

func (f *fakeWebSessions) GetWebSession(context.Context, string) (repository.WebSession, error) {
	f.rec("get")
	return f.ws, f.err
}

func (f *fakeWebSessions) UpdateWebSessionTokens(context.Context, string, string, string, string, int64, string, int64) error {
	f.rec("update")
	return f.err
}

func (f *fakeWebSessions) MarkWebSessionRefreshFailing(context.Context, string, int64) error {
	f.rec("mark")
	return f.err
}

func (f *fakeWebSessions) DeleteWebSession(context.Context, string) error {
	f.rec("delete")
	return f.err
}

func (f *fakeWebSessions) DeleteWebSessionsByIdpSid(context.Context, string) (int64, error) {
	f.rec("delete_sid")
	return f.n, f.err
}

func (f *fakeWebSessions) DeleteWebSessionsByIdpSub(context.Context, string) (int64, error) {
	f.rec("delete_sub")
	return f.n, f.err
}

func TestWebSessionService_OneRepoCallPerMethod(t *testing.T) {
	ctx := context.Background()
	fake := &fakeWebSessions{n: 2}
	svc := service.NewWebSessionService(fake)

	require.NoError(t, svc.CreateWebSession(ctx, repository.WebSession{IDHash: "h"}))
	got, err := svc.GetWebSession(ctx, "h")
	require.NoError(t, err)
	assert.Equal(t, "h", got.IDHash)
	require.NoError(t, svc.UpdateWebSessionTokens(ctx, "h", "i", "a", "r", 1, "{}", 2))
	require.NoError(t, svc.MarkWebSessionRefreshFailing(ctx, "h", 3))
	require.NoError(t, svc.DeleteWebSession(ctx, "h"))
	n, err := svc.DeleteWebSessionsByIdpSid(ctx, "sid")
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)
	n, err = svc.DeleteWebSessionsByIdpSub(ctx, "sub")
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	assert.Equal(t, []string{"create", "get", "update", "mark", "delete", "delete_sid", "delete_sub"}, fake.calls)
}

func TestWebSessionService_ErrorsPassThroughUnchanged(t *testing.T) {
	ctx := context.Background()
	fake := &fakeWebSessions{err: errBoom}
	svc := service.NewWebSessionService(fake)

	assert.Same(t, errBoom, svc.CreateWebSession(ctx, repository.WebSession{}))
	_, err := svc.GetWebSession(ctx, "h")
	assert.Same(t, errBoom, err)
	assert.Same(t, errBoom, svc.UpdateWebSessionTokens(ctx, "h", "", "", "", 0, "", 0))
	assert.Same(t, errBoom, svc.MarkWebSessionRefreshFailing(ctx, "h", 0))
	assert.Same(t, errBoom, svc.DeleteWebSession(ctx, "h"))
	_, err = svc.DeleteWebSessionsByIdpSid(ctx, "x")
	assert.Same(t, errBoom, err)
	_, err = svc.DeleteWebSessionsByIdpSub(ctx, "x")
	assert.Same(t, errBoom, err)
}

type fakeIAMBarnUsers struct {
	user repository.User
	err  error
}

func (f *fakeIAMBarnUsers) FindUserByIAMBarnSub(context.Context, string) (repository.User, error) {
	return f.user, f.err
}

func (f *fakeIAMBarnUsers) CreateIAMBarnUser(_ context.Context, sub, username string) (repository.User, error) {
	return repository.User{Username: username, IAMBarnSub: sub}, f.err
}

func TestIAMBarnUserService(t *testing.T) {
	ctx := context.Background()
	fake := &fakeIAMBarnUsers{user: repository.User{Username: "found"}}
	svc := service.NewIAMBarnUserService(fake)

	created, err := svc.CreateIAMBarnUser(ctx, "sub-1", "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", created.Username)
	found, err := svc.FindUserByIAMBarnSub(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "found", found.Username)

	fake.err = errBoom
	_, err = svc.CreateIAMBarnUser(ctx, "s", "u")
	assert.ErrorIs(t, err, errBoom)
	_, err = svc.FindUserByIAMBarnSub(ctx, "s")
	assert.ErrorIs(t, err, errBoom)
}

func TestSessionService_SessionDistributions(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	p, err := service.NewProjectService(store).CreateProject(ctx, "Dist", "dist-project")
	require.NoError(t, err)

	got, err := service.NewSessionService(store).SessionDistributions(ctx, p.ID)
	require.NoError(t, err)
	assert.NotNil(t, got)
}
