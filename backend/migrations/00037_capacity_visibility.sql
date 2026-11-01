-- +goose Up
-- Whose capacity somebody may read person by person. Sums over a unit, a position or a role are for everybody
-- signed in, as the organisation chart is; one person's capacity, and what partners later add to it such as their
-- load and absences, is for the person, those above them in the management chain today, and editors.

-- +goose StatementBegin
CREATE FUNCTION current_actor_sees_capacity_of(whom uuid) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
AS $$
    WITH RECURSIVE held AS (
        SELECT h.position_id FROM holding h
        WHERE h.person_id = whom AND h.position_id IS NOT NULL
          AND h.valid_from <= current_date AND (h.valid_to IS NULL OR h.valid_to > current_date)
    ), above AS (
        SELECT rl.reports_to_id AS position_id FROM reporting_line rl JOIN held ON rl.position_id = held.position_id
        WHERE rl.kind = 'solid' AND rl.valid_from <= current_date AND (rl.valid_to IS NULL OR rl.valid_to > current_date)
        UNION
        SELECT rl.reports_to_id FROM reporting_line rl JOIN above ON rl.position_id = above.position_id
        WHERE rl.kind = 'solid' AND rl.valid_from <= current_date AND (rl.valid_to IS NULL OR rl.valid_to > current_date)
    )
    SELECT current_actor_may_edit()
        OR EXISTS (SELECT 1 FROM person p WHERE p.id = whom AND p.account_id = current_actor_id())
        OR EXISTS (
            SELECT 1 FROM above
            JOIN holding h ON h.position_id = above.position_id
                 AND h.valid_from <= current_date AND (h.valid_to IS NULL OR h.valid_to > current_date)
            JOIN person p ON p.id = h.person_id AND p.account_id = current_actor_id())
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION current_actor_sees_capacity_of(uuid);
