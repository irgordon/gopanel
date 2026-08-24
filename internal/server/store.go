package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, input Input) (Server, error) {
	id, err := randomID()
	if err != nil {
		return Server{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var credRef *string
	if input.CredentialReference != "" {
		v := input.CredentialReference
		credRef = &v
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO servers(id, name, address, connection_type, credential_reference, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
		id, input.Name, input.Address, input.ConnectionType, credRef, now, now)
	if err != nil {
		return Server{}, fmt.Errorf("create server: %w", err)
	}
	return Server{ID: id, Name: input.Name, Address: input.Address, ConnectionType: input.ConnectionType, CredentialReference: credRef}, nil
}

func (s *Store) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, address, connection_type, credential_reference, created_at, updated_at FROM servers ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var srv Server
		var created, updated string
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.Address, &srv.ConnectionType, &srv.CredentialReference, &created, &updated); err != nil {
			return nil, err
		}
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (Server, error) {
	var srv Server
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, name, address, connection_type, credential_reference, created_at, updated_at FROM servers WHERE id=?`, id).Scan(
		&srv.ID, &srv.Name, &srv.Address, &srv.ConnectionType, &srv.CredentialReference, &created, &updated)
	if err != nil {
		return Server{}, err
	}
	return srv, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
