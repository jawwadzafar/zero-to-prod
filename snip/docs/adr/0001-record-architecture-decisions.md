# 1. Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-09-25

## Context

Decisions about snip's design were spread across code comments, commit
messages and the handbook's chapters. Someone joining later can see *what* the
code does, but not *why*, or which alternatives were considered and rejected.
That makes it easy to undo a good decision by accident.

## Decision

We record each significant decision as a short Markdown file in `docs/adr/`,
using this structure: title, status, date, context, decision, consequences.
"Significant" means hard to reverse, or affecting more than one component, or
something people will ask "why?" about.

Records are never rewritten. A changed decision gets a new record that says
which one it supersedes, and the old one's status becomes "Superseded by N".

## Consequences

- The reasoning survives the people who made it.
- Writing a record forces the alternatives and costs to be stated.
- It's a small, ongoing effort; a decision that isn't worth a paragraph isn't
  significant enough to need one.
