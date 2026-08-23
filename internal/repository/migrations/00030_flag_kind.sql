-- +goose Up
-- flag_kind separates the two things flags are used for.
--
-- 'experiment' (the default, and everything that existed before this column) is
-- the A/B-test shape: a user is bucketed by targeting key, every read is an
-- analytics data point, and the variant/conversion report is the whole purpose.
--
-- 'config' is a singleton value a server reads on a loop — a rate limit, a batch
-- size, a daily cap. There is no user, no bucket and no conversion, so recording
-- an evaluation row per read only produces junk (one polling service at 60s from
-- 3 pods writes ~4.3k rows/day for a value that changes twice) and drowns the
-- flag's own analytics in machine reads.
ALTER TABLE feature_flags ADD COLUMN flag_kind TEXT NOT NULL DEFAULT 'experiment';

-- +goose Down
ALTER TABLE feature_flags DROP COLUMN flag_kind;
