package repository

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// UpsertSessionSignals sets device/browser signals on a session only if not
// already collected. Scoped by project: the same session ID can exist under
// several projects, and one project's screen size is not another's.
func (s *Store) UpsertSessionSignals(ctx context.Context, projectID, sessionID string, signals domain.SessionSignals) error {
	return updateSessionSignals(ctx, s.db, projectID, sessionID, signals)
}

func updateSessionSignals(ctx context.Context, ex execer, projectID, sessionID string, signals domain.SessionSignals) error {
	const q = `
		UPDATE sessions SET
			screen_width      = ?,
			screen_height     = ?,
			pixel_ratio       = ?,
			touch             = ?,
			dark_mode         = ?,
			reduced_motion    = ?,
			browser_timezone  = ?,
			cpu_cores         = ?,
			signals_collected = 1
		WHERE project_id = ? AND id = ? AND signals_collected = 0`
	_, err := ex.ExecContext(ctx, q,
		nullInt(signals.ScreenWidth), nullInt(signals.ScreenHeight),
		nullFloatPtr(signals.PixelRatio),
		nullBool(signals.Touch), nullBool(signals.DarkMode), nullBool(signals.ReducedMotion),
		nullStr(signals.BrowserTimezone),
		nullInt(signals.CPUCores),
		projectID, sessionID,
	)
	return err
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullFloatPtr(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullBool(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}
