package diagnostic

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MaxRecords            = 200
	maxPublicMessageBytes = 512
	maxTechnicalBytes     = 4096
)

var fallbackReferenceCounter atomic.Uint64

type Input struct {
	Event              string
	Component          string
	PublicMessage      string
	TechnicalDetail    string
	UserID             string
	Action             string
	Target             string
	HTTPStatus         int
	AuditCorrelationID string
}

type Record struct {
	ID                 string
	CreatedAt          time.Time
	Event              string
	Component          string
	PublicMessage      string
	TechnicalDetail    string
	UserID             string
	Action             string
	Target             string
	HTTPStatus         int
	AuditCorrelationID string
}

type Recorder struct {
	logger  *slog.Logger
	mutex   sync.RWMutex
	records []Record
}

func NewRecorder(logger *slog.Logger) *Recorder {
	return &Recorder{
		logger:  logger,
		records: make([]Record, 0, MaxRecords),
	}
}

func (r *Recorder) Record(input Input) Record {
	record := buildRecord(input)
	r.appendRecord(record)
	r.logRecord(record)
	return record
}

func (r *Recorder) Lookup(reference string) (Record, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	for index := len(r.records) - 1; index >= 0; index-- {
		if r.records[index].ID == reference {
			return r.records[index], true
		}
	}
	return Record{}, false
}

func (r *Recorder) Snapshot() []Record {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return append([]Record(nil), r.records...)
}

func (r *Recorder) appendRecord(record Record) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.records) == MaxRecords {
		copy(r.records, r.records[1:])
		r.records[len(r.records)-1] = record
		return
	}
	r.records = append(r.records, record)
}

func (r *Recorder) logRecord(record Record) {
	r.logger.Error(
		"diagnostic",
		"event", record.Event,
		"error_ref", record.ID,
		"component", record.Component,
		"detail", record.TechnicalDetail,
		"user_id", record.UserID,
		"action", record.Action,
		"target", record.Target,
		"audit_id", record.AuditCorrelationID,
		"http_status", record.HTTPStatus,
	)
}

func buildRecord(input Input) Record {
	return Record{
		ID:                 newReference(),
		CreatedAt:          time.Now().UTC(),
		Event:              limitText(sanitizeDetail(input.Event), maxPublicMessageBytes),
		Component:          limitText(sanitizeDetail(input.Component), maxPublicMessageBytes),
		PublicMessage:      limitText(sanitizeDetail(input.PublicMessage), maxPublicMessageBytes),
		TechnicalDetail:    limitText(sanitizeDetail(input.TechnicalDetail), maxTechnicalBytes),
		UserID:             limitText(sanitizeDetail(input.UserID), maxPublicMessageBytes),
		Action:             limitText(sanitizeDetail(input.Action), maxPublicMessageBytes),
		Target:             limitText(sanitizeDetail(input.Target), maxPublicMessageBytes),
		HTTPStatus:         input.HTTPStatus,
		AuditCorrelationID: limitText(sanitizeDetail(input.AuditCorrelationID), maxPublicMessageBytes),
	}
}

func newReference() string {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err == nil {
		return "err_" + hex.EncodeToString(randomBytes)
	}
	fallbackInput := fmt.Sprintf("%d:%d", time.Now().UnixNano(), fallbackReferenceCounter.Add(1))
	digest := sha256.Sum256([]byte(fallbackInput))
	return "err_" + hex.EncodeToString(digest[:16])
}

func limitText(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return strings.TrimSpace(value[:maximum]) + "…"
}
