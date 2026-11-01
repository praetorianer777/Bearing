-- +goose Up
-- A search vector is what Bearing reads out of a row's words, not something anybody changed, so the log leaves it
-- out as it leaves out the timestamps.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_change() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
DECLARE
    old_row  jsonb := CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) - 'created_at' - 'updated_at' - 'data' - 'search_vector' END;
    new_row  jsonb := CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) - 'created_at' - 'updated_at' - 'data' - 'search_vector' END;
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
    WHERE e.key IN ('id', 'unit_id', 'parent_id', 'position_id', 'reports_to_id', 'person_id', 'role_id', 'deputy_id', 'type_id',
                    'process_id', 'owner_id')
      AND e.value IS NOT NULL;
    INSERT INTO audit_event (actor_id, table_name, operation, subjects, before, after)
    VALUES (current_actor_id(), TG_TABLE_NAME, lower(TG_OP)::audit_operation, subjects, old_row, new_row);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
