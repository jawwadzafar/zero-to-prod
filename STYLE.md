# Zero to Prod — Style Guide: how we write docs that *teach*

This guide is the standard for every chapter in this handbook. It exists so
that anyone — a career-switcher, a student, a product person, a frontend dev,
a curious founder — can start with **basic tech literacy** and climb, one rung
at a time, to **working backend + DevOps engineer**.

If you only remember one line:

> **Start where the reader already is, move one small step at a time, prove
> every idea on a real system they can run, and never leave them at a step
> they can't climb.**

---

## 1. Who we're writing for

### The reader at the start

Picture a **semi-technical person**. They:

- use computers all day and are comfortable with apps, browsers, files and
  folders;
- have maybe copy-pasted a terminal command from a runbook, or seen a JSON
  blob, or opened DevTools — but couldn't explain *what actually happened*;
- are smart, busy, and allergic to being talked down to.

They are **not** assumed to know: what a process, port, server, database
index, container, or protocol is. They may never have written code. Some
readers *are* developers (often frontend) — we serve them too, but we never
*require* that background.

### The reader at the end

By the end of a chapter (and much more so, a reading path), the same person
should be able to:

1. **Explain** the idea to someone else in plain words.
2. **Do it** — run it, build it, find it in `snip` (the project they build).
3. **Predict** what goes wrong and **diagnose** it when it does.
4. **Judge** trade-offs — why we chose this, what the alternative costs.

That's "0 → 100%". Every chapter is a ladder between those two readers.

### The test

Before publishing, ask: *could a sharp product manager with no coding
background follow the first half of this chapter, and could a senior engineer
still learn something from the second half?* If both answers aren't "yes",
the ladder is missing rungs (too hard) or stops too early (too shallow).

---

## 2. The depth ladder (how we go from 0 to 100%)

Every concept climbs the same five rungs, **in this order**. Never skip a
rung; never start on rung 3.

| Rung | Question it answers | What it looks like |
|---|---|---|
| **0 — Anchor** | "What is this *like*?" | An everyday analogy: apartment numbers, a phone book, a receipt, a restaurant order ticket. |
| **1 — Plain mechanism** | "What *actually* happens?" | The real behavior in plain words, then the **name** for it (bolded). A small 3–4 box diagram. |
| **2 — Hands on** | "Where do I *see* it?" | Real commands and real code — in `snip`, the project the reader builds, or in real tools on their own machine. A fuller diagram. |
| **3 — What goes wrong** | "How does it *break*?" | Failure modes, gotchas, and a real incident if we have one. |
| **4 — Judgment** | "Why *this* way?" | Trade-offs, alternatives, what the industry does, when you'd choose differently. |

A reader who stops after rung 1 has still learned something true. A reader
who finishes rung 4 can make decisions. Both are wins.

**Example — "port":**

- *Rung 0:* An IP address gets you to the right building; a port gets you to
  the right apartment.
- *Rung 1:* One machine runs many programs. A **port** is a number (1–65535)
  a program claims so the OS knows which program incoming traffic is for.
  Only one program may hold a port at a time.
- *Rung 2:* `snip` claims `8080` (`SNIP_HTTP_ADDR` in
  `snip/internal/config/config.go`); Postgres claims `5432`, Redis `6379`.
  Run `lsof -i -P | grep LISTEN` and see who holds what on your laptop.
- *Rung 3:* "address already in use" = someone else is in that apartment.
  Find them with `lsof -i :8080`.
- *Rung 4:* Why services use fixed well-known ports inside the cluster but
  never expose them directly — the reverse proxy decides what's public.

---

## 3. The ten teaching rules

1. **Anchor before you define.** Open every new idea with something the
   reader already knows. Prefer **everyday** anchors (buildings, receipts,
   phone books, kitchens, post offices) because they work for everyone.
   Where it genuinely helps, add a *second* anchor for coders (e.g. "`$PATH`
   is like `node_modules` lookup") — but the chapter must still make sense
   if the reader skips it.

2. **Behavior first, name second.** Describe what happens, *then* give it
   its name in **bold**: "the running instance of a program is called a
   **process**." Never lead with a term the reader must look up.

3. **Spell out every acronym on first use in each chapter** — `PID (process
   ID)`, `DNS (Domain Name System — the phone book that turns names into
   addresses)`. Add a short gloss, not just the expansion.

4. **One new idea per paragraph.** If a paragraph introduces two unknown
   terms, split it. Sentences carry one step each.

5. **Small picture, then big picture.** First diagram: 3–4 boxes, the whole
   idea at a glance. Then, if needed, the detailed diagram. Never open with
   the 15-box version.

6. **Real over invented.** Examples come from `snip` (the project in
   `snip/`), from real tools (Postgres, Redis, nginx, Docker, Kubernetes,
   AWS), and from real, well-known incidents. Invented `foo`/`bar` examples
   only when nothing real fits. Every code snippet about `snip` must match
   the real file in `snip/` — name the path.

7. **Explain *why* before *how*.** A reader who knows why a rule exists can
   handle the case the rule didn't cover. "Environment variables let the
   *same* built image behave differently in dev and production" comes before
   `export FOO=bar`.

8. **Tell the war stories.** Use real, public incidents (with a source
   link) and the classic mistakes every engineer makes once — what
   happened, why, and the rule it taught ("exit code 0 means *this command
   didn't error*, not *what you wanted is true*"). Never invent an incident
   and present it as real; a hypothetical is labelled "Imagine…".

9. **Be honest.** Say when something is simplified, when it depends on
   version or OS, and when the industry disagrees. Readers make real
   decisions from this handbook.

10. **Respect the reader.** Plain is not childish. No "simply", "just",
    "obviously", "easy" — if it were obvious they wouldn't be reading. No
    filler, no hype, no apologising for complexity; make it clear instead.

---

## 4. Voice & tone

- **Second person, present tense.** "You type a command. The shell finds
  the program." Speak *to* the reader.
- **Warm, confident, a little wry.** A light line is welcome when it
  sharpens a point ("Small-tools philosophy, paying rent."); never jokes for
  their own sake.
- **Short sentences for mechanisms, longer ones for reasoning.**
- **Bold** marks a key term the first time it appears, or the one sentence
  in a section that matters most. Don't bold whole paragraphs.
- *Italics* for emphasis on a single word that changes the meaning ("the
  *shell* expands `*`, not `ls`").
- **Headings make a claim, not a label.**
  ✅ "A server is a program that never exits"
  ❌ "Servers"
  ✅ "Pipes and redirection: composing small tools"
  ❌ "Pipes"
- Prefer concrete numbers and names over vague words: "`429
  insufficient_credit`" beats "an error"; "port `8080`" beats "a port".

---

## 5. Chapter anatomy

Every teaching chapter follows this skeleton. Chapters are **MDX** files at
`docs/<partId>/<chapterId>.mdx` (see `CLAUDE.md` §4). Components `Quiz`,
`SqlPlayground`, `Tabs`/`TabItem` and `Roadmap` are available everywhere
without an import.

````mdx
---
title: "<N.N> <Title that says what you'll understand>"
sidebar_label: "<N.N> <Short title>"
description: <One sentence for search results and link previews.>
---

# <N.N> <Title>

<Hook paragraph, 2–4 sentences, written to "you". Start from where the
reader already is — something they've done, seen, or heard — and say what
this chapter will let them do that they can't today.>

## What you'll learn

- 4–6 bullets. Each one already teaches a sliver ("What a **port** is:
  an apartment number for programs") — not just a topic name.

---

## <Claim-style heading for idea 1>

<Rung 0 anchor → rung 1 mechanism + small diagram → rung 2 hands on.>

:::note[Everyday anchor]
<Optional reinforcement of the analogy in 2–3 sentences.>
:::

---

## <Claim-style heading for idea 2>
...

:::warning[What can go wrong]
- **<Failure in bold.>** Why it happens and how you'd spot it.
:::

---

## Cheat sheet            {/* optional, for command/term-heavy chapters */}

| Command / term | What it does | When you reach for it |
|---|---|---|

---

## Lab

<One line saying where it runs and that it's free: "on your laptop".>

1. **<What you'll see, in bold.>**
   ```bash
   <real commands, each with a trailing # comment>
   ```
   <One or two sentences: what to notice and why it matters.>

2. ...

## Self-check

<Quiz id="<partId>-<chapterId>" questions={[
  {q: '…', options: ['…', '…', '…', '…'], answer: 1, explain: 'Why — including why the tempting wrong answer is wrong.'},
]} />

<details>
<summary>An open question that needs explaining, not picking</summary>

A full answer that explains the *why* and names the rule to keep.

</details>
````

### Section-by-section guidance

- **Title** — numbered to match the part/chapter order in
  `src/data/curriculum.json` (`# 2.1 …`).
- **Hook** — meet the reader in their world: "You've opened a website
  thousands of times…", "You've seen a command in a tutorial and pasted
  it…". Then promise the shift: from copy-pasting to reasoning.
- **Horizontal rules (`---`)** between major sections.
- **Length** — as long as the ladder needs, no longer. Typical: 250–500
  lines. If a chapter passes ~700 lines, it's two chapters.
- **Lab** — 2–4 steps, real commands, **safe and free by default**, runnable
  on the reader's own laptop (Docker, local Kubernetes via `kind`). Anything
  that costs money (cloud) gets a `:::danger[This costs money]` box and a
  teardown step. After each step, tell the reader what to *notice*. Give
  macOS / Linux / Windows (WSL) variants with `<Tabs>` when commands differ.
- **Self-check** — a `<Quiz>` with 3–5 questions **plus** 1–3 `<details>`
  open questions. At least one "what would you check first?" diagnostic and
  one "why is it built this way?" judgment question. Every quiz
  explanation teaches, including why the tempting wrong answer is wrong.
  Quiz `id` is `<partId>-<chapterId>`.
- **Interactive where it helps** — `<SqlPlayground setup="…" sql="…" />`
  in data chapters so readers run real SQL in the browser.

### MDX gotchas (the build will tell you, but save yourself the trip)

- A bare `<` or `{` in prose is parsed as JSX. Put it in backticks
  (`` `a < b` ``) or write "less than". Inside code blocks it's fine.
- Comments are `{/* like this */}`, not `<!-- this -->`.
- Inside HTML/JSX tags (`<summary>`, `<code>`), `{` starts JavaScript even
  though backtick code in normal Markdown is safe. Wrap such text as a JS
  string: `<code>{'${VAR:-default}'}</code>`.
- Inside `<Quiz questions={[…]}>`, strings are JavaScript: use single quotes
  and escape apostrophes (`it\'s`) or use double-quoted strings.
- Leave a blank line inside `<details>` before and after Markdown content.
- Front matter is YAML: wrap `title`/`description` in double quotes if they
  contain `: ` (colon + space), or the whole site build fails.
- Links between chapters are relative file links: `[6.4](../data/indexes-and-performance.mdx)`.

---

## 6. Callouts (admonitions)

Docusaurus admonitions. Keep titles consistent so readers learn what each
box means:

| Syntax | Use for |
|---|---|
| `:::note[Everyday anchor]` | The analogy, restated or extended. |
| `:::info[If you've written code]` | Optional anchor for developers (JS/Python/etc.). Must be skippable. |
| `:::tip` (optionally `[A short claim]`) | A habit worth adopting. |
| `:::warning[What can go wrong]` | Bulleted failure modes, each bold-led. |
| `:::danger` (optionally `[This costs money]`) | Destructive, irreversible, or paid actions. |
| `:::info[In the real world]` | How companies actually do it; industry variation. |

---

## 7. Diagrams

- **Mermaid only** (fenced as ` ```mermaid `), so diagrams live in the text,
  diff cleanly, and follow light/dark mode.
- **Two-step rule:** a tiny `flowchart LR` first (3–4 boxes), then the full
  version if needed. Introduce each: "Four boxes: …" / "Here is the fuller
  picture, because …".
- Label with **plain words plus the real name**: `A["snip API<br/>(Go)"]`.
- Use `<br/>` for line breaks and **quote** labels containing punctuation,
  parentheses, or code.
- After a diagram, name the **one or two things** in it that matter —
  never leave the reader to decode it alone.
- ASCII trees/annotations in ` ```text ` blocks are great for filesystems
  and field-by-field breakdowns (see the `-rw-r--r--` breakdown in 0.5).

---

## 8. Code, commands and tables

- Every command block gets a language (` ```bash `, ` ```go `, ` ```yaml `).
- **Comment every non-obvious line** with a trailing `#` explaining what it
  does *in plain words*.
- Mark trimmed code honestly: `// simplified — the real file adds graceful
  shutdown`, and name the real file path.
- Use generic placeholders for hosts and secrets: `ssh user@server`,
  `$API_KEY`, documentation IPs (`203.0.113.10`). **Never commit
  credentials, real IPs, or personal usernames.**
- Tables for: vocabulary (symbol | means | example), comparisons (v1 vs v2),
  and cheat sheets. Not for prose.

---

## 9. Cross-linking and the glossary

- Link back to where a concept was first taught: "You met processes in
  chapter 0.1". Link forward sparingly: "covered in chapter 0.7".
- Use relative file links: `[6.4 Indexes](../data/indexes-and-performance.mdx)`.
- When you introduce a term that other chapters will use, **add it to
  `src/data/glossary.json`** as `{"term": "…", "def": "one or two plain
  sentences", "doc": "<partId>/<chapterId>"}`. Order doesn't matter; the
  page sorts and links it.
- The sidebar and roadmap come from `src/data/curriculum.json` — never edit them by hand.

---

## 10. Words to avoid → use instead

| Avoid | Why | Instead |
|---|---|---|
| "simply", "just", "obviously", "easy" | Makes stuck readers feel stupid | Delete the word |
| "etc.", "and so on" | Hides what you meant | Name the items, or say "for example" |
| "it", "this" at the start of a paragraph | Ambiguous after a diagram | Repeat the noun |
| Unexplained jargon ("idempotent", "fan-out") | Breaks the ladder | Plain behavior first, then the term in bold |
| "should work" | Readers act on this | Say what *does* happen, verified |
| Passive voice for actions ("the request is sent") | Hides *who* does it | "Envoy sends the request" |

---

## 11. Pre-publish checklist

- [ ] A semi-technical reader can follow the first half; an engineer learns
      something from the second half.
- [ ] Every concept climbs rungs 0 → 4 in order (anchor → mechanism → our
      system → failure → judgment).
- [ ] Every new term is bolded and defined on first use; every acronym is
      expanded.
- [ ] Headings are claims, not labels.
- [ ] Diagrams: small first, explained after.
- [ ] Examples are real: `snip`, real tools, real public incidents.
- [ ] Every `snip` snippet matches the real code in `snip/`, and the code
      builds and passes tests (`cd snip && go test ./...`).
- [ ] Lab steps are safe, labelled, and each says what to notice.
- [ ] Self-check has a diagnostic question and a judgment question, with
      full *why* answers.
- [ ] No credentials, real IPs, or personal hostnames.
- [ ] Chapter is in `src/data/curriculum.json`; new terms in `src/data/glossary.json`.
- [ ] Builds cleanly: `npm run build` (fails on broken links).

---

## 12. Reference chapters to imitate

When in doubt, open these and copy their moves:

- `docs/start/how-this-handbook-works.mdx` — the anatomy and interactive
  components in action.
- `docs/linux/shell-and-filesystem.mdx` — anchors, two-step diagrams,
  cheat sheet, safe lab, strong self-check.
- `docs/foundations/what-a-server-is.mdx` — anchoring a whole field in what
  the reader already knows; tracing a real request through real code.
- `docs/data/transactions.mdx` — a hard idea taught with the ladder all
  the way to rung 4.
