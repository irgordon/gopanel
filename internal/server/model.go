package server

import "time"

type Server struct {
	ID                  string
	Name                string
	Address             string
	ConnectionType      string
	CredentialReference *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Input struct {
	Name                string
	Address             string
	ConnectionType      string
	CredentialReference string
}
