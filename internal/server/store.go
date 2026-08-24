package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, id string, input ValidatedInput) (Server, error) {
	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO servers(id, name, address, connection_type, credential_reference, created_at, updated_at) VALUES(?,?,?,?,NULL,?,?)`,
		id, input.Name, input.Address, input.ConnectionType, timestamp, timestamp)
	if err != nil {
		return Server{}, fmt.Errorf("create server: %w", err)
	}
	return Server{ID: id, Name: input.Name, Address: input.Address, ConnectionType: input.ConnectionType, CreatedAt: now, UpdatedAt: now}, nil
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
			return nil, fmt.Errorf("scan server: %w", err)
		}
		if err := assignTimestamps(&srv, created, updated); err != nil {
			return nil, err
		}
		servers = append(servers, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate servers: %w", err)
	}
	return servers, nil
}

func (s *Store) Get(ctx context.Context, id string) (Server, error) {
	var srv Server
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, name, address, connection_type, credential_reference, created_at, updated_at FROM servers WHERE id=?`, id).Scan(
		&srv.ID, &srv.Name, &srv.Address, &srv.ConnectionType, &srv.CredentialReference, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	if err != nil {
		return Server{}, fmt.Errorf("get server: %w", err)
	}
	if err := assignTimestamps(&srv, created, updated); err != nil {
		return Server{}, err
	}
	return srv, nil
}

func assignTimestamps(server *Server, created, updated string) error {
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return fmt.Errorf("parse server creation time: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return fmt.Errorf("parse server update time: %w", err)
	}
	server.CreatedAt = createdAt
	server.UpdatedAt = updatedAt
	return nil
}
