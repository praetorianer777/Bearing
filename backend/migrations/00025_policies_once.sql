-- +goose Up
-- The model's policies ask who acts once per query, not once per row. A function call in a policy runs for every row
-- it guards; wrapped in a sub-select it becomes an init plan the planner evaluates once, which a walk over thousands
-- of positions, holdings and people needs to answer in time.

ALTER POLICY unit_read ON unit USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY unit_editors ON unit USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY unit_placement_read ON unit_placement USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY unit_placement_editors ON unit_placement USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY position_read ON position USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY position_editors ON position USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY reporting_line_read ON reporting_line USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY reporting_line_editors ON reporting_line USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY person_read ON person USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY person_editors ON person USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY person_photo_read ON person_photo USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY person_photo_editors ON person_photo USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY holding_read ON holding USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY holding_editors ON holding USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY role_read ON role USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY role_editors ON role USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY position_role_read ON position_role USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY position_role_editors ON position_role USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY deputy_read ON deputy USING ((SELECT current_actor_id()) IS NOT NULL);
ALTER POLICY deputy_editors ON deputy USING ((SELECT current_actor_may_edit())) WITH CHECK ((SELECT current_actor_may_edit()));
ALTER POLICY unit_type_read ON unit_type USING ((SELECT current_actor_id()) IS NOT NULL);

-- +goose Down
ALTER POLICY unit_read ON unit USING (current_actor_id() IS NOT NULL);
ALTER POLICY unit_editors ON unit USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY unit_placement_read ON unit_placement USING (current_actor_id() IS NOT NULL);
ALTER POLICY unit_placement_editors ON unit_placement USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY position_read ON position USING (current_actor_id() IS NOT NULL);
ALTER POLICY position_editors ON position USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY reporting_line_read ON reporting_line USING (current_actor_id() IS NOT NULL);
ALTER POLICY reporting_line_editors ON reporting_line USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY person_read ON person USING (current_actor_id() IS NOT NULL);
ALTER POLICY person_editors ON person USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY person_photo_read ON person_photo USING (current_actor_id() IS NOT NULL);
ALTER POLICY person_photo_editors ON person_photo USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY holding_read ON holding USING (current_actor_id() IS NOT NULL);
ALTER POLICY holding_editors ON holding USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY role_read ON role USING (current_actor_id() IS NOT NULL);
ALTER POLICY role_editors ON role USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY position_role_read ON position_role USING (current_actor_id() IS NOT NULL);
ALTER POLICY position_role_editors ON position_role USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY deputy_read ON deputy USING (current_actor_id() IS NOT NULL);
ALTER POLICY deputy_editors ON deputy USING (current_actor_may_edit()) WITH CHECK (current_actor_may_edit());
ALTER POLICY unit_type_read ON unit_type USING (current_actor_id() IS NOT NULL);
