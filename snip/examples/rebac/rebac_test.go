package rebac

import "testing"

// snip with sharing: links live in folders; folders are shared with people and teams.
func newStore() *Store {
	s := &Store{Rules: map[string]map[string]Rule{
		"folder": {
			"viewer": {Implied: []string{"editor"}},
			"editor": {Implied: []string{"owner"}},
		},
		"link": {
			"viewer": {Implied: []string{"editor"}, FromParent: map[string]string{"parent": "viewer"}},
			"editor": {Implied: []string{"owner"}, FromParent: map[string]string{"parent": "editor"}},
		},
	}}
	s.Write(
		Tuple{"folder:launch", "owner", "user:ada"},
		Tuple{"folder:launch", "viewer", "team:growth#member"},
		Tuple{"team:growth", "member", "user:grace"},
		Tuple{"link:q3-launch", "parent", "folder:launch"},
		Tuple{"link:private", "owner", "user:bob"},
	)
	return s
}

func TestRelationships(t *testing.T) {
	s := newStore()
	cases := []struct {
		object, relation, user string
		want                   bool
	}{
		{"folder:launch", "owner", "user:ada", true},     // direct
		{"folder:launch", "viewer", "user:ada", true},    // owner ⇒ editor ⇒ viewer
		{"link:q3-launch", "editor", "user:ada", true},   // folder editor ⇒ link editor
		{"link:q3-launch", "viewer", "user:grace", true}, // team member ⇒ folder viewer ⇒ link viewer
		{"link:q3-launch", "editor", "user:grace", false},
		{"link:private", "viewer", "user:ada", false},
		{"link:q3-launch", "viewer", "user:mallory", false},
	}
	for _, c := range cases {
		if got := s.Check(c.object, c.relation, c.user); got != c.want {
			t.Errorf("%s", s.Explain(c.object, c.relation, c.user))
		}
	}
}

func TestRemovingATupleRevokesEverythingDerivedFromIt(t *testing.T) {
	s := newStore()
	// Grace leaves the team: remove one tuple, and every access that came through it is gone.
	var kept []Tuple
	for _, tp := range s.tuples {
		if tp != (Tuple{"team:growth", "member", "user:grace"}) {
			kept = append(kept, tp)
		}
	}
	s.tuples = kept
	if s.Check("link:q3-launch", "viewer", "user:grace") {
		t.Fatal("access derived from team membership must disappear with it")
	}
}

func TestCyclesTerminate(t *testing.T) {
	s := newStore()
	s.Write(
		Tuple{"team:a", "member", "team:b#member"},
		Tuple{"team:b", "member", "team:a#member"},
	)
	if s.Check("team:a", "member", "user:nobody") {
		t.Fatal("a cycle must not grant access")
	}
}
