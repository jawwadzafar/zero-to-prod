# CLAUDE.md — Zero to Prod

This file is the complete brief for anyone (human or AI) working on this repo.
Read it fully before doing anything. **Then read `STYLE.md` before writing any
chapter.** The owner should never have to repeat what is written here.

---

## 1. Vision (what the owner asked for, verbatim intent)

- **A free, open, public learning platform** that takes a **semi-technical
  person** (basic tech literacy — uses computers, maybe pasted a command once)
  **from zero to 100%**: job-ready engineer.
- It must be **the best place on the internet to learn this** — better than
  paid platforms — through clarity (the ladder in `STYLE.md`), one real
  project (`snip`), and interactivity.
- **Scope grows by tracks, all starting from the same base.** Today:
  Foundations → Backend, DevOps/Platform, AI Engineering. The structure must
  make it easy to add more tracks later (e.g. Frontend, Data, Security, Mobile,
  ML) by editing `src/data/curriculum.json`.
- **Pure static site, no backend.** Everything interactive runs in the
  browser: progress (localStorage), quizzes, a real Postgres (PGlite/WASM),
  roadmap graph, and "Ask the handbook" (client-side retrieval + optional
  bring-your-own-API-key AI answers). Deployed free on **GitHub Pages** via
  GitHub Actions.
- **Industry-standard tooling:** Docusaurus 3 (MDX + React + TypeScript),
  local search, Mermaid diagrams, CI on every PR, automatic deploy on `main`.
- **Autonomy:** work in a continuous loop. Research, decide, fix, write,
  verify, commit, push. Don't stop to ask about things this file or
  `STYLE.md` already answers. Find and fix problems wherever you see them.

## 2. Hard rules

- **Git authorship:** every commit is authored `Jawwad Zafar
  <zafarjawwad@gmail.com>`. **Never** add `Co-Authored-By`, session links,
  "Generated with …", or any mention of Claude/AI in commits, PRs, issues,
  or comments.
- **Push constantly.** Commit and push to `main` after every chapter or
  meaningful unit of work. Never leave work only on a local disk.
- **Never break the build.** Before every push: `npm run typecheck && npm run
  build` (site) and, if `snip/` changed, `cd snip && go vet ./... && go test
  ./...`. The build fails on broken links — that's intentional.
- **No secrets, real IPs, or personal hostnames** anywhere. Use placeholders
  (`user@server`, `203.0.113.10`, `$API_KEY`).
- **Every code snippet about snip must match real code in `snip/`** and that
  code must build and pass tests.
- **Honesty:** no invented incidents presented as real, no fake statistics.
  Real public incidents get a source link.

## 3. Repo map

```text
.
├── CLAUDE.md                 ← this brief
├── STYLE.md                  ← how to write chapters (the ladder, voice, anatomy, MDX syntax)
├── README.md                 ← public front page of the repo
├── docusaurus.config.ts      ← site config (baseUrl /zero-to-prod/, GitHub Pages)
├── sidebars.ts               ← generated from curriculum.json (only written chapters appear)
├── src/
│   ├── data/curriculum.json  ← SINGLE SOURCE OF TRUTH: tracks, parts, chapters, order, minutes
│   ├── lib/progress.ts       ← localStorage progress store (done chapters, quiz scores, track)
│   ├── lib/curriculum.ts     ← helpers over curriculum.json
│   ├── components/
│   │   ├── Roadmap/          ← roadmap graph + track picker + progress summary
│   │   ├── Quiz/             ← <Quiz> multiple choice with explanations
│   │   ├── SqlPlayground/    ← <SqlPlayground> real Postgres in the browser (PGlite)
│   │   ├── AskPanel/         ← floating "Ask the handbook" (retrieval + BYOK Claude)
│   │   └── ChapterComplete/  ← "Mark complete" + next-in-track, under every chapter
│   ├── theme/                ← Root (mounts AskPanel), MDXComponents (global components),
│   │                           DocItem/Footer (mounts ChapterComplete)
│   └── pages/                ← home (index.tsx), /roadmap, /progress
├── plugins/curriculum.js     ← exposes which chapters exist (for "coming soon")
├── scripts/
│   ├── build-ask-index.mjs   ← builds static/ask-index.json (runs before start/build)
│   ├── status.mjs            ← prints written/total chapters per part
│   └── make-social-card.py   ← regenerates static/img/social-card.png
├── docs/                     ← the handbook: docs/<partId>/<chapterId>.mdx
├── snip/                     ← the Go project readers build (reference implementation)
├── ai/                       ← Python code for the AI Engineering track
└── .github/workflows/        ← deploy.yml (Pages), ci.yml (site + snip)
```

## 4. How to add or write a chapter

1. The chapter must exist in `src/data/curriculum.json` (part id + chapter id,
   title, minutes, optional `tracks` override). Add it there if new.
2. Create `docs/<partId>/<chapterId>.mdx` with front matter:
   ```yaml
   ---
   title: "6.2 SQL from zero"
   sidebar_label: "6.2 SQL from zero"
   description: One sentence for search engines and link previews.
   ---
   ```
   It appears in the sidebar and roadmap automatically.
3. Follow `STYLE.md` exactly: the ladder, the anatomy, `<Quiz>` + `<details>`
   self-check, a Lab that is free and runs locally.
4. Add new terms to `src/data/glossary.json` (`{"term", "def", "doc": "<partId>/<chapterId>"}`); the Glossary page and Ask index pick them up.
5. Link only to chapters that exist (the build fails otherwise). When a later
   chapter is written, go back and add forward links if useful.
6. Verify: `npm run build`. Commit (`docs: add 6.2 SQL from zero`) and push.
7. Run `node scripts/status.mjs` to see what's left.

## 5. The snip project (what readers build)

A **URL shortener** in **Go**, grown chapter by chapter. The reference
implementation lives in `snip/` and is the source of every snip snippet.

- HTTP API with Go's standard library (`net/http`, method-based routing).
- `POST /api/links` create · `GET /{slug}` redirect (302) ·
  `GET /api/links/{slug}` stats · `DELETE /api/links/{slug}` ·
  `GET /healthz` · `GET /readyz` · `GET /metrics`.
- Storage behind an interface: in-memory (early chapters) → **Postgres**
  (pgx, embedded SQL migrations).
- **Redis**: read-through cache for redirects, fixed-window rate limiting,
  and a **Redis Streams** queue for click events consumed by a **worker**.
- API keys (hashed) for authentication; structured logging (`log/slog`);
  config from environment variables; graceful shutdown; Prometheus metrics.
- Packaging and ops: multi-stage `Dockerfile`, `docker-compose.yml`
  (snip, worker, postgres, redis, nginx, prometheus, grafana), Kubernetes
  manifests, a Helm chart, Terraform for AWS, GitHub Actions CI.
- AI track features (Python in `ai/`): link summaries/tags via an LLM, semantic
  search over saved links (embeddings + RAG), evals.

## 6. Curriculum (source of truth: `src/data/curriculum.json`)

16 parts, 94 chapters: 0 Start Here · 1 Computers & the Internet · 2 Command
Line & Linux · 3 Git · 4 Go · 5 APIs · 6 Data · 7 Security · 8 Distributed
Systems · 9 Containers · 10 Kubernetes · 11 Shipping · 12 Cloud & IaC ·
13 Observability · 14 AI Engineering · 15 Architecture & Career.

Order of work: write parts in order (0 → 15), so every chapter can link back
to what came before. Build the matching `snip/` code alongside the chapter
that introduces it.

## 7. Deploy

- Push to `main` → `.github/workflows/deploy.yml` builds and publishes to
  `https://jawwadzafar.github.io/zero-to-prod/`.
- One-time repo setting (owner): **Settings → Pages → Source: GitHub
  Actions**. The repo must be public for free Pages.

## 8. Backlog (ideas to build when chapters are in good shape)

- Semantic retrieval for Ask (embeddings built at deploy time, queried in the
  browser with transformers.js) and an optional fully in-browser model
  (WebLLM) so AI answers work with no key at all.
- Spaced-repetition review of past quiz questions on the progress page.
- In-browser terminal/Linux sandbox for Part 2 labs (e.g. WebVM/v86).
- Interactive diagrams (step-through request lifecycle), Go playground embeds.
- Printable/PDF edition; i18n (translations) via Docusaurus i18n.
- More tracks: Frontend, Data Engineering, Security, Mobile.
