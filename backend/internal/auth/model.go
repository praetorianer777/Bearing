// Package auth is who may come in and how: the installation's setup, accounts with passwords, sessions, invitations
// and password resets.
package auth

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Role is what an account may do in the installation.
type Role string

const (
	RoleAdministrator Role = "administrator"
	RoleEditor        Role = "editor"
	RoleMember        Role = "member"
	RoleReader        Role = "reader"
)

// Roles are every role, from the most it may do to the least.
var Roles = []Role{RoleAdministrator, RoleEditor, RoleMember, RoleReader}

// Valid says whether r is a role.
func (r Role) Valid() bool { return slices.Contains(Roles, r) }

// Account is somebody who signs in.
type Account struct {
	ID       uuid.UUID
	Email    string
	Name     string
	Role     Role
	Disabled bool
	// HasPassword is whether the account can sign in with a password, as opposed to only through single sign-on.
	HasPassword bool
	// Groups are what the identity provider last said the person belongs to; empty for a local account.
	Groups    []string
	CreatedAt time.Time
}

// Installation is the one organisation this installation serves.
type Installation struct {
	Name  string
	SetUp bool
}

// Invitation is an open invitation, as administrators see it.
type Invitation struct {
	ID        uuid.UUID
	Email     string
	Role      Role
	InvitedBy *uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// The refusals the service gives, each a sentence a person can act on.
var (
	ErrAlreadySetUp     = errors.New("this installation is already set up")
	ErrNotSetUp         = errors.New("this installation is not set up yet")
	ErrBadCredentials   = errors.New("that email and password do not match an account")
	ErrDisabled         = errors.New("this account is switched off; ask an administrator")
	ErrEmailTaken       = errors.New("an account with this email exists already")
	ErrLinkInvalid      = errors.New("this link is unknown, used or expired; ask for a new one")
	ErrNotFound         = errors.New("there is no such account")
	ErrSelf             = errors.New("you cannot switch off your own account")
	ErrInvalidRole      = errors.New("that is not a role")
	ErrNoPasswordToMail = errors.New("this installation sends no mail")
	// ErrAwaitingAdmission is somebody the provider vouches for whom nobody has let in yet; they have asked to join.
	ErrAwaitingAdmission = errors.New("you have asked to join; an administrator lets you in")
)

// Identity is who single sign-on says somebody is.
type Identity struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
	Groups  []string
}

// JoinRequest is somebody the provider vouches for who asked to join.
type JoinRequest struct {
	ID          uuid.UUID
	Email       string
	Name        string
	Groups      []string
	RequestedAt time.Time
}

// GroupAdmission lets the members of a provider's group in at once, with a role.
type GroupAdmission struct {
	Group string
	Role  Role
}
