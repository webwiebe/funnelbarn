package mock

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func (s *Store) FlagContextKeySuggestions(_ context.Context, _ string) ([]repository.ContextKeySuggestion, error) {
	return nil, nil
}

func (s *Store) AnonymizeSessionGeo(_ context.Context, _ string) error { return nil }

func (s *Store) SessionDistributions(_ context.Context, _ string) (map[string][]repository.DistributionEntry, error) {
	return map[string][]repository.DistributionEntry{}, nil
}

func (s *Store) CreateSegment(_ context.Context, seg repository.Segment) (repository.Segment, error) {
	return seg, nil
}
func (s *Store) SegmentByID(_ context.Context, _ string) (repository.Segment, error) {
	return repository.Segment{}, nil
}
func (s *Store) ListSegments(_ context.Context, _ string) ([]repository.Segment, error) {
	return nil, nil
}
func (s *Store) UpdateSegment(_ context.Context, seg repository.Segment) (repository.Segment, error) {
	return seg, nil
}
func (s *Store) DeleteSegment(_ context.Context, _ string) error { return nil }
func (s *Store) UpsertSessionSignals(_ context.Context, _ string, _ repository.SessionSignals) error {
	return nil
}

func (s *Store) AnonymizeSessionsByIP(_ context.Context, _ string) (int64, error) { return 0, nil }

func (s *Store) GetInstanceSetting(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (s *Store) SetInstanceSetting(_ context.Context, _, _ string) error { return nil }

func (s *Store) GetAllInstanceSettings(_ context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *Store) UpsertRecording(_ context.Context, _ repository.Recording) error { return nil }
func (s *Store) GetRecording(_ context.Context, _ string) (repository.Recording, error) {
	return repository.Recording{}, nil
}
func (s *Store) ListRecordings(_ context.Context, _ string, _ repository.RecordingListOpts) ([]repository.Recording, error) {
	return nil, nil
}
func (s *Store) ListOldRecordings(_ context.Context, _ time.Time) ([]repository.Recording, error) {
	return nil, nil
}
func (s *Store) DeleteRecording(_ context.Context, _ string) error { return nil }
func (s *Store) FlagEvaluationsForSession(_ context.Context, _, _ string) ([]repository.FlagEvaluationEntry, error) {
	return nil, nil
}

func (s *Store) GetProjectHealth(_ context.Context, projectID string) (repository.ProjectHealth, error) {
	return repository.ProjectHealth{ProjectID: projectID}, nil
}
func (s *Store) MarkProjectHealthSetupCalled(_ context.Context, _ string) error        { return nil }
func (s *Store) MarkProjectHealthEventsReceived(_ context.Context, _ string) error     { return nil }
func (s *Store) MarkProjectHealthFlagsEvaluated(_ context.Context, _ string) error     { return nil }
func (s *Store) MarkProjectHealthRecordingsReceived(_ context.Context, _ string) error { return nil }
func (s *Store) ResetProjectHealth(_ context.Context, _ string) error                  { return nil }
