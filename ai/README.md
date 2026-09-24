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
