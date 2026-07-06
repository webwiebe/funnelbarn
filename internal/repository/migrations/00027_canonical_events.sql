-- +goose Up
-- Canonical event catalog: instance-level vocabulary shared across projects so
-- differently-named raw events (e.g. "registration" vs "sign_up") can be reasoned
-- about together in cross-project analytics and funnels.
CREATE TABLE IF NOT EXISTS canonical_events (
    key        TEXT PRIMARY KEY,          -- e.g. 'sign_up', 'page_view'
    label      TEXT NOT NULL,             -- human label, e.g. 'Sign Up'
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Per-project mapping of a raw event name onto a canonical key. A project may map
-- several raw names to one canonical key. Deleting a project or a canonical event
-- removes its mappings.
CREATE TABLE IF NOT EXISTS event_name_mappings (
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    raw_name      TEXT NOT NULL,
    canonical_key TEXT NOT NULL REFERENCES canonical_events(key) ON DELETE CASCADE,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, raw_name)
);
-- Reverse lookup (canonical -> raw names for a project), used by the funnel engine.
CREATE INDEX IF NOT EXISTS idx_event_name_mappings_canonical
    ON event_name_mappings (project_id, canonical_key);

-- Instance-level cross-project funnels defined over canonical event keys.
CREATE TABLE IF NOT EXISTS canonical_funnels (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    scope       TEXT NOT NULL DEFAULT 'session',  -- 'session' | 'page_view'
    project_ids TEXT NOT NULL DEFAULT '[]',        -- JSON array; empty [] = all projects
    segment     TEXT,                              -- default preset segment; NULL = none
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS canonical_funnel_steps (
    id            TEXT PRIMARY KEY,
    funnel_id     TEXT NOT NULL REFERENCES canonical_funnels(id) ON DELETE CASCADE,
    step_order    INTEGER NOT NULL,
    -- RESTRICT: don't let a canonical-event delete silently break a saved funnel.
    canonical_key TEXT NOT NULL REFERENCES canonical_events(key) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_canonical_funnel_steps_funnel
    ON canonical_funnel_steps (funnel_id, step_order);

-- Seed a small default catalog so the instance is usable immediately.
INSERT INTO canonical_events (key, label, sort_order) VALUES
    ('page_view',      'Page View',      10),
    ('sign_up',        'Sign Up',        20),
    ('login',          'Login',          30),
    ('add_to_cart',    'Add to Cart',    40),
    ('checkout_start', 'Checkout Start', 50),
    ('purchase',       'Purchase',       60)
ON CONFLICT(key) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS canonical_funnel_steps;
DROP TABLE IF EXISTS canonical_funnels;
DROP TABLE IF EXISTS event_name_mappings;
DROP TABLE IF EXISTS canonical_events;
