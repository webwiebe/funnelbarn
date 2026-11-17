package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

type fakeChecker map[string]service.InputCheck

func (f fakeChecker) CheckInputs(_ context.Context, id string, _ ...string) (service.InputCheck, error) {
	r, ok := f[id]
	if !ok {
		return service.InputCheck{}, errors.New("unknown")
	}
	return r, nil
}

func TestParseCheckInputsFlags(t *testing.T) {
	f, err := parseCheckInputsFlags([]string{"--id", "a", "--id", "b", "--contains", "x y"})
	require.NoError(t, err)
	assert.Equal(t, idList{"a", "b"}, f.ids)
	assert.Equal(t, idList{"x y"}, f.needles)

	for _, bad := range [][]string{{}, {"--id", "a", "extra"}, {"--nope"}} {
		_, err := parseCheckInputsFlags(bad)
		assert.ErrorContains(t, err, "usage: funnelbarn recordings check-inputs", "%v", bad)
	}
}

func TestWriteInputChecks_OneJSONLinePerRecording(t *testing.T) {
	checker := fakeChecker{
		"a": {RecordingID: "a", Chunks: 2, Inputs: map[string]*service.InputCounts{"email": {Blocked: 1}}},
		"b": {RecordingID: "b", Chunks: 1, Inputs: map[string]*service.InputCounts{}},
	}
	var out bytes.Buffer
	require.NoError(t, writeInputChecks(context.Background(), &out, checker, []string{"a", "b"}, nil))
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)
	assert.Contains(t, string(lines[0]), `"recording_id":"a"`)
	assert.Contains(t, string(lines[0]), `"email":{"blocked":1`)

	err := writeInputChecks(context.Background(), &out, checker, []string{"zz"}, nil)
	assert.ErrorContains(t, err, "zz")
}
