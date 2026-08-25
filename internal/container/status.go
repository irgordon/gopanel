package container

import (
	"sync"
	"time"
)

type StatusState string

const (
	StatusNotChecked  StatusState = "not_checked"
	StatusChecking    StatusState = "checking"
	StatusConnected   StatusState = "connected"
	StatusUnavailable StatusState = "unavailable"
)

type Status struct {
	ServerID       string
	State          StatusState
	CheckedAt      time.Time
	ErrorReference string
}

type StatusCache struct {
	mutex    sync.RWMutex
	statuses map[string]Status
}

func NewStatusCache() *StatusCache {
	return &StatusCache{statuses: make(map[string]Status)}
}

func (cache *StatusCache) Get(serverID string) Status {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	status, ok := cache.statuses[serverID]
	if !ok {
		return Status{ServerID: serverID, State: StatusNotChecked}
	}
	return status
}

func (cache *StatusCache) MarkChecking(serverID string) {
	cache.store(Status{ServerID: serverID, State: StatusChecking})
}

func (cache *StatusCache) MarkConnected(serverID string, checkedAt time.Time) {
	cache.store(Status{ServerID: serverID, State: StatusConnected, CheckedAt: checkedAt.UTC()})
}

func (cache *StatusCache) MarkUnavailable(serverID string, checkedAt time.Time, reference string) {
	cache.store(Status{ServerID: serverID, State: StatusUnavailable, CheckedAt: checkedAt.UTC(), ErrorReference: reference})
}

func (cache *StatusCache) store(status Status) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.statuses[status.ServerID] = status
}
