# Contributing to Zero to Prod

Thank you — every fix makes this better for the next person who's stuck.

## The easiest contributions (no setup)

- **"I got lost here."** Open an issue quoting the sentence and saying what you
  didn't understand. A reader getting lost is a bug in the handbook.
- **Typos and small fixes.** Click **Edit this page** at the bottom of any
  chapter and GitHub will walk you through a pull request.

## Bigger contributions

1. Read [STYLE.md](STYLE.md) — the ladder, the voice, the chapter anatomy.
2. Read [CLAUDE.md](CLAUDE.md) §3–4 for the repo map and how chapters are added.
3. `npm install && npm start`, make your change, then `npm run build` (it fails
   on broken links) and, for `snip/` changes, `cd snip && go test ./...`.
4. Open a pull request describing what a reader will now understand that they
   didn't before.

## Ground rules

- Real over invented; honest over impressive. No fake incidents or statistics.
- Every snip code snippet must match the real code in `snip/`.
- No secrets, real IPs, or personal hostnames — use placeholders.
- Be kind in reviews. Everyone here was a beginner once.
