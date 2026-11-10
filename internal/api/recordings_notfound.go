package api

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// isChunkNotFound reports whether a chunk fetch failed because the chunk does
// not exist. It reads the error's type or HTTP status rather than its text, so
// a storage failure that merely contains "404" (a request id, a port) is not
// taken for a missing chunk.
func isChunkNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrNotFound) {
		return true
	}
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var withStatus interface{ HTTPStatusCode() int }
	return errors.As(err, &withStatus) && withStatus.HTTPStatusCode() == 404
}
