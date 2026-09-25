# snipai — the AI Engineering track's code

Python code for Part 14 of [Zero to Prod](https://jawwadzafar.github.io/zero-to-prod/).
It adds AI features to **snip**, the handbook's URL shortener (in `../snip`).

Everything here runs **free and offline by default**. Features that call a
language model work with a built-in fake model (for tests and first steps), a
free local model through Ollama, or, optionally, a hosted model with your own
API key.

```bash
cd ai
uv sync            # create .venv and install everything (chapter 14.1)
uv run pytest      # run the tests
uv run ruff check . && uv run mypy   # lint and type-check
```

| Module | What it does | Chapter |
|---|---|---|
| `snipai.client` | A typed client for snip's REST API | 14.1 |
| `snipai.slugs` | snip's slug rules, in Python | 14.1 |
| `snipai.abuse` | A phishing-link classifier, trained from scratch with NumPy | 14.2 |
| `snipai.tinylm` | A BPE tokenizer and an n-gram language model, trained on this handbook | 14.3 |
| `snipai.llm` | One interface to language models: fake (offline), Ollama (local), Anthropic (API) | 14.4 |
| `snipai.pages` | Fetch a page safely (SSRF checks) and extract its text | 14.4 |
| `snipai.describe` | snip's first AI feature: a one-line description of a link's page | 14.4 |
| `snipai.prompt_lab` | Compare prompt versions on fixed pages with automatic checks | 14.5 |
| `snipai.tagger` | Structured output: category, tags and language, validated and retried | 14.6 |
| `snipai.ask` | Tool use: answer questions about your links through snip's API | 14.6 |
| `snipai.embed` | Embeddings, chunking and vector search over this handbook | 14.7 |
| `snipai.rag` | Ask the handbook: retrieval-augmented generation with checked citations | 14.8 |
| `snipai.agent` | An agent that finds broken links and deletes them with your approval, within budgets | 14.9 |
| `snipai.evals` | End-to-end evaluation of RAG: retrieval, citations, LLM-as-judge, abstention, confidence intervals | 14.10 |
| `snipai.bench` | Benchmark a local model: time to first token, tokens per second, throughput | 14.11 |
