-- +goose Up
ALTER TABLE users ADD COLUMN iambarn_sub TEXT;
CREATE UNIQUE INDEX idx_users_iambarn_sub ON users (iambarn_sub) WHERE iambarn_sub IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_iambarn_sub;
ALTER TABLE users DROP COLUMN iambarn_sub;
