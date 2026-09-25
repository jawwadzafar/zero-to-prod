// Package rebac is a tiny relationship-based access control checker in the
// style of Google's Zanzibar (chapter 7.10): permissions come from stored
// relationships ("ada is an editor of folder:launch") plus a few rules
// ("editors are also viewers"; "viewers of a folder can view its links").
// Real systems (SpiceDB, OpenFGA, and others) add a schema language, a
// database, consistency tokens and caching; the core idea fits on a page.
package rebac

import "fmt"

// Tuple is one stored relationship: Subject has Relation on Object.
// A subject may be a user ("user:ada") or everyone with a relation on
// another object ("team:growth#member"): a userset.
type Tuple struct {
	Object   string // "link:q3-launch", "folder:launch", "team:growth"
	Relation string // "owner", "editor", "viewer", "parent", "member"
	Subject  string // "user:ada", or "team:growth#member", or "folder:launch" (for parent)
}

// Rule says how a relation is computed from others, for one object type.
type Rule struct {
	Implied    []string          // relations that also grant this one: editor ⇒ viewer
	FromParent map[string]string // via "parent" tuples: viewer of parent ⇒ viewer here
}

// Store holds tuples and the rules for each object type.
type Store struct {
	tuples []Tuple
	Rules  map[string]map[string]Rule // object type → relation → rule
}

func (s *Store) Write(t ...Tuple) { s.tuples = append(s.tuples, t...) }

// Check answers: does user have relation on object?
func (s *Store) Check(object, relation, user string) bool {
	return s.check(object, relation, user, map[string]bool{}, 0)
}

func (s *Store) check(object, relation, user string, seen map[string]bool, depth int) bool {
	key := object + "#" + relation
	if seen[key] || depth > 25 { // cycles and runaway nesting end here
		return false
	}
	seen[key] = true
	defer delete(seen, key)

	for _, t := range s.tuples {
		if t.Object != object || t.Relation != relation {
			continue
		}
		if t.Subject == user {
			return true // a direct relationship
		}
		// A userset: "team:growth#member" — is the user a member of the team?
		if obj, rel, ok := cutLast(t.Subject, "#"); ok && s.check(obj, rel, user, seen, depth+1) {
			return true
		}
	}

	rule := s.Rules[typeOf(object)][relation]
	for _, stronger := range rule.Implied {
		if s.check(object, stronger, user, seen, depth+1) {
			return true
		}
	}
	for via, parentRelation := range rule.FromParent {
		for _, t := range s.tuples {
			if t.Object == object && t.Relation == via && s.check(t.Subject, parentRelation, user, seen, depth+1) {
				return true
			}
		}
	}
	return false
}

func typeOf(object string) string {
	for i := range len(object) {
		if object[i] == ':' {
			return object[:i]
		}
	}
	return object
}

func cutLast(s, sep string) (string, string, bool) {
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

// Explain is Check for humans: it prints the question and the answer.
func (s *Store) Explain(object, relation, user string) string {
	return fmt.Sprintf("can %s %s %s? %t", user, relation, object, s.Check(object, relation, user))
}
