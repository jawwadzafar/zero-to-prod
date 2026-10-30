package memory_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/audit"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/store/memory"
)

// Every change to keys and links leaves one audit event; failed changes
// and reads leave none.
func TestAuditTrail(t *testing.T) {
	st := memory.New()
	ctx := audit.WithRequestID(context.Background(), "req-1")
	key, owner, err := auth.CreateKey(context.Background(), st, "alice") // like `snip keys create`: no request
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RotateKey(context.Background(), st, owner.ID); err != nil {
		t.Fatal(err)
	}
	l := links.Link{Slug: "docs", URL: "https://go.dev", OwnerID: owner.ID, CreatedAt: time.Now()}
	if _, err := st.Create(ctx, l); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, l); !errors.Is(err, links.ErrSlugTaken) { // failed: no event
		t.Fatalf("duplicate create = %v", err)
	}
	if err := st.Delete(ctx, "docs", owner.ID+1); !errors.Is(err, links.ErrNotFound) { // not theirs: no event
		t.Fatalf("delete by another owner = %v", err)
	}
	if _, err := st.Get(ctx, "docs"); err != nil { // reads aren't audited
		t.Fatal(err)
	}
	if err := st.Delete(ctx, "docs", owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeKey(context.Background(), owner.ID); err != nil {
		t.Fatal(err)
	}

	got, err := st.AuditLog(context.Background(), owner.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		action    audit.Action
		slug, req string
		detail    map[string]string
	}{ // newest first
		{audit.KeyRevoked, "", "", nil},
		{audit.LinkDeleted, "docs", "req-1", map[string]string{"url": "https://go.dev"}},
		{audit.LinkCreated, "docs", "req-1", map[string]string{"url": "https://go.dev"}},
		{audit.KeyRotated, "", "", nil},
		{audit.KeyCreated, "", "", map[string]string{"name": "alice"}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		e := got[i]
		if e.Action != w.action || e.Slug != w.slug || e.RequestID != w.req || e.OwnerID != owner.ID ||
			len(e.Detail) != len(w.detail) || e.Detail["url"] != w.detail["url"] || e.Detail["name"] != w.detail["name"] {
			t.Errorf("event %d = %+v, want %+v", i, e, w)
		}
		for _, v := range e.Detail {
			if strings.Contains(v, key) || strings.Contains(v, auth.HashKey(key)) {
				t.Errorf("event %d holds the API key or its hash", i)
			}
		}
	}

	if limited, _ := st.AuditLog(context.Background(), owner.ID, 2); len(limited) != 2 || limited[0].Action != audit.KeyRevoked {
		t.Errorf("AuditLog with limit 2 = %+v", limited)
	}
	if other, _ := st.AuditLog(context.Background(), owner.ID+1, 100); len(other) != 0 {
		t.Errorf("another owner's trail = %+v, want empty", other)
	}
}
