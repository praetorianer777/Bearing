-- +goose Up
-- The way an arrow takes around the nodes, as the layout routed it: x and y of each point in turn, from where it
-- leaves its node to where it reaches the next. Empty is no route; the canvas then finds a way of its own.
ALTER TABLE process_arrow ADD COLUMN route double precision[] NOT NULL DEFAULT '{}'
    CONSTRAINT process_arrow_route_points CHECK (cardinality(route) % 2 = 0 AND cardinality(route) <= 200);

-- +goose Down
ALTER TABLE process_arrow DROP COLUMN route;
