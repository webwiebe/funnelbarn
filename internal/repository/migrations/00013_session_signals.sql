-- +goose Up
ALTER TABLE sessions ADD COLUMN screen_width INTEGER;
ALTER TABLE sessions ADD COLUMN screen_height INTEGER;
ALTER TABLE sessions ADD COLUMN pixel_ratio REAL;
ALTER TABLE sessions ADD COLUMN touch INTEGER;
ALTER TABLE sessions ADD COLUMN dark_mode INTEGER;
ALTER TABLE sessions ADD COLUMN reduced_motion INTEGER;
ALTER TABLE sessions ADD COLUMN browser_timezone TEXT;
ALTER TABLE sessions ADD COLUMN cpu_cores INTEGER;
ALTER TABLE sessions ADD COLUMN signals_collected INTEGER NOT NULL DEFAULT 0;

-- +goose Down
