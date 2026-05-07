-- +goose Up
ALTER TABLE feature_flags ADD COLUMN targeting_rules TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE feature_flags DROP COLUMN targeting_rules;
