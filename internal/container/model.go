package container

import (
	"context"
	"time"
)

const (
	DefaultTimeout = 5 * time.Second
	LogTailLines   = 100
	MaxLogBytes    = 1024 * 1024
)

type Container struct {
	ID     string
	Name   string
	Image  string
	State  string
	Status string
}

type RegisteredServer struct {
	ID             string
	Name           string
	ConnectionType string
}

type ServerLookup func(context.Context, string) (RegisteredServer, bool, error)

type ServerLister func(context.Context) ([]RegisteredServer, error)
