// Package audit describes snip's audit trail: a lasting record of who
// changed what (chapter 7.4). Logs say what happened too, but logs rotate
// away and anyone with access can edit a file; the audit trail lives in the
// database, next to the data it describes, and can only grow.
//
// The stores write the events, in the same step as the change they record
// (internal/store/postgres: one transaction), so there is never a deleted
// link without its "link.deleted" row. This package only holds the shared
// vocabulary.
package audit

import (
	"context"
	"time"
)

// Action is what happened. The database accepts only these values (the
// CHECK on audit_events.action in migrations/0002_audit_events.sql).
type Action string

const (
	KeyCreated  Action = "key.created" // snip keys create (an operator)
	KeyRotated  Action = "key.rotated" // snip keys rotate (an operator)
	KeyRevoked  Action = "key.revoked" // snip keys revoke (an operator)
	LinkCreated Action = "link.created"
	LinkDeleted Action = "link.deleted"
)

// Event is one row of the audit trail. It never holds an API key or its
// hash: an audit trail is read by more people than the key table, and it
// can't be cleaned up afterwards.
type Event struct {
	ID        int64             `json:"id"`
	At        time.Time         `json:"at"`
	OwnerID   int64             `json:"owner_id"` // whose key or link it was
	Action    Action            `json:"action"`
	Slug      string            `json:"slug,omitempty"`       // for link events
	RequestID string            `json:"request_id,omitempty"` // matches the request's log line
	Detail    map[string]string `json:"detail,omitempty"`     // e.g. the URL a deleted link pointed to
}

// Reader lists an owner's events, newest first (`snip audit OWNER_ID`).
type Reader interface {
	AuditLog(ctx context.Context, ownerID int64, limit int) ([]Event, error)
}

type ctxKey struct{}

// WithRequestID returns ctx carrying the request's ID, so a store can put
// it in the audit row. snip's HTTP middleware (observe) calls it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID returns the ID put there by WithRequestID, or "" (for example
// for `snip keys` commands, which aren't HTTP requests).
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
