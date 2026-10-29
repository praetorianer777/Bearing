package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultsNeedOnlyThePrimary(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != EnvProduction || !cfg.HTTP.SecureCookies {
		t.Errorf("env %q secure %v, want production with secure cookies", cfg.Env, cfg.HTTP.SecureCookies)
	}
	if cfg.DB.AdminURL != "postgres://p" {
		t.Errorf("admin url %q, want the primary's", cfg.DB.AdminURL)
	}
	if cfg.DB.ReadYourWritesTTL != DefaultReadYourWrites || cfg.Valkey.JobStream != DefaultJobStream {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestDevelopmentDefaultsToPlainCookies(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	t.Setenv("BEARING_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.SecureCookies {
		t.Error("development asks for secure cookies on plain http")
	}
}

func TestReplicasAreAList(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	t.Setenv("BEARING_DB_REPLICA_URLS", " postgres://r1 ,, postgres://r2")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DB.ReplicaURLs) != 2 || cfg.DB.ReplicaURLs[1] != "postgres://r2" {
		t.Errorf("replicas %q", cfg.DB.ReplicaURLs)
	}
}

func TestEveryProblemIsNamedAtOnce(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "")
	t.Setenv("BEARING_ENV", "prod")
	t.Setenv("BEARING_DB_MAX_REPLICA_LAG", "soon")
	t.Setenv("BEARING_DB_PRIMARY_MAX_CONNS", "0")
	_, err := Load()
	if err == nil {
		t.Fatal("a broken configuration was accepted")
	}
	for _, want := range []string{"BEARING_DB_PRIMARY_URL", "BEARING_ENV", "BEARING_DB_MAX_REPLICA_LAG", "BEARING_DB_PRIMARY_MAX_CONNS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %s:\n%s", want, err)
		}
	}
}

func TestDurationsParse(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	t.Setenv("BEARING_DB_MAX_REPLICA_LAG", "500ms")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.MaxReplicaLag != 500*time.Millisecond {
		t.Errorf("lag %s", cfg.DB.MaxReplicaLag)
	}
}

func TestValkeyIsRequiredWhereAsked(t *testing.T) {
	if err := (Config{}).RequireValkey(); err == nil {
		t.Error("no Valkey was accepted")
	}
	if err := (Config{Valkey: Valkey{URL: "redis://v"}}).RequireValkey(); err != nil {
		t.Error(err)
	}
}

func TestTheFirstAdministratorNeedsAPassword(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	t.Setenv("BEARING_BOOTSTRAP_ADMIN_EMAIL", "admin@bearing.test")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BEARING_BOOTSTRAP_ADMIN_PASSWORD") {
		t.Errorf("an administrator without a password was accepted: %v", err)
	}
}

func TestTheAppURLLosesItsTrailingSlash(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	t.Setenv("BEARING_APP_URL", "https://org.example/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.AppURL != "https://org.example" {
		t.Errorf("app url %q", cfg.Auth.AppURL)
	}
}

func TestSingleSignOnNeedsAClient(t *testing.T) {
	t.Setenv("BEARING_DB_PRIMARY_URL", "postgres://p")
	t.Setenv("BEARING_OIDC_ISSUER", "https://id.example/realms/r/")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BEARING_OIDC_CLIENT_ID") {
		t.Errorf("an issuer without a client was accepted: %v", err)
	}
	t.Setenv("BEARING_OIDC_CLIENT_ID", "bearing")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OIDC.Enabled() || cfg.OIDC.Issuer != "https://id.example/realms/r" || cfg.OIDC.GroupsClaim != "groups" {
		t.Errorf("%+v", cfg.OIDC)
	}
}
