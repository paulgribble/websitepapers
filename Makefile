.PHONY: install run test clean

install:
	uv sync

run:
	uv run python app.py

test:
	uv run pytest

clean:
	rm -rf .venv __pycache__ */__pycache__ .pytest_cache uv.lock
