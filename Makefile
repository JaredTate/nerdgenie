# Coeus build targets. Every target in docs/WORK_PLAN.md Part 1 is here, and the
# same table is in CLAUDE.md. `make check` is the gate: a wave does not pass until
# it is clean, and continuous integration runs it on every push.

# Go lives in /usr/local/go on the development machine and staticcheck installs
# itself into the Go bin folder, so put both on the path rather than asking every
# reader to change their shell profile.
export PATH := $(PATH):/usr/local/go/bin:$(HOME)/go/bin

# The version string the `version` subcommand prints. A release sets it from the
# tag; a development build says "dev".
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

.PHONY: all build test fuzz check live release install repo-map clean

all: check

build:
	go build -ldflags "$(LDFLAGS)" -o bin/coeus ./cmd/coeus
	scripts/release/workers.sh

test:
	go test -tags integration ./...
	go test ./test/functional/...
	scripts/fuzz.sh 5s

fuzz:
	scripts/fuzz.sh 60s

check:
	@echo "== gofmt =="
	scripts/gofmt.sh
	@echo "== go vet =="
	go vet ./...
	@echo "== staticcheck =="
	staticcheck ./...
	@echo "== style checker =="
	go run ./scripts/stylecheck ./...
	@echo "== repository map drift =="
	go test ./scripts/repomap/...
	@echo "== coverage =="
	scripts/coverage.sh
	@echo "== tests =="
	@$(MAKE) --no-print-directory test

# The live suite needs the development machine: the llama-server daemon on port
# 19091, an Anthropic key, and an OpenAI key. A missing key is a failure, never a
# skip. Until wave 1 there are no live tests and the target passes trivially.
live:
	go test -tags live ./test/functional/... ./internal/...

# One release: bin/coeus for linux/amd64 and linux/arm64, each packed with the two
# worker bundles and a pinned Node runtime so that nobody has to install Node, and
# beside them a SHA256SUMS the installer checks against and a manifest.json the
# updater reads. The version comes from git describe unless VERSION says otherwise.
release:
	scripts/release/build.sh $(if $(filter-out dev,$(VERSION)),--version $(VERSION))

install:
	@echo "make install is built in wave 3, brief 3.3. It is not done yet."
	@exit 1

repo-map:
	go run ./scripts/repomap > REPO_MAP.md

clean:
	rm -rf bin dist coverage
