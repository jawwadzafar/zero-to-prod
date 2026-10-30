package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The roles migration 0003 creates. Neither can sign in by itself; EnsureLogin
// makes a login a member of one (or the role itself, if the login has its name).
const (
	AppRole      = "snip_app"      // the API and the worker: read and write rows, append audit events
	ReadonlyRole = "snip_readonly" // analytics extracts: read, never key hashes
)

// EnsureLogin makes sure the user named in databaseURL can sign in with the
// password in databaseURL and holds role's privileges: it creates the login,
// or resets its password if it exists. s must be connected as the schema's
// owner (SNIP_MIGRATE_DATABASE_URL). It returns the user name, never the
// password, so the caller may log it.
//
// The password never reaches the server as text: snip sends a SCRAM-SHA-256
// verifier (a salted hash), as psql's \password does, so a server that logs
// DDL statements (log_statement = 'ddl') records only the hash.
func (s *Store) EnsureLogin(ctx context.Context, databaseURL, role string) (string, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return "", fmt.Errorf("login for %s: %w", role, err)
	}
	user, password := cfg.User, cfg.Password
	if user == "" || password == "" {
		return "", fmt.Errorf("login for %s: the connection URL needs a user name and a password", role)
	}
	verifier, err := scramVerifier(password)
	if err != nil {
		return "", fmt.Errorf("login %q: %w", user, err)
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Release()
	// The same lock as Migrate: two migrators granting at once can fail with
	// "tuple concurrently updated".
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(7412)`); err != nil {
		return "", err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7412)`) //nolint:errcheck

	var self, exists, super bool
	err = conn.QueryRow(ctx, `
		SELECT $1 = current_user,
		       EXISTS (SELECT FROM pg_roles WHERE rolname = $1),
		       EXISTS (SELECT FROM pg_roles WHERE rolname = $1 AND rolsuper)`, user,
	).Scan(&self, &exists, &super)
	if err != nil {
		return "", err
	}
	switch {
	case self:
		return "", fmt.Errorf("login %q is the schema owner's own login: give the app a login of its own", user)
	case super:
		return "", fmt.Errorf("login %q is a superuser, which ignores every privilege: give the app a login of its own", user)
	}

	// Names and passwords can't be query parameters in CREATE/ALTER ROLE, so
	// they are quoted: the name as an identifier, the verifier as a string
	// (it holds only base64 characters, '$' and ':').
	ident := pgx.Identifier{user}.Sanitize()
	verb := "CREATE"
	if exists {
		verb = "ALTER"
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`%s ROLE %s WITH LOGIN PASSWORD '%s'`, verb, ident, verifier)); err != nil {
		return "", fmt.Errorf("login %q: %w", user, err)
	}
	if user != role {
		if _, err := conn.Exec(ctx, fmt.Sprintf(`GRANT %s TO %s`, pgx.Identifier{role}.Sanitize(), ident)); err != nil {
			return "", fmt.Errorf("login %q: %w", user, err)
		}
	}
	return user, nil
}

// scramVerifier returns what Postgres stores for a SCRAM-SHA-256 password
// (RFC 5802 and RFC 7677): a random salt, the iteration count, and two keys
// derived from the password. Postgres accepts this in place of the password
// and can check logins against it, but the password can't be read back out.
func scramVerifier(password string) (string, error) {
	for _, r := range password {
		if r > 127 {
			// Postgres normalises non-ASCII passwords (SASLprep) before
			// hashing; snip doesn't, so it refuses them rather than store
			// a hash that would never match.
			return "", fmt.Errorf("the password must be ASCII")
		}
	}
	const iterations = 4096 // Postgres's default (scram_iterations)
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return "", err
	}
	clientKey := hmacSHA256(salted, "Client Key")
	storedKey := sha256.Sum256(clientKey)
	serverKey := hmacSHA256(salted, "Server Key")
	b64 := base64.StdEncoding.EncodeToString
	v := fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", iterations, b64(salt), b64(storedKey[:]), b64(serverKey))
	if strings.ContainsRune(v, '\'') { // impossible with base64; checked because it is quoted above
		return "", fmt.Errorf("unexpected quote in verifier")
	}
	return v, nil
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}
