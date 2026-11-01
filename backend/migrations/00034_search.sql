-- +goose Up
-- Search over people, positions, units, roles, processes and their steps. Each keeps a search vector of its name,
-- weighted A, and its other words, weighted B, read with bearing_search: case and accents ignored, nothing stemmed,
-- so a name is found as written in any language. Descriptions are read once more with the English and the German
-- stemmer, weighted D, so "approves" finds "approve" without a name ranking below a sentence. Names are indexed by
-- trigram as well, so a name typed with a slip is still found.

CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TEXT SEARCH CONFIGURATION bearing_search (COPY = pg_catalog.simple);
ALTER TEXT SEARCH CONFIGURATION bearing_search ALTER MAPPING FOR hword, hword_part, word WITH unaccent, simple;
CREATE TEXT SEARCH CONFIGURATION bearing_english (COPY = pg_catalog.english);
ALTER TEXT SEARCH CONFIGURATION bearing_english ALTER MAPPING FOR hword, hword_part, word WITH unaccent, english_stem;
CREATE TEXT SEARCH CONFIGURATION bearing_german (COPY = pg_catalog.german);
ALTER TEXT SEARCH CONFIGURATION bearing_german ALTER MAPPING FOR hword, hword_part, word WITH unaccent, german_stem;

-- unaccent is only stable, since its dictionary could change; naming the dictionary makes the answer fixed, which an
-- index needs.
CREATE FUNCTION search_fold(t text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    RETURN lower(public.unaccent('public.unaccent'::regdictionary, t));

CREATE FUNCTION search_words(name text, rest text, description text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    RETURN setweight(to_tsvector('bearing_search'::regconfig, COALESCE(name, '')), 'A') ||
           setweight(to_tsvector('bearing_search'::regconfig, COALESCE(rest, '') || ' ' || COALESCE(description, '')), 'B') ||
           setweight(to_tsvector('bearing_english'::regconfig, COALESCE(description, '')), 'D') ||
           setweight(to_tsvector('bearing_german'::regconfig, COALESCE(description, '')), 'D');

-- A person's address counts by its part before the @, which is how people are known; the domain everybody shares
-- would find them all.
ALTER TABLE person ADD COLUMN search_vector tsvector GENERATED ALWAYS AS
    (search_words(name, COALESCE(split_part(email::text, '@', 1), '') || ' ' || COALESCE(location, ''), NULL)) STORED;
ALTER TABLE position ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (search_words(title, NULL, description)) STORED;
ALTER TABLE unit ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (search_words(name, short_name, description)) STORED;
ALTER TABLE role ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (search_words(name, key, description)) STORED;
ALTER TABLE process ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (search_words(name, key || ' ' || area, description)) STORED;
ALTER TABLE process_node ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (search_words(name, NULL, description)) STORED;

CREATE INDEX person_search ON person USING gin (search_vector);
CREATE INDEX position_search ON position USING gin (search_vector);
CREATE INDEX unit_search ON unit USING gin (search_vector);
CREATE INDEX role_search ON role USING gin (search_vector);
CREATE INDEX process_search ON process USING gin (search_vector);
CREATE INDEX process_node_search ON process_node USING gin (search_vector);

CREATE INDEX person_name_trgm ON person USING gin (search_fold(name) gin_trgm_ops);
CREATE INDEX position_title_trgm ON position USING gin (search_fold(title) gin_trgm_ops);
CREATE INDEX unit_name_trgm ON unit USING gin (search_fold(name) gin_trgm_ops);
CREATE INDEX role_name_trgm ON role USING gin (search_fold(name) gin_trgm_ops);
CREATE INDEX process_name_trgm ON process USING gin (search_fold(name) gin_trgm_ops);
CREATE INDEX process_node_name_trgm ON process_node USING gin (search_fold(name) gin_trgm_ops);

-- +goose Down
ALTER TABLE process_node DROP COLUMN search_vector;
ALTER TABLE process DROP COLUMN search_vector;
ALTER TABLE role DROP COLUMN search_vector;
ALTER TABLE unit DROP COLUMN search_vector;
ALTER TABLE position DROP COLUMN search_vector;
ALTER TABLE person DROP COLUMN search_vector;
DROP INDEX process_node_name_trgm, process_name_trgm, role_name_trgm, unit_name_trgm, position_title_trgm, person_name_trgm;
DROP FUNCTION search_words(text, text, text);
DROP FUNCTION search_fold(text);
DROP TEXT SEARCH CONFIGURATION bearing_german;
DROP TEXT SEARCH CONFIGURATION bearing_english;
DROP TEXT SEARCH CONFIGURATION bearing_search;
