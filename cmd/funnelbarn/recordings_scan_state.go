package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// scanStateKey is the object that keeps the finished projects' counts between
// runs of recordings scan-url-params. It holds counts, times and URL shapes
// (host, masked path, parameter name): no keys, no event content, no value.
const scanStateKey = "scans/url-params/state.json"

// scanObjects is the part of object storage the scan state needs.
type scanObjects interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, data []byte) error
}

// scanState keeps the final line of every project a run finished, so a run
// that is cut off by its time budget leaves its totals behind and the next run
// continues with the projects that are left.
type scanState struct {
	SavedAt  time.Time           `json:"saved_at"`
	Projects map[string]scanLine `json:"projects"`
}

func (s *scanState) done(slug string) (scanLine, bool) {
	if s == nil {
		return scanLine{}, false
	}
	line, ok := s.Projects[slug]
	return line, ok
}

func (s *scanState) record(slug string, line scanLine) {
	if s.Projects == nil {
		s.Projects = map[string]scanLine{}
	}
	line.Progress, line.Stopped, line.FromPreviousRun, line.ElapsedSeconds = false, "", false, 0
	s.Projects[slug] = line
	s.SavedAt = time.Now().UTC()
}

// loadScanState reads the saved state. A missing object is an empty state; any
// other failure is an error, so a storage hiccup cannot start over silently and
// overwrite the saved totals.
func loadScanState(ctx context.Context, objects scanObjects) (*scanState, error) {
	data, err := objects.Get(ctx, scanStateKey)
	if err != nil {
		if isObjectMissing(err) {
			return &scanState{}, nil
		}
		return nil, fmt.Errorf("recordings scan: load state: %w", err)
	}
	var s scanState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("recordings scan: state object is not valid: %w", err)
	}
	return &s, nil
}

func saveScanState(ctx context.Context, objects scanObjects, s *scanState) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return objects.Put(ctx, scanStateKey, data)
}

func isObjectMissing(err error) bool {
	var noSuchKey *types.NoSuchKey
	var notFound *types.NotFound
	return errors.As(err, &noSuchKey) || errors.As(err, &notFound)
}
