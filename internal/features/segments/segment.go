package segments

import (
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// Segment is a named, reusable filter applied to funnel analysis. The rule
// type stays in domain because the funnel queries evaluate it.
type Segment struct {
	ID        string               `json:"id"`
	ProjectID string               `json:"project_id"`
	Name      string               `json:"name"`
	Rules     []domain.SegmentRule `json:"rules"`
	CreatedAt time.Time            `json:"created_at"`
}
