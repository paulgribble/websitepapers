# syntax=docker/dockerfile:1.7

FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim AS builder

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PROJECT_ENVIRONMENT=/app/.venv

WORKDIR /app

COPY pyproject.toml ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --no-install-project --no-dev

COPY app.py bibtex.py crossref.py db.py doi.py wsgi.py ./
COPY templates ./templates


FROM python:3.12-slim-bookworm

ENV PATH="/app/.venv/bin:$PATH" \
    DB_PATH=/storage/dois.db \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

COPY --from=builder /app /app

RUN mkdir -p /storage \
 && useradd -r -u 1000 -d /app app \
 && chown -R app:app /storage /app

EXPOSE 80
VOLUME ["/storage"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://localhost/up', timeout=2).status == 200 else 1)" || exit 1

CMD ["gunicorn", "--bind", "0.0.0.0:80", "--user", "app", "--group", "app", "--workers", "2", "--access-logfile", "-", "wsgi:app"]
