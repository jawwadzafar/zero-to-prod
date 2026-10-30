# The analytics consumer as an image (chapter 17.6). Two stages, like snip's own
# Dockerfile (chapter 9.2): uv builds a virtual environment, and only that
# environment is copied onto a slim Python image.
FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim AS build
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_PYTHON_DOWNLOADS=0
WORKDIR /app
COPY pyproject.toml uv.lock README.md ./
RUN uv sync --locked --no-dev --no-install-project      # dependencies first: cached until uv.lock changes
COPY src ./src
RUN uv sync --locked --no-dev --no-editable             # then the package itself

FROM python:3.12-slim-bookworm
COPY --from=build /app/.venv /app/.venv
ENV PATH=/app/.venv/bin:$PATH PYTHONUNBUFFERED=1 LAKE_DIR=/lake
# Not root. snip's compose file runs it as your own user ID, so it can write your lake folder.
USER 65532:65532
ENTRYPOINT ["python", "-m", "clickstream"]
