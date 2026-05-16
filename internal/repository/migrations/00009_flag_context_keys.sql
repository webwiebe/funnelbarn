-- +goose Up
ALTER TABLE flag_evaluations ADD COLUMN context_keys TEXT NOT NULL DEFAULT '[]';
CREATE INDEX idx_flag_evals_project_time ON flag_evaluations (project_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_flag_evals_project_time;
ALTER TABLE flag_evaluations DROP COLUMN context_keys;
