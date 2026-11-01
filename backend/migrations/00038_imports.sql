-- +goose Up
-- Imports of the organisation from CSV files: what was sent, what it would change as previewed, and what running it
-- did. An import is previewed, then queued for the worker, which claims it, so a job delivered twice runs it once,
-- and applies it as the editor who sent it, in one transaction. Editors see and run imports.

CREATE TYPE org_import_status AS ENUM ('previewed', 'queued', 'running', 'done', 'failed');

CREATE TABLE org_import (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    created_by  uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    -- The day the changes take effect from.
    effective   date NOT NULL,
    files       jsonb NOT NULL,
    preview     jsonb NOT NULL,
    status      org_import_status NOT NULL DEFAULT 'previewed',
    -- What running it did, or why it did nothing.
    result      jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

CREATE INDEX org_import_created ON org_import (created_at DESC);

ALTER TABLE org_import ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_import_admin_role ON org_import TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY org_import_editors ON org_import TO bearing_app
    USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

-- +goose Down
DROP TABLE org_import;
DROP TYPE org_import_status;
