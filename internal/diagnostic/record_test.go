package diagnostic

import (
	"bytes"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestRecorderRecordsAndLooksUpOpaqueReference(t *testing.T) {
	recorder, _ := newTestRecorder()
	record := recorder.Record(Input{
		Event:           "database_open_failed",
		Component:       "sqlite",
		PublicMessage:   "SQLite could not be opened.",
		TechnicalDetail: "database is unavailable",
	})

	if !regexp.MustCompile(`^err_[0-9a-f]{32}$`).MatchString(record.ID) {
		t.Fatalf("expected opaque reference, got %q", record.ID)
	}

	stored, found := recorder.Lookup(record.ID)
	if !found {
		t.Fatal("expected recorded diagnostic to be found")
	}
	if stored != record {
		t.Fatalf("expected lookup to return recorded value: got %#v", stored)
	}
}

func TestRecorderRedactsSensitiveValues(t *testing.T) {
	recorder, logs := newTestRecorder()
	detail := `password=hunter2 passwd='legacy' pwd="short" token=abc123 access_token=access-value refresh_token: refresh-value API_KEY=api-value secret=secret-value Authorization: Bearer bearer-value Authorization: Basic YmFzaWM= Cookie: cookie-value Set-Cookie: session=set-cookie-value; HttpOnly https://user:pass@example.com`
	record := recorder.Record(Input{
		Event:           "configuration_rejected",
		Component:       "config",
		PublicMessage:   "token=public-secret",
		TechnicalDetail: detail,
	})

	for _, sensitive := range []string{"hunter2", "legacy", "short", "abc123", "access-value", "refresh-value", "api-value", "secret-value", "bearer-value", "YmFzaWM=", "cookie-value", "set-cookie-value", "user:pass", "public-secret"} {
		if strings.Contains(record.PublicMessage, sensitive) || strings.Contains(record.TechnicalDetail, sensitive) {
			t.Fatalf("record retained sensitive value %q", sensitive)
		}
		if strings.Contains(logs.String(), sensitive) {
			t.Fatalf("log retained sensitive value %q", sensitive)
		}
	}

	if !strings.Contains(record.TechnicalDetail, "[REDACTED]") {
		t.Fatal("expected visible redaction marker")
	}
}

func TestRecorderEvictsOldestRecordAtCapacity(t *testing.T) {
	recorder, _ := newTestRecorder()
	identifiers := make([]string, 0, MaxRecords+5)
	for index := 0; index < MaxRecords+5; index++ {
		record := recorder.Record(Input{Event: fmt.Sprintf("event_%03d", index)})
		identifiers = append(identifiers, record.ID)
	}

	snapshot := recorder.Snapshot()
	if len(snapshot) != MaxRecords {
		t.Fatalf("expected %d records, got %d", MaxRecords, len(snapshot))
	}
	if _, found := recorder.Lookup(identifiers[0]); found {
		t.Fatal("expected oldest record to be evicted")
	}
	if snapshot[0].ID != identifiers[5] {
		t.Fatalf("expected deterministic oldest retention at index 5, got %q", snapshot[0].ID)
	}
	if snapshot[len(snapshot)-1].ID != identifiers[len(identifiers)-1] {
		t.Fatal("expected newest record to remain last")
	}
}

func TestRecorderCorrelatesRecordAndStructuredLog(t *testing.T) {
	recorder, logs := newTestRecorder()
	record := recorder.Record(Input{
		Event:           "migration_failed",
		Component:       "sqlite",
		TechnicalDetail: "execute migration 0001",
	})

	if !strings.Contains(logs.String(), "error_ref="+record.ID) {
		t.Fatalf("expected log to contain record reference %q; log=%q", record.ID, logs.String())
	}
	if !strings.Contains(logs.String(), "event=migration_failed") {
		t.Fatalf("expected stable event name; log=%q", logs.String())
	}
}

func TestRecorderSupportsConcurrentAccess(t *testing.T) {
	recorder, _ := newTestRecorder()
	start := make(chan struct{})
	var group sync.WaitGroup

	for worker := 0; worker < 32; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			<-start
			for entry := 0; entry < 32; entry++ {
				record := recorder.Record(Input{Event: fmt.Sprintf("worker_%d_entry_%d", worker, entry)})
				recorder.Lookup(record.ID)
				recorder.Snapshot()
			}
		}(worker)
	}

	close(start)
	group.Wait()

	if len(recorder.Snapshot()) != MaxRecords {
		t.Fatalf("expected concurrent writes to retain %d records", MaxRecords)
	}
}

func newTestRecorder() (*Recorder, *bytes.Buffer) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	return NewRecorder(logger), &logs
}
