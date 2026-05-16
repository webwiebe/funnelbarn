-- +goose Up
ALTER TABLE flag_evaluations ADD COLUMN context_keys TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE flag_evaluations DROP COLUMN context_keys;
