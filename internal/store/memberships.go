package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rushikeshg25/runnelix/internal/identity"
)

var ErrForbidden = errors.New("permission denied")

type Permission uint8

const (
	Read Permission = 1 << iota
	Edit
	Publish
	Execute
	DataRead
	Manage
)

type Membership struct {
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
	Role     string `json:"role"`
	Publish  bool   `json:"publish"`
	Execute  bool   `json:"execute"`
	DataRead bool   `json:"dataRead"`
}

func (m Membership) Allows(required Permission) bool {
	var allowed Permission
	switch m.Role {
	case "owner":
		allowed = Read | Edit | Publish | Execute | DataRead | Manage
	case "viewer":
		allowed = Read
	case "editor":
		allowed = Read | Edit
	case "operator":
		allowed = Read | Execute
	default:
		return false
	}
	if m.DataRead {
		allowed |= DataRead
	}
	if m.Role == "editor" || m.Role == "operator" {
		if m.Publish {
			allowed |= Publish
		}
		if m.Execute {
			allowed |= Execute
		}
	}
	return required != 0 && allowed&required == required
}

func (s *Store) Membership(ctx context.Context, workspace string, p identity.Principal) (Membership, error) {
	var m Membership
	err := s.Pool.QueryRow(ctx, `SELECT issuer,subject,role,publish,execute,data_read FROM workspace_memberships WHERE workspace_id=$1 AND issuer=$2 AND subject=$3`, workspace, p.Issuer, p.Subject).Scan(&m.Issuer, &m.Subject, &m.Role, &m.Publish, &m.Execute, &m.DataRead)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrForbidden
	}
	return m, err
}

func validMember(m Membership) bool {
	return strings.TrimSpace(m.Issuer) != "" && len(m.Issuer) <= 2048 && strings.TrimSpace(m.Subject) != "" && len(m.Subject) <= 255 && !strings.ContainsRune(m.Issuer, 0) && !strings.ContainsRune(m.Subject, 0)
}

// ProvisionOwner is an explicit installation operation, never a first-login side effect.
// It is allowed only before the workspace has any members.
func (s *Store) ProvisionOwner(ctx context.Context, workspace string, p identity.Principal) error {
	return s.changeMembership(ctx, workspace, nil, Membership{Issuer: p.Issuer, Subject: p.Subject, Role: "owner"}, false)
}

func (s *Store) ChangeMembership(ctx context.Context, workspace string, actor identity.Principal, member Membership, remove bool) error {
	return s.changeMembership(ctx, workspace, &actor, member, remove)
}

func (s *Store) changeMembership(ctx context.Context, workspace string, actor *identity.Principal, m Membership, remove bool) error {
	if !validMember(m) || (!remove && m.Role != "owner" && m.Role != "viewer" && m.Role != "editor" && m.Role != "operator") {
		return ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ws string
	if err = tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, workspace).Scan(&ws); err != nil {
		return notFound(err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM workspace_memberships WHERE workspace_id=$1`, workspace).Scan(&count); err != nil {
		return err
	}
	if actor == nil {
		if count != 0 {
			return ErrConflict
		}
	} else {
		var role string
		err = tx.QueryRow(ctx, `SELECT role FROM workspace_memberships WHERE workspace_id=$1 AND issuer=$2 AND subject=$3`, workspace, actor.Issuer, actor.Subject).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "owner") {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
	}
	var old string
	err = tx.QueryRow(ctx, `SELECT role FROM workspace_memberships WHERE workspace_id=$1 AND issuer=$2 AND subject=$3`, workspace, m.Issuer, m.Subject).Scan(&old)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if old == "owner" && (remove || m.Role != "owner") {
		var owners int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM workspace_memberships WHERE workspace_id=$1 AND role='owner'`, workspace).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrConflict
		}
	}
	if remove {
		if old == "" {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id=$1 AND issuer=$2 AND subject=$3`, workspace, m.Issuer, m.Subject)
	} else {
		if old == "" && count >= 100 {
			return ErrLimit
		}
		_, err = tx.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,issuer,subject,role,publish,execute,data_read) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(workspace_id,issuer,subject) DO UPDATE SET role=excluded.role,publish=excluded.publish,execute=excluded.execute,data_read=excluded.data_read`, workspace, m.Issuer, m.Subject, m.Role, m.Publish, m.Execute, m.DataRead)
	}
	if err != nil {
		return err
	}
	action := "membership.set"
	details := AuditDetails{Role: m.Role, PreviousRole: old, Grants: &AuditGrants{Publish: m.Publish, Execute: m.Execute, DataRead: m.DataRead}}
	if actor == nil {
		action = "membership.provisioned"
	} else {
		ctx = WithAuditContext(ctx, PrincipalActor(*actor), auditRequestID(ctx))
	}
	if remove {
		action = "membership.removed"
		details = AuditDetails{PreviousRole: old}
	}
	if err = appendAudit(ctx, tx, workspace, action, PrincipalActor(identity.Principal{Issuer: m.Issuer, Subject: m.Subject}), details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Memberships(ctx context.Context, workspace string) ([]Membership, error) {
	rows, err := s.Pool.Query(ctx, `SELECT issuer,subject,role,publish,execute,data_read FROM workspace_memberships WHERE workspace_id=$1 ORDER BY issuer,subject LIMIT 100`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Membership{}
	for rows.Next() {
		var m Membership
		if err = rows.Scan(&m.Issuer, &m.Subject, &m.Role, &m.Publish, &m.Execute, &m.DataRead); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
