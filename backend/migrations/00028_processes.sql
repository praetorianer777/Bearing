-- +goose Up
-- Processes: flow graphs whose steps say who does what. Each process has one draft, edited freely, and the versions
-- published from it, numbered from one and never changed again. A version is a graph of nodes joined by arrows; the
-- draft is the version without a number. Publishing numbers the draft and copies it into a new one, so whatever later
-- hangs off a node, such as its responsibilities, is copied along and the published copy keeps it as it was.

CREATE TYPE process_node_kind AS ENUM ('start', 'end', 'step', 'decision', 'subprocess');

CREATE TABLE process (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    -- What other tools name the process by; it does not change with the name.
    key            text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    name           text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    -- Processes are grouped by area, such as "Sales" or "Finance"; empty is none.
    area           text NOT NULL DEFAULT '' CHECK (length(area) <= 100),
    description    text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    -- The position answering for the process as a whole.
    owner_id       uuid REFERENCES position (id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER process_touch BEFORE UPDATE ON process FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX process_owner ON process (owner_id);

CREATE TABLE process_version (
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    process_id   uuid NOT NULL REFERENCES process (id) ON DELETE CASCADE,
    -- NULL is the draft.
    number       integer CHECK (number >= 1),
    published_at timestamptz,
    published_by uuid REFERENCES account (id) ON DELETE SET NULL,
    -- What changed, in the publisher's words.
    note         text NOT NULL DEFAULT '' CHECK (length(note) <= 1000),
    -- Counts the draft's saves, so a save made from an older copy of the draft is told so and not taken.
    revision     integer NOT NULL DEFAULT 0,
    saved_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (process_id, number),
    CHECK ((number IS NULL) = (published_at IS NULL))
);

CREATE UNIQUE INDEX process_version_one_draft ON process_version (process_id) WHERE number IS NULL;

CREATE TABLE process_node (
    version_id     uuid NOT NULL REFERENCES process_version (id) ON DELETE CASCADE,
    -- The editor names nodes; the name holds within the version and is copied with it.
    key            text NOT NULL CHECK (key ~ '^[A-Za-z0-9_-]{1,64}$'),
    kind           process_node_kind NOT NULL,
    name           text NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    description    text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    -- Where it was left on the canvas.
    x              double precision NOT NULL DEFAULT 0,
    y              double precision NOT NULL DEFAULT 0,
    -- Checked at commit, so processes naming each other can go together; Bearing refuses removing one in use itself.
    subprocess_id  uuid REFERENCES process (id) DEFERRABLE INITIALLY DEFERRED,
    PRIMARY KEY (version_id, key),
    CHECK (subprocess_id IS NULL OR kind = 'subprocess')
);

CREATE INDEX process_node_subprocess ON process_node (subprocess_id);

CREATE TABLE process_arrow (
    version_id uuid NOT NULL,
    key        text NOT NULL CHECK (key ~ '^[A-Za-z0-9_-]{1,64}$'),
    from_key   text NOT NULL,
    to_key     text NOT NULL,
    -- The condition the way is taken on, out of a decision; empty for none.
    label      text NOT NULL DEFAULT '' CHECK (length(label) <= 200),
    PRIMARY KEY (version_id, key),
    UNIQUE (version_id, from_key, to_key),
    FOREIGN KEY (version_id, from_key) REFERENCES process_node (version_id, key) ON DELETE CASCADE,
    FOREIGN KEY (version_id, to_key) REFERENCES process_node (version_id, key) ON DELETE CASCADE,
    CHECK (from_key <> to_key)
);

CREATE INDEX process_arrow_to ON process_arrow (version_id, to_key);

-- A published version stays as it was published. Its nodes and arrows go only with it, and it goes only with its
-- process, which Bearing removes only when nothing was ever published; a superuser cleaning up may do anything.
-- +goose StatementBegin
CREATE FUNCTION process_version_fixed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.number IS NOT NULL AND EXISTS (SELECT 1 FROM process WHERE id = OLD.process_id) THEN
            RAISE EXCEPTION 'a published version of a process does not change' USING ERRCODE = 'integrity_constraint_violation',
                CONSTRAINT = 'process_version_fixed';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.number IS NOT NULL THEN
        RAISE EXCEPTION 'a published version of a process does not change' USING ERRCODE = 'integrity_constraint_violation',
            CONSTRAINT = 'process_version_fixed';
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION process_graph_fixed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version uuid := CASE WHEN TG_OP = 'INSERT' THEN NEW.version_id ELSE OLD.version_id END;
BEGIN
    IF EXISTS (SELECT 1 FROM process_version WHERE id = version AND number IS NOT NULL) THEN
        RAISE EXCEPTION 'a published version of a process does not change' USING ERRCODE = 'integrity_constraint_violation',
            CONSTRAINT = 'process_version_fixed';
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER process_version_fixed BEFORE UPDATE OR DELETE ON process_version FOR EACH ROW EXECUTE FUNCTION process_version_fixed();
CREATE TRIGGER process_node_fixed BEFORE INSERT OR UPDATE OR DELETE ON process_node FOR EACH ROW EXECUTE FUNCTION process_graph_fixed();
CREATE TRIGGER process_arrow_fixed BEFORE INSERT OR UPDATE OR DELETE ON process_arrow FOR EACH ROW EXECUTE FUNCTION process_graph_fixed();

-- A process's entries and its versions' name the process, and the position owning it.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_change() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
DECLARE
    old_row  jsonb := CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) - 'created_at' - 'updated_at' - 'data' END;
    new_row  jsonb := CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) - 'created_at' - 'updated_at' - 'data' END;
    subjects uuid[];
BEGIN
    IF old_row = new_row THEN
        RETURN NULL;
    END IF;
    SELECT coalesce(array_agg(DISTINCT e.value::uuid), '{}') INTO subjects
    FROM (SELECT * FROM jsonb_each_text(coalesce(old_row, '{}')) UNION ALL SELECT * FROM jsonb_each_text(coalesce(new_row, '{}'))) e
    WHERE e.key IN ('id', 'unit_id', 'parent_id', 'position_id', 'reports_to_id', 'person_id', 'role_id', 'deputy_id', 'type_id',
                    'process_id', 'owner_id')
      AND e.value IS NOT NULL;
    INSERT INTO audit_event (actor_id, table_name, operation, subjects, before, after)
    VALUES (current_actor_id(), TG_TABLE_NAME, lower(TG_OP)::audit_operation, subjects, old_row, new_row);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- The log keeps the processes and each publishing; a draft changes with every save, and the versions published from
-- it keep what it was.
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON process FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_publish AFTER UPDATE ON process_version FOR EACH ROW
    WHEN (OLD.number IS NULL AND NEW.number IS NOT NULL) EXECUTE FUNCTION audit_change();

ALTER TABLE process ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_node ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_arrow ENABLE ROW LEVEL SECURITY;

CREATE POLICY process_admin_role ON process TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY process_read ON process FOR SELECT TO bearing_app USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY process_editors ON process TO bearing_app USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

CREATE POLICY process_version_admin_role ON process_version TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY process_version_read ON process_version FOR SELECT TO bearing_app USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY process_version_editors ON process_version TO bearing_app
    USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

CREATE POLICY process_node_admin_role ON process_node TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY process_node_read ON process_node FOR SELECT TO bearing_app USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY process_node_editors ON process_node TO bearing_app
    USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

CREATE POLICY process_arrow_admin_role ON process_arrow TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY process_arrow_read ON process_arrow FOR SELECT TO bearing_app USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY process_arrow_editors ON process_arrow TO bearing_app
    USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

-- When the demo's processes were loaded, so loading them again changes nothing.
ALTER TABLE installation ADD COLUMN demo_processes_loaded_at timestamptz;

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_change() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
DECLARE
    old_row  jsonb := CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) - 'created_at' - 'updated_at' - 'data' END;
    new_row  jsonb := CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) - 'created_at' - 'updated_at' - 'data' END;
    subjects uuid[];
BEGIN
    IF old_row = new_row THEN
        RETURN NULL;
    END IF;
    SELECT coalesce(array_agg(DISTINCT e.value::uuid), '{}') INTO subjects
    FROM (SELECT * FROM jsonb_each_text(coalesce(old_row, '{}')) UNION ALL SELECT * FROM jsonb_each_text(coalesce(new_row, '{}'))) e
    WHERE e.key IN ('id', 'unit_id', 'parent_id', 'position_id', 'reports_to_id', 'person_id', 'role_id', 'deputy_id', 'type_id')
      AND e.value IS NOT NULL;
    INSERT INTO audit_event (actor_id, table_name, operation, subjects, before, after)
    VALUES (current_actor_id(), TG_TABLE_NAME, lower(TG_OP)::audit_operation, subjects, old_row, new_row);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
ALTER TABLE installation DROP COLUMN demo_processes_loaded_at;
DROP TABLE process_arrow;
DROP TABLE process_node;
DROP TABLE process_version;
DROP TABLE process;
DROP FUNCTION process_graph_fixed();
DROP FUNCTION process_version_fixed();
DROP TYPE process_node_kind;
