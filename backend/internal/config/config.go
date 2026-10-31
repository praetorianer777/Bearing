// Package config loads and validates process configuration from the environment. Every setting is a BEARING_*
// variable; there is no file.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// Environments a deployment may declare itself as.
const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// Defaults, named so the tests and the documentation can quote them.
const (
	DefaultHTTPAddr        = ":8080"
	DefaultRequestTimeout  = 30 * time.Second
	DefaultShutdownTimeout = 20 * time.Second
	DefaultMaxConns        = 20
	DefaultMinConns        = 2
	DefaultConnMaxLifetime = time.Hour
	DefaultHealthInterval  = 5 * time.Second
	DefaultMaxReplicaLag   = 2 * time.Second
	DefaultLagSamples      = 3
	DefaultReadYourWrites  = 30 * time.Second
	DefaultJobStream       = "bearing:jobs"
	DefaultAppURL          = "http://localhost:5173"
	DefaultSessionCookie   = "bearing_session"
	DefaultSessionTTL      = 720 * time.Hour
	DefaultInvitationTTL   = 7 * 24 * time.Hour
	DefaultResetTTL        = time.Hour
	DefaultMailFrom        = "Bearing <bearing@localhost>"
	DefaultGroupsClaim     = "groups"
	// DefaultName is what a first start calls the installation when nobody names it.
	DefaultName = "Bearing"
)

// Config is everything a process reads at start.
type Config struct {
	Env      string
	LogLevel string

	HTTP      HTTP
	DB        DB
	Valkey    Valkey
	Auth      Auth
	Mail      Mail
	OIDC      OIDC
	Bootstrap Bootstrap
}

// OIDC is the identity provider people may sign in through; without an issuer the sign-in page offers passwords only.
type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	GroupsClaim  string
	// Backchannel is "public=reachable" pairs, for a provider this process reaches at another address than browsers.
	Backchannel string
}

// Enabled says whether a provider is configured.
func (o OIDC) Enabled() bool { return o.Issuer != "" }

// OIDCCallbackPath is where the provider sends the browser back, under the web client's origin, which proxies the API.
const OIDCCallbackPath = "/api/v1/auth/oidc/callback"

// Auth is how people stay signed in, and how long the links that bring them in last.
type Auth struct {
	// AppURL is the web client's address as people open it, for the links a mail carries.
	AppURL        string
	SessionCookie string
	SessionTTL    time.Duration
	InvitationTTL time.Duration
	ResetTTL      time.Duration
}

// Mail is the SMTP relay mail goes out through; without an address no mail is sent, and an administrator hands an
// invitation's link on by hand.
type Mail struct {
	SMTPAddr string
	From     string
}

// Bootstrap is what cmd/seed sets an installation up with when it is not set up yet: its name and its first
// administrator. Without an email nothing is set up, and the first person to open the web client does it.
type Bootstrap struct {
	Name          string
	AdminName     string
	AdminEmail    string
	AdminPassword string
	// Members are let in ahead of their first single sign-on: "email=role" pairs, separated by commas.
	Members string
	// Demo has the seed load the demo organisation into an installation without one, as the first administrator.
	Demo bool
}

// HTTP is how the api listens.
type HTTP struct {
	Addr            string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	// SecureCookies marks cookies Secure, which a deployment behind TLS wants and a development stack on plain
	// http cannot have.
	SecureCookies bool
}

// DB describes the Postgres cluster: one writable primary and optional read replicas, so a developer can run
// against a single instance.
type DB struct {
	PrimaryURL  string
	ReplicaURLs []string
	// AdminURL connects as bearing_admin, exempt from row level security by policy, for signing in and jobs that
	// act for nobody. Defaults to PrimaryURL.
	AdminURL string

	// PrimaryMaxConns bounds the write pool, and the admin pool beside it; ReplicaMaxConns bounds each read pool.
	PrimaryMaxConns int32
	ReplicaMaxConns int32
	MinConns        int32
	ConnMaxLifetime time.Duration
	HealthInterval  time.Duration
	MaxReplicaLag   time.Duration
	// ReplicaLagSamples is how many connections of a replica's pool each health pass asks, so behind a load
	// balanced service it sees several.
	ReplicaLagSamples int
	// ReadYourWritesTTL is how long a reader's last write position keeps their reads off any replica that has not
	// replayed it.
	ReadYourWritesTTL time.Duration
}

// Valkey is where read-your-writes positions, sessions and the worker's jobs live.
type Valkey struct {
	URL string
	// JobStream is the stream the api adds jobs to and the worker reads them from.
	JobStream string
}

// Load reads the environment and refuses it, naming every problem at once, when something is missing or wrong.
func Load() (Config, error) {
	var l loader
	cfg := Config{
		Env:      l.oneOf("BEARING_ENV", EnvProduction, EnvDevelopment, EnvStaging, EnvProduction),
		LogLevel: l.oneOf("BEARING_LOG_LEVEL", "info", "debug", "info", "warn", "error"),
		HTTP: HTTP{
			Addr:            l.str("BEARING_HTTP_ADDR", DefaultHTTPAddr),
			RequestTimeout:  l.duration("BEARING_REQUEST_TIMEOUT", DefaultRequestTimeout),
			ShutdownTimeout: l.duration("BEARING_SHUTDOWN_TIMEOUT", DefaultShutdownTimeout),
		},
		DB: DB{
			PrimaryURL:        l.str("BEARING_DB_PRIMARY_URL", ""),
			ReplicaURLs:       l.list("BEARING_DB_REPLICA_URLS"),
			AdminURL:          l.str("BEARING_DB_ADMIN_URL", ""),
			PrimaryMaxConns:   int32(l.positive("BEARING_DB_PRIMARY_MAX_CONNS", DefaultMaxConns)),
			ReplicaMaxConns:   int32(l.positive("BEARING_DB_REPLICA_MAX_CONNS", DefaultMaxConns)),
			MinConns:          int32(l.positive("BEARING_DB_MIN_CONNS", DefaultMinConns)),
			ConnMaxLifetime:   l.duration("BEARING_DB_CONN_MAX_LIFETIME", DefaultConnMaxLifetime),
			HealthInterval:    l.duration("BEARING_DB_HEALTH_INTERVAL", DefaultHealthInterval),
			MaxReplicaLag:     l.duration("BEARING_DB_MAX_REPLICA_LAG", DefaultMaxReplicaLag),
			ReplicaLagSamples: l.positive("BEARING_DB_REPLICA_LAG_SAMPLES", DefaultLagSamples),
			ReadYourWritesTTL: l.duration("BEARING_READ_YOUR_WRITES_TTL", DefaultReadYourWrites),
		},
		Valkey: Valkey{
			URL:       l.str("BEARING_VALKEY_URL", ""),
			JobStream: l.str("BEARING_JOB_STREAM", DefaultJobStream),
		},
	}
	cfg.Auth = Auth{
		AppURL:        strings.TrimRight(l.str("BEARING_APP_URL", DefaultAppURL), "/"),
		SessionCookie: l.str("BEARING_SESSION_COOKIE", DefaultSessionCookie),
		SessionTTL:    l.duration("BEARING_SESSION_TTL", DefaultSessionTTL),
		InvitationTTL: l.duration("BEARING_INVITATION_TTL", DefaultInvitationTTL),
		ResetTTL:      l.duration("BEARING_RESET_TTL", DefaultResetTTL),
	}
	cfg.Mail = Mail{SMTPAddr: l.str("BEARING_SMTP_ADDR", ""), From: l.str("BEARING_MAIL_FROM", DefaultMailFrom)}
	cfg.Bootstrap = Bootstrap{
		Name:          l.str("BEARING_NAME", DefaultName),
		AdminName:     l.str("BEARING_BOOTSTRAP_ADMIN_NAME", "Administrator"),
		AdminEmail:    l.str("BEARING_BOOTSTRAP_ADMIN_EMAIL", ""),
		AdminPassword: os.Getenv("BEARING_BOOTSTRAP_ADMIN_PASSWORD"),
	}
	cfg.Bootstrap.Members = l.str("BEARING_BOOTSTRAP_MEMBERS", "")
	cfg.Bootstrap.Demo = l.boolean("BEARING_DEMO", false)
	cfg.OIDC = OIDC{
		Issuer:       strings.TrimRight(l.str("BEARING_OIDC_ISSUER", ""), "/"),
		ClientID:     l.str("BEARING_OIDC_CLIENT_ID", ""),
		ClientSecret: os.Getenv("BEARING_OIDC_CLIENT_SECRET"),
		GroupsClaim:  l.str("BEARING_OIDC_GROUPS_CLAIM", DefaultGroupsClaim),
		Backchannel:  l.str("BEARING_OIDC_BACKCHANNEL", ""),
	}
	if cfg.OIDC.Enabled() && cfg.OIDC.ClientID == "" {
		l.problem("BEARING_OIDC_ISSUER is set without BEARING_OIDC_CLIENT_ID; name the client Bearing is registered as at the provider.")
	}
	if cfg.Bootstrap.AdminEmail != "" && cfg.Bootstrap.AdminPassword == "" {
		l.problem("BEARING_BOOTSTRAP_ADMIN_EMAIL is set without BEARING_BOOTSTRAP_ADMIN_PASSWORD; give the first administrator a password, or leave both unset.")
	}
	cfg.HTTP.SecureCookies = l.boolean("BEARING_SECURE_COOKIES", cfg.Env != EnvDevelopment)
	if cfg.DB.PrimaryURL == "" {
		l.problem("BEARING_DB_PRIMARY_URL is not set; name the primary database, such as postgres://bearing_app:...@host:5432/bearing.")
	}
	if cfg.DB.AdminURL == "" {
		cfg.DB.AdminURL = cfg.DB.PrimaryURL
	}
	if len(l.problems) > 0 {
		return cfg, errors.New("the configuration cannot be used:\n  " + strings.Join(l.problems, "\n  "))
	}
	return cfg, nil
}

// RequireValkey refuses a configuration without Valkey, for the processes that cannot run without it.
func (c Config) RequireValkey() error {
	if c.Valkey.URL == "" {
		return errors.New("BEARING_VALKEY_URL is not set; name the Valkey server, such as redis://:password@valkey:6379/0")
	}
	return nil
}
