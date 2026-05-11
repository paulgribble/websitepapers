.PHONY: install run test clean docker-build docker-run

IMAGE ?= websitepapers
STORAGE_DIR ?= $(PWD)/storage

install:
	uv sync

run:
	uv run python app.py

test:
	uv run pytest

clean:
	rm -rf .venv __pycache__ */__pycache__ .pytest_cache uv.lock

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	mkdir -p $(STORAGE_DIR)
	docker run --rm -p 80:8080 -v $(STORAGE_DIR):/storage --name websitepapers $(IMAGE)
