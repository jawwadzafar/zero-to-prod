package testdemo

import (
	"errors"
	"fmt"
	"strings"
)

// Mailer is anything that can send an email. In production it would talk
// to an email provider over the network; in tests it is a fake.
type Mailer interface {
	Send(to, subject string) error
}

var (
	ErrInvalidEmail = errors.New("invalid email")
	ErrExists       = errors.New("already registered")
)

// Signup registers users and welcomes them by email.
type Signup struct {
	mail  Mailer
	users map[string]bool
}

// NewSignup is handed its Mailer from outside (dependency injection), so a
// test can hand it a fake one.
func NewSignup(m Mailer) *Signup {
	return &Signup{mail: m, users: map[string]bool{}}
}

// Register adds a user and sends one welcome email.
func (s *Signup) Register(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return ErrInvalidEmail
	}
	if s.users[email] {
		return ErrExists
	}
	s.users[email] = true
	if err := s.mail.Send(email, "Welcome!"); err != nil {
		return fmt.Errorf("send welcome email: %w", err)
	}
	return nil
}
