package container

import (
	"testing"
	"time"
)

func TestStatusCacheIsFreshForEachProcessInstance(t *testing.T) {
	checkedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	firstProcess := NewStatusCache()
	firstProcess.MarkConnected("server-1", checkedAt)
	if firstProcess.Get("server-1").State != StatusConnected {
		t.Fatal("first process did not retain its observation")
	}

	restartedProcess := NewStatusCache()
	status := restartedProcess.Get("server-1")
	if status.State != StatusNotChecked || !status.CheckedAt.IsZero() || status.ErrorReference != "" {
		t.Fatalf("restart retained stale Docker observation: %#v", status)
	}
}

func TestStatusCachePreservesFreshnessAndFailureReference(t *testing.T) {
	checkedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	cache := NewStatusCache()
	cache.MarkUnavailable("server-1", checkedAt, "err_test")

	status := cache.Get("server-1")
	if status.State != StatusUnavailable || !status.CheckedAt.Equal(checkedAt) || status.CheckedAt.Location() != time.UTC || status.ErrorReference != "err_test" {
		t.Fatalf("unexpected cached status: %#v", status)
	}
}
