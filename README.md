<div align="center">

<img src="static/img/logo.svg" width="72" alt="Zero to Prod logo" />

# Zero to Prod

**Learn backend, DevOps and AI engineering from first principles — by building and shipping one real system.**

Free. Open source. Interactive. No sign-up.

[**Start learning →**](https://jawwadzafar.github.io/zero-to-prod/) · [Roadmap](https://jawwadzafar.github.io/zero-to-prod/roadmap) · [How we write](STYLE.md)

</div>

---

## What this is

A handbook that takes you from **basic tech literacy** — you use computers, maybe you've pasted a command once — to **job-ready engineer**. Every idea climbs the same five-rung ladder:

| Rung | Question | Example: a *port* |
|---|---|---|
| 0 · Anchor | What is it like? | An apartment number for programs |
| 1 · Mechanism | What actually happens? | A program claims a number; the OS routes traffic to it |
| 2 · Hands on | Where do I see it? | Your snip server claims `:8080` — watch it with `lsof` |
| 3 · What breaks | How does it go wrong? | "address already in use", and how to fix it |
| 4 · Judgment | Why this way? | Why real systems hide ports behind a proxy |

You build **snip**, a link shortener, and grow it into a production-shaped system: Postgres, Redis, a queue and worker, Docker, Kubernetes, CI/CD, AWS with Terraform, metrics and tracing — and AI features in the AI track.

## Tracks

| Track | Parts |
|---|---|
| 🧱 Foundations — computers, networks, the web, Linux, Git | 0–3 |
| ⚙️ Backend Engineer — Go, APIs, data, security, distributed systems | 4–8, 13, 15 |
| 🚀 DevOps / Platform Engineer — containers, Kubernetes, CI/CD, cloud, IaC, reliability | 2, 8–13, 15 |
| 🧠 AI Engineer — LLMs, prompting, tool use, embeddings, RAG, agents, evals, LLMOps | 5–6, 14, 15 |

## Interactive, with no backend

Everything runs in your browser — the site is static and hosted free on GitHub Pages:

- 🗺️ **Roadmap graph** with tracks and your progress (saved in `localStorage`, exportable)
- 🧩 **Quizzes** that explain every answer
- 🐘 **A real PostgreSQL** in the page (PGlite / WebAssembly) for the SQL chapters
- 💬 **Ask the handbook**: instant retrieval over every section; optional AI answers with citations using your own API key (sent only from your browser to the provider)

## Run it locally

```bash
npm install
npm start          # http://localhost:3000/zero-to-prod/
npm run build      # production build into ./build (fails on broken links)
npm run status     # which chapters are written
```

The snip project:

```bash
cd snip
go test ./...
```

## Contributing

Found a confusing sentence, a missing step, or a mistake? That's a bug — please [open an issue](https://github.com/jawwadzafar/zero-to-prod/issues/new) or a pull request. Read [STYLE.md](STYLE.md) first; it's short and it's the whole philosophy. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Text: [CC BY-SA 4.0](LICENSE-CONTENT.md). Code (site, `snip/`, `ai/`): [MIT](LICENSE).
