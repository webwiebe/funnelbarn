package service_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// snapshotChunk is a full snapshot with a blocked email input, a masked
// password input, a readable text input, an empty text input and a textarea
// holding readable text, plus a blocked wrapper.
const snapshotChunk = `[{"type":2,"timestamp":1,"data":{"node":{"id":1,"type":0,"childNodes":[
 {"id":2,"type":2,"tagName":"form","attributes":{},"childNodes":[
  {"id":3,"type":2,"tagName":"input","attributes":{"type":"email","class":"fb-block","rr_width":"200px","rr_height":"30px"},"childNodes":[]},
  {"id":4,"type":2,"tagName":"input","attributes":{"type":"password","value":"*********"},"childNodes":[]},
  {"id":5,"type":2,"tagName":"input","attributes":{"type":"text","value":"visible name"},"childNodes":[]},
  {"id":6,"type":2,"tagName":"input","attributes":{"type":"text"},"childNodes":[]},
  {"id":7,"type":2,"tagName":"textarea","attributes":{},"childNodes":[{"id":8,"type":3,"textContent":"free text"}]},
  {"id":9,"type":2,"tagName":"div","attributes":{"class":"fb-block","rr_width":"10px","rr_height":"10px"},"childNodes":[]}
 ]}]}}}]`

const typingChunk = `[
 {"type":3,"timestamp":2,"data":{"source":5,"id":4,"text":"***"}},
 {"type":3,"timestamp":3,"data":{"source":5,"id":5,"text":"abc"}},
 {"type":3,"timestamp":4,"data":{"source":5,"id":99,"text":"zzz"}},
 {"type":3,"timestamp":5,"data":{"source":0,"adds":[{"parentId":2,"nextId":null,"node":{"id":20,"type":2,"tagName":"input","attributes":{"type":"tel","value":"0612345678"},"childNodes":[]}}]}},
 {"type":3,"timestamp":6,"data":{"source":5,"id":20,"text":"***"}}
]`

func TestCheckInputs_ClassifiesFieldsAndTypingWithoutValues(t *testing.T) {
	svc, _ := seedRecordings(t, 1, [][]byte{gz(t, snapshotChunk), gz(t, typingChunk)})

	res, err := svc.CheckInputs(context.Background(), "rec-big-000")
	require.NoError(t, err)

	assert.Equal(t, 2, res.Chunks)
	assert.Equal(t, 0, res.Unreadable)
	assert.Equal(t, 1, res.UnmatchedEvents)
	assert.Equal(t, 2, res.BlockedElements)
	assert.Equal(t, []string{"email", "password", "tel", "text", "textarea"}, res.Types())
	assert.Equal(t, service.InputCounts{Blocked: 1}, *res.Inputs["email"])
	assert.Equal(t, service.InputCounts{Masked: 1, EventsMasked: 1}, *res.Inputs["password"])
	assert.Equal(t, service.InputCounts{Empty: 1, Readable: 1, EventsReadable: 1}, *res.Inputs["text"])
	assert.Equal(t, service.InputCounts{Readable: 1}, *res.Inputs["textarea"])
	assert.Equal(t, service.InputCounts{Readable: 1, EventsMasked: 1}, *res.Inputs["tel"])
	assert.Equal(t, 4, res.ReadableTotal())

	out, err := json.Marshal(res)
	require.NoError(t, err)
	for _, secret := range []string{"visible name", "free text", "0612345678", "abc", "zzz"} {
		assert.NotContains(t, string(out), secret)
	}
}

func TestCheckInputs_CountsAnUnreadableChunkAndRefusesAnUnknownRecording(t *testing.T) {
	svc, _ := seedRecordings(t, 1, [][]byte{[]byte("not gzip")})

	res, err := svc.CheckInputs(context.Background(), "rec-big-000")
	require.NoError(t, err)
	assert.Equal(t, 1, res.Chunks)
	assert.Equal(t, 1, res.Unreadable)

	_, err = svc.CheckInputs(context.Background(), "missing")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
