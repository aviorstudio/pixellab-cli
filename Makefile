.PHONY: build install test

VERSION ?= dev
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || printf none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
export VERSION COMMIT DATE

build:
	cargo build --release --locked
	mkdir -p bin
	cp "$${CARGO_TARGET_DIR:-target}/release/pxlb" bin/pxlb

install:
	cargo install --path . --locked

test:
	cargo test --locked
	cargo build --locked
	python3 tests/cli_test.py
