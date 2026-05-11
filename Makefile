.PHONY: install run test clean docker-build docker-run docker-pull docker-run-ghcr

IMAGE ?= websitepapers
GHCR_IMAGE ?= ghcr.io/paulgribble/websitepapers:latest
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
	docker run --rm -p 80:80 -v $(STORAGE_DIR):/storage --name websitepapers $(IMAGE)

docker-pull:
	docker pull $(GHCR_IMAGE)

docker-run-ghcr:
	mkdir -p $(STORAGE_DIR)
	docker run --rm -p 80:80 -v $(STORAGE_DIR):/storage --name websitepapers $(GHCR_IMAGE)
