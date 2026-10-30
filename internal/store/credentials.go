package store

import (
	"context"
	"github.com/google/uuid"
	"github.com/rushikeshg25/runnelix/internal/secrets"
	"net/url"
	"strings"
)

type Credential struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Destination string `json:"destination"`
	Revoked     bool   `json:"revoked"`
}

func (s *Store) CreateCredential(ctx context.Context, workspace, name, kind, value, destination string, box *secrets.Box) (Credential, error) {
	if box == nil || len(strings.TrimSpace(name)) == 0 || len(name) > 120 || (kind != "bearer" && kind != "webhook-hmac") || strings.ContainsAny(value, "\r\n") {
		return Credential{}, ErrInvalid
	}
	if kind == "bearer" {
		u, err := url.Parse("https://" + destination)
		if err != nil || u.Hostname() != destination || u.Port() != "" || strings.ToLower(destination) != destination {
			return Credential{}, ErrInvalid
		}
	} else if destination != "" {
		return Credential{}, ErrInvalid
	}
	id := "credential_" + uuid.NewString()
	encrypted, err := box.Seal(workspace+"/"+id, []byte(value))
	if err != nil {
		return Credential{}, ErrInvalid
	}
	_, err = s.Pool.Exec(ctx, "INSERT INTO credentials(workspace_id,id,name,kind,ciphertext,key_id,destination) VALUES($1,$2,$3,$4,$5,$6,$7)", workspace, id, name, kind, encrypted, box.KeyID, destination)
	return Credential{ID: id, Name: name, Kind: kind, Destination: destination}, err
}
func (s *Store) GrantCredential(ctx context.Context, workspace, credential, workflow string) error {
	_, err := s.Pool.Exec(ctx, "INSERT INTO credential_grants(workspace_id,credential_id,workflow_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", workspace, credential, workflow)
	return err
}
func (s *Store) CredentialAllowed(ctx context.Context, workspace, workflow, credential, kind string) error {
	var allowed bool
	err := s.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM credentials c JOIN credential_grants g ON (g.workspace_id,g.credential_id)=(c.workspace_id,c.id) WHERE c.workspace_id=$1 AND c.id=$2 AND g.workflow_id=$3 AND c.kind=$4 AND NOT c.revoked)", workspace, credential, workflow, kind).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrNotFound
	}
	return nil
}
func (s *Store) ResolveCredential(ctx context.Context, workspace, workflow, credential, kind, destination string, box *secrets.Box) (string, error) {
	if box == nil {
		return "", ErrInvalid
	}
	var data []byte
	var keyID string
	err := s.Pool.QueryRow(ctx, "SELECT c.ciphertext,c.key_id FROM credentials c JOIN credential_grants g ON (g.workspace_id,g.credential_id)=(c.workspace_id,c.id) WHERE c.workspace_id=$1 AND c.id=$2 AND g.workflow_id=$3 AND c.kind=$4 AND NOT c.revoked AND c.destination=$5", workspace, credential, workflow, kind, destination).Scan(&data, &keyID)
	if err != nil {
		return "", notFound(err)
	}
	value, err := box.Open(workspace+"/"+credential, keyID, data)
	if err != nil {
		return "", ErrInvalid
	}
	return string(value), nil
}
func (s *Store) Credentials(ctx context.Context, workspace, after string) ([]Credential, error) {
	rows, err := s.Pool.Query(ctx, "SELECT id,name,kind,destination,revoked FROM credentials WHERE workspace_id=$1 AND id>$2 ORDER BY id LIMIT 100", workspace, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Credential{}
	for rows.Next() {
		var c Credential
		if err = rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Destination, &c.Revoked); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// RotateCredential preserves the destination and grants. A revoked credential stays revoked.
func (s *Store) RotateCredential(ctx context.Context, workspace, id, value string, box *secrets.Box) error {
	if box == nil || strings.ContainsAny(value, "\r\n") {
		return ErrInvalid
	}
	encrypted, err := box.Seal(workspace+"/"+id, []byte(value))
	if err != nil {
		return ErrInvalid
	}
	tag, err := s.Pool.Exec(ctx, "UPDATE credentials SET ciphertext=$3,key_id=$4 WHERE workspace_id=$1 AND id=$2 AND NOT revoked", workspace, id, encrypted, box.KeyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

// RevokeCredential prevents future resolutions, including from already-published versions.
func (s *Store) RevokeCredential(ctx context.Context, workspace, id string) error {
	tag, err := s.Pool.Exec(ctx, "UPDATE credentials SET revoked=true WHERE workspace_id=$1 AND id=$2", workspace, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
