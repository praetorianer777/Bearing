//go:build integration

package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/migrations"
)

// probeTable is the suite's own table, made by the superuser, so the routing test needs no part of the schema.
const probeTable = "routing_probe"

// A write followed by a read on a replica that has not replayed it is answered by the primary, and a read with no
// write to respect is answered by the replica, older as it is.
func TestAReadAfterAWriteNeverSeesALaggingReplica(t *testing.T) {
	ctx := context.Background()
	primary := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	replica := superuser(t, "BEARING_TEST_REPLICA_SUPERUSER_URL")
	if _, err := primary.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+probeTable+` (id text PRIMARY KEY);
		GRANT SELECT, INSERT ON `+probeTable+` TO bearing_app`); err != nil {
		t.Fatal(err)
	}
	waitForReplica(t, replica, primary)

	cfg := loadConfig(t)
	// No lag bound: the paused replica must stay in the rotation for the test to mean anything.
	cfg.DB.MaxReplicaLag = 0
	cluster, err := db.Open(ctx, cfg.DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)

	if _, err := replica.Exec(ctx, `SELECT pg_wal_replay_pause()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = replica.Exec(context.Background(), `SELECT pg_wal_replay_resume()`) })

	id := time.Now().Format(time.RFC3339Nano)
	lsn, err := cluster.Write(ctx, func(ctx context.Context, q db.DBTX) error {
		_, err := q.Exec(ctx, `INSERT INTO `+probeTable+` (id) VALUES ($1)`, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if !sees(t, cluster, db.PinLSN(ctx, lsn), id) {
		t.Error("the writer's own read missed the write")
	}
	if sees(t, cluster, ctx, id) {
		t.Error("a read with nothing to respect did not go to the paused replica")
	}
	if stats := cluster.Stats(); stats.StaleFallbacks == 0 || stats.ReadsToReplica == 0 {
		t.Errorf("routing counters %+v, want a stale fallback and a replica read", stats)
	}
}

func sees(t *testing.T, cluster *db.Cluster, ctx context.Context, id string) bool {
	t.Helper()
	var found bool
	err := cluster.Read(ctx, func(ctx context.Context, q db.DBTX) error {
		return q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+probeTable+` WHERE id = $1)`, id).Scan(&found)
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// waitForReplica waits until the replica has replayed everything the primary has written so far.
func waitForReplica(t *testing.T, replica, primary interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) {
	t.Helper()
	ctx := context.Background()
	var target string
	if err := primary.QueryRow(ctx, `SELECT pg_current_wal_insert_lsn()::text`).Scan(&target); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var caughtUp bool
		if err := replica.QueryRow(ctx, `SELECT pg_last_wal_replay_lsn() >= $1::pg_lsn`, target).Scan(&caughtUp); err != nil {
			t.Fatal(err)
		}
		if caughtUp {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the replica did not catch up within ten seconds")
}

func TestTheAppRoleIsHeldByRowLevelSecurity(t *testing.T) {
	cluster, err := db.Open(context.Background(), loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	if err := cluster.RefuseSuperuser(context.Background()); err != nil {
		t.Errorf("bearing_app was refused: %v", err)
	}
	err = migrations.CheckOwner(context.Background(), cluster.Primary())
	if err == nil {
		t.Error("bearing_app was accepted as the migrations' owner")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
