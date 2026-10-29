#!/bin/sh
# Runs once, on first initialisation of the primary: bearing_app is bound by every row level security policy,
# bearing_admin is exempt by policy rather than by being superuser, and migrations create neither.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
    CREATE ROLE bearing_app LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD '${APP_DB_PASSWORD:-bearing_app}';
    CREATE ROLE bearing_admin LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD '${ADMIN_DB_PASSWORD:-bearing_admin}';
    CREATE ROLE replicator WITH REPLICATION LOGIN PASSWORD '${REPLICATION_PASSWORD:-replicator}';

    GRANT CONNECT ON DATABASE "$POSTGRES_DB" TO bearing_app, bearing_admin;

    -- The pool watches pg_stat_wal_receiver to tell a caught-up standby from one whose receiver dropped.
    GRANT pg_read_all_stats TO bearing_app, bearing_admin;
SQL

# The streaming replica connects as replicator, and replication connections match no database entry.
cat >> "$PGDATA/pg_hba.conf" <<-HBA
	host replication replicator all scram-sha-256
HBA
