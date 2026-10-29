//go:build integration

package test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/praetorianer777/bearing/backend/internal/db"
)

// withRole makes a local account with a role and signs it in.
func withRole(t *testing.T, admin *browser, role string) (*browser, uuid.UUID) {
	t.Helper()
	email := uniqueEmail(role)
	var made accountBody
	if status := admin.do(http.MethodPost, "/accounts", map[string]string{"name": role, "email": email, "role": role, "password": role + " long password"}, &made); status != http.StatusCreated {
		t.Fatalf("making a %s answered %d", role, status)
	}
	return signedIn(t, email, role+" long password"), uuid.MustParse(made.Account.ID)
}

func roleOf(t *testing.T, b *browser) (role, own string) {
	t.Helper()
	var me struct {
		Account struct{ Role, OwnRole string }
	}
	if status := b.do(http.MethodGet, "/auth/me", nil, &me); status != http.StatusOK {
		t.Fatalf("me answered %d", status)
	}
	return me.Account.Role, me.Account.OwnRole
}

// holds waits for somebody's role to become want: it is somebody else's write that changed it, which their reads see
// once the replica has it.
func holds(t *testing.T, b *browser, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		role, _ := roleOf(t, b)
		if role == want {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("holds %q, want %q", role, want)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestAnAdministratorGrantsRolesButNotTheirOwn(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	reader, id := withRole(t, admin, "reader")

	if status := reader.do(http.MethodPatch, "/auth/me", map[string]string{"name": "Renamed"}, nil); status != http.StatusForbidden {
		t.Errorf("a reader changed their profile: %d", status)
	}
	if status := admin.do(http.MethodPut, "/accounts/"+id.String()+"/role", map[string]string{"role": "member"}, nil); status != http.StatusNoContent {
		t.Fatalf("granting a role answered %d", status)
	}
	holds(t, reader, "member")
	var renamed accountBody
	if status := reader.do(http.MethodPatch, "/auth/me", map[string]string{"name": "  Renamed  "}, &renamed); status != http.StatusOK || renamed.Account.Name != "Renamed" {
		t.Errorf("a member changing their name answered %d, %q", status, renamed.Account.Name)
	}
	if status := reader.do(http.MethodPatch, "/auth/me", map[string]string{"name": " "}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("an empty name answered %d", status)
	}
	if status := admin.do(http.MethodPut, "/accounts/"+id.String()+"/role", map[string]string{"role": "owner"}, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("an unknown role answered %d", status)
	}

	var me accountBody
	admin.do(http.MethodGet, "/auth/me", nil, &me)
	if status := admin.do(http.MethodPut, "/accounts/"+me.Account.ID+"/role", map[string]string{"role": "reader"}, nil); status != http.StatusConflict {
		t.Errorf("an administrator changed their own role: %d", status)
	}
}

func TestAGroupGrantHoldsWhileTheGroupIsHeld(t *testing.T) {
	forget(t, "dave@suite.test")
	admin := signedIn(t, adminEmail, adminPassword)
	var before groupGrants
	admin.do(http.MethodGet, "/group-grants", nil, &before)
	t.Cleanup(func() { admin.do(http.MethodPut, "/group-grants", before, nil) })
	admin.do(http.MethodPut, "/group-grants", groupGrants{Grants: []grant{{"readers", "editor"}}}, nil)

	dave, _ := ssoSignIn(t, "dave", "dave password")
	if role, own := roleOf(t, dave); role != "editor" || own != "reader" {
		t.Errorf("dave holds %q with %q of their own, want editor through readers and reader of their own", role, own)
	}
	admin.do(http.MethodPut, "/group-grants", groupGrants{Grants: []grant{{"marketing", "member"}}}, nil)
	holds(t, dave, "member")
	admin.do(http.MethodPut, "/group-grants", groupGrants{}, nil)
	holds(t, dave, "reader")
}

// The database holds the roles itself, whatever a handler asks of it.
func TestTheDatabaseHoldsEachRoleToItsRights(t *testing.T) {
	ctx := context.Background()
	cluster, err := db.Open(ctx, loadConfig(t).DB, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	admin := signedIn(t, adminEmail, adminPassword)
	_, reader := withRole(t, admin, "reader")
	_, member := withRole(t, admin, "member")
	_, editor := withRole(t, admin, "editor")
	_, granted := withRole(t, admin, "reader")

	super := superuser(t, "BEARING_TEST_SUPERUSER_URL")
	group := "editors-" + uuid.NewString()
	if _, err := super.Exec(ctx, `INSERT INTO account_identity (account_id, issuer, subject, groups) VALUES ($1, 'test', $2, $3)`,
		granted, uuid.NewString(), []string{group}); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `INSERT INTO group_grant (group_name, role) VALUES ($1, 'editor')`, group); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = super.Exec(ctx, `DELETE FROM group_grant WHERE group_name = $1`, group) })

	for _, c := range []struct {
		name    string
		id      uuid.UUID
		mayEdit bool
	}{{"reader", reader, false}, {"member", member, false}, {"editor", editor, true}, {"reader in an editors' group", granted, true}} {
		var may bool
		err := cluster.Read(db.PinPrimary(db.WithUser(ctx, c.id)), func(ctx context.Context, q db.DBTX) error {
			return q.QueryRow(ctx, `SELECT current_actor_may_edit()`).Scan(&may)
		})
		if err != nil || may != c.mayEdit {
			t.Errorf("%s may edit: %v, %v", c.name, may, err)
		}
	}

	rename := func(id uuid.UUID, set string) (int64, error) {
		var n int64
		_, err := cluster.Write(db.WithUser(ctx, id), func(ctx context.Context, q db.DBTX) error {
			tag, err := q.Exec(ctx, `UPDATE account SET `+set+` WHERE id = $1`, id)
			n = tag.RowsAffected()
			return err
		})
		return n, err
	}
	if n, err := rename(reader, `name = 'Reader renamed'`); err != nil || n != 0 {
		t.Errorf("a reader renamed themselves: %d, %v", n, err)
	}
	if n, err := rename(member, `name = 'Member renamed'`); err != nil || n != 1 {
		t.Errorf("a member could not rename themselves: %d, %v", n, err)
	}
	var pgErr *pgconn.PgError
	if _, err := rename(member, `role = 'administrator'`); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Errorf("a member raised their own role: %v", err)
	}
	if _, err := rename(member, `disabled_at = now()`); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Errorf("a member switched themselves off: %v", err)
	}
	_, err = cluster.Write(db.WithUser(ctx, editor), func(ctx context.Context, q db.DBTX) error {
		tag, err := q.Exec(ctx, `UPDATE account SET name = 'Taken over' WHERE id = $1`, member)
		if err == nil && tag.RowsAffected() != 0 {
			return errors.New("an editor renamed somebody else")
		}
		return err
	})
	if err != nil {
		t.Error(err)
	}
}

// A role taken away is gone at the next request, not at the next sign-in.
func TestARoleTakenAwayIsGoneAtOnce(t *testing.T) {
	admin := signedIn(t, adminEmail, adminPassword)
	member, id := withRole(t, admin, "member")
	admin.do(http.MethodPut, "/accounts/"+id.String()+"/role", map[string]string{"role": "administrator"}, nil)
	holds(t, member, "administrator")
	if status := member.do(http.MethodGet, "/accounts", nil, nil); status != http.StatusOK {
		t.Fatalf("a new administrator listing accounts answered %d", status)
	}
	admin.do(http.MethodPut, "/accounts/"+id.String()+"/role", map[string]string{"role": "reader"}, nil)
	holds(t, member, "reader")
	if status := member.do(http.MethodGet, "/accounts", nil, nil); status != http.StatusForbidden {
		t.Errorf("the administrator role outlived being taken away: %d", status)
	}
}
