-- +goose Up
-- Who is responsible, accountable, consulted and informed for a node of a process: a role, a position or a unit,
-- as the organisation names it. Responsibilities belong to a version like its nodes: saved with the draft, copied on
-- publishing and fixed with the published version.

CREATE TYPE raci AS ENUM ('responsible', 'accountable', 'consulted', 'informed');

CREATE TABLE process_responsibility (
    version_id  uuid NOT NULL,
    node_key    text NOT NULL,
    kind        raci NOT NULL,
    role_id     uuid REFERENCES role (id),
    position_id uuid REFERENCES position (id),
    unit_id     uuid REFERENCES unit (id),
    FOREIGN KEY (version_id, node_key) REFERENCES process_node (version_id, key) ON DELETE CASCADE,
    CONSTRAINT process_responsibility_one_party CHECK (num_nonnulls(role_id, position_id, unit_id) = 1),
    CONSTRAINT process_responsibility_once UNIQUE NULLS NOT DISTINCT (version_id, node_key, kind, role_id, position_id, unit_id)
);

-- What a role, a position or a unit answers for is asked of them, not of the process.
CREATE INDEX process_responsibility_role ON process_responsibility (role_id) WHERE role_id IS NOT NULL;
CREATE INDEX process_responsibility_position ON process_responsibility (position_id) WHERE position_id IS NOT NULL;
CREATE INDEX process_responsibility_unit ON process_responsibility (unit_id) WHERE unit_id IS NOT NULL;

CREATE TRIGGER process_responsibility_fixed BEFORE INSERT OR UPDATE OR DELETE ON process_responsibility
    FOR EACH ROW EXECUTE FUNCTION process_graph_fixed();

ALTER TABLE process_responsibility ENABLE ROW LEVEL SECURITY;

CREATE POLICY process_responsibility_admin_role ON process_responsibility TO bearing_admin USING (true) WITH CHECK (true);
CREATE POLICY process_responsibility_read ON process_responsibility FOR SELECT TO bearing_app
    USING ((SELECT current_actor_id()) IS NOT NULL);
CREATE POLICY process_responsibility_editors ON process_responsibility TO bearing_app
    USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));

-- +goose Down
DROP TABLE process_responsibility;
DROP TYPE raci;
