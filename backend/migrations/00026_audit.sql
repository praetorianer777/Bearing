-- +goose Up
-- The audit log: every change to the organisation model, who made it, when, and the row before and after. Dated
-- relations say what holds on a day; the log says who changed them and when. A trigger on each model table writes it
-- in the transaction that makes the change, so a change is never without its entry. Nobody changes the log; editors
-- and administrators read it.

CREATE TYPE audit_operation AS ENUM ('insert', 'update', 'delete');

CREATE TABLE audit_event (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    -- The changing transaction's time, so the changes of one request share it.
    at         timestamptz NOT NULL DEFAULT now(),
    -- Whom the change was made for; NULL when Bearing made it itself, as when a sign-in links a person.
    actor_id   uuid REFERENCES account (id) ON DELETE SET NULL,
    table_name text NOT NULL,
    operation  audit_operation NOT NULL,
    -- The units, positions, people, roles and holdings the change concerns, for each one's own view of the log.
    subjects   uuid[] NOT NULL,
    before     jsonb,
    after      jsonb
);

CREATE INDEX audit_event_subjects ON audit_event USING gin (subjects);

-- Timestamps say nothing the entry does not, and a photo's bytes do not belong in a log; updates that change neither
-- side otherwise are not entries at all.
-- +goose StatementBegin
CREATE FUNCTION audit_change() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
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

CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON unit_type FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON unit FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON unit_placement FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON position FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON reporting_line FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON person FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON person_photo FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON holding FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON role FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON position_role FOR EACH ROW EXECUTE FUNCTION audit_change();
CREATE TRIGGER audit_change AFTER INSERT OR UPDATE OR DELETE ON deputy FOR EACH ROW EXECUTE FUNCTION audit_change();

REVOKE INSERT, UPDATE, DELETE ON audit_event FROM bearing_app, bearing_admin;

ALTER TABLE audit_event ENABLE ROW LEVEL SECURITY;

CREATE POLICY audit_event_admin_role ON audit_event FOR SELECT TO bearing_admin USING (true);
CREATE POLICY audit_event_editors ON audit_event FOR SELECT TO bearing_app USING ((SELECT current_actor_may_edit()));

-- +goose Down
DROP TRIGGER audit_change ON deputy;
DROP TRIGGER audit_change ON position_role;
DROP TRIGGER audit_change ON role;
DROP TRIGGER audit_change ON holding;
DROP TRIGGER audit_change ON person_photo;
DROP TRIGGER audit_change ON person;
DROP TRIGGER audit_change ON reporting_line;
DROP TRIGGER audit_change ON position;
DROP TRIGGER audit_change ON unit_placement;
DROP TRIGGER audit_change ON unit;
DROP TRIGGER audit_change ON unit_type;
DROP FUNCTION audit_change();
DROP TABLE audit_event;
DROP TYPE audit_operation;
