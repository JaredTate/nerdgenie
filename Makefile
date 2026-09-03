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

# build makes the binary and the two worker bundles it starts as child processes.
# internal/browser/process.go and internal/desktop/process.go run
# `node bin/workers/browser/main.js` and `node bin/workers/desktop/main.js`, so
# each worker's compiled dist/ folder is emptied out into exactly that place.
#
# A bundle is more than the compiled JavaScript. Node looks for a package.json
# beside a file to learn that it is a module rather than a script, and it looks
# for node_modules beside it to find playwright-core and the cua driver, so the
# package file, the lock file, and the run-time half of the packages go into the
# bundle too. The test packages are left behind: nothing in bin/ ever runs them.
#
# Neither step is done again when it does not need to be. A worker is compiled
# again only when its dist/main.js is missing or older than the newest file in
# its src/, which is the same staleness rule
# test/functional/browserworker_integration_test.go uses, and its packages are
# fetched again only when the bundle has none or its lock file has changed.
build:
	go build -ldflags "$(LDFLAGS)" -o bin/coeus ./cmd/coeus
	@command -v npm >/dev/null 2>&1 || { \
	  echo "npm is not on the path, and the two TypeScript workers cannot be built without it. Install Node 22 or newer, or put the Node you have on your path."; \
	  exit 1; \
	}
	@for name in browser desktop; do \
	  folder=worker/$$name; \
	  bundle=bin/workers/$$name; \
	  if [ -f $$folder/dist/main.js ] && [ -z "$$(find $$folder/src -newer $$folder/dist/main.js)" ]; then \
	    echo "the $$name worker is newer than its source, so it is not compiled again"; \
	  else \
	    echo "compiling the $$name worker"; \
	    ( cd $$folder && npm ci && npm run build ) || exit 1; \
	  fi; \
	  needsPackages=no; \
	  if [ ! -d $$bundle/node_modules ] || ! cmp -s $$folder/package-lock.json $$bundle/package-lock.json; then \
	    needsPackages=yes; \
	  fi; \
	  mkdir -p $$bundle; \
	  find $$bundle -mindepth 1 -maxdepth 1 ! -name node_modules -exec rm -rf {} +; \
	  cp -R $$folder/dist/. $$bundle/; \
	  cp $$folder/package.json $$folder/package-lock.json $$bundle/; \
	  if [ $$needsPackages = yes ]; then \
	    echo "fetching what the $$name worker needs while it runs"; \
	    ( cd $$bundle && npm ci --omit=dev ) || exit 1; \
	  else \
	    echo "the $$name worker bundle already has the packages its lock file names"; \
	  fi; \
	done
	@echo "the worker bundles are in bin/workers/browser and bin/workers/desktop"

# The browser and desktop integration tests start a real Chrome and drive the
# desktop, which puts windows on the screen of whoever runs them, so they live
# in their own target and an ordinary test run never opens one. The gate and
# continuous integration run both targets; the browser target asks for headless
# Chrome through COEUS_HEADLESS_TESTS, which internal/browser honours.
test:
	go test -tags integration $$(go list ./... | grep -v '/internal/browser$$' | grep -v '/internal/desktop$$' | grep -v '/test/functional$$')
	go test ./test/functional/...
	scripts/fuzz.sh 5s

test-browser:
	COEUS_HEADLESS_TESTS=1 go test -tags integration ./internal/browser/ ./internal/desktop/ ./test/functional/

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
# release depends on build, so the two worker bundles are always compiled by the
# one step above that knows how, and scripts/release/build.sh is left with only
# the work a release adds: a binary per architecture, a pinned Node runtime, a
# worker tree installed for the machine each archive is for, and beside them the
# SHA256SUMS the installer checks against and the manifest.json the updater reads.
release: build
	scripts/release/build.sh $(if $(filter-out dev,$(VERSION)),--version $(VERSION))

install: build
	./bin/coeus install

repo-map:
	go run ./scripts/repomap > REPO_MAP.md

clean:
	rm -rf bin dist coverage
