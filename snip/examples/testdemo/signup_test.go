package testdemo

import (
	"errors"
	"testing"
)

// fakeMailer is a hand-written test double. It sends nothing; it records what
// it was asked to send, and can be told to fail.
type fakeMailer struct {
	sent []string // who we were asked to email
	fail error    // if set, Send returns it
}

func (f *fakeMailer) Send(to, _ string) error {
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, to)
	return nil
}

func TestRegisterSendsOneWelcomeEmail(t *testing.T) {
	mail := &fakeMailer{}
	s := NewSignup(mail)

	if err := s.Register(" Ada@Example.com "); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(mail.sent) != 1 || mail.sent[0] != "ada@example.com" {
		t.Fatalf("sent = %v, want exactly one email to ada@example.com", mail.sent)
	}
}

func TestRegisterTwiceIsRefused(t *testing.T) {
	mail := &fakeMailer{}
	s := NewSignup(mail)
	_ = s.Register("ada@example.com")

	err := s.Register("ADA@example.com")
	if !errors.Is(err, ErrExists) {
		t.Fatalf("second Register error = %v, want ErrExists", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("sent %d emails, want 1: a refused signup must not email anyone", len(mail.sent))
	}
}

func TestRegisterReportsMailFailure(t *testing.T) {
	down := errors.New("smtp: connection refused")
	s := NewSignup(&fakeMailer{fail: down})

	err := s.Register("ada@example.com")
	if !errors.Is(err, down) {
		t.Fatalf("Register error = %v, want it to wrap the mailer's error", err)
	}
}
