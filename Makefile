BIN_DIR := bin
TEST_HOME := $(PWD)/.autodev-home

.PHONY: build build-runtime test local-up local-down sample-run build-stage-images guard-deprecated-invariants install-test-config require-config-source

install-test-config: require-config-source
	rm -rf $(TEST_HOME)
	mkdir -p $(TEST_HOME)/.autodev/config
	cp -R $(CONFIG_SOURCE)/. $(TEST_HOME)/.autodev/config/

require-config-source:
	@test -n "$(CONFIG_SOURCE)" || (echo "CONFIG_SOURCE must be set to PROD or TEST" >&2; exit 1)
	@case "$(CONFIG_SOURCE)" in PROD|TEST) ;; *) echo "CONFIG_SOURCE must be PROD or TEST" >&2; exit 1;; esac

guard-deprecated-invariants:
	@MARKER_A=DEPRECIATED; MARKER_B=' INVARIANT'; MARKER="$$MARKER_A$$MARKER_B"; \
	! find . \( -path ./.git -o -path ./.git/\* \) -prune -o -print | grep -n "$$MARKER"
	@MARKER_A=DEPRECIATED; MARKER_B=' INVARIANT'; MARKER="$$MARKER_A$$MARKER_B"; \
	! rg -n --hidden --glob '!**/.git/**' "$$MARKER" .

build: guard-deprecated-invariants require-config-source install-test-config build-stage-images build-runtime
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/control-plane ./cmd/control-plane
	go build -o $(BIN_DIR)/stage-runner ./cmd/stage-runner

build-runtime: guard-deprecated-invariants
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/autodev-stage-runtime ./tooling/runtime

test: guard-deprecated-invariants require-config-source install-test-config build-runtime build-stage-images
	HOME=$(TEST_HOME) go test ./...

local-up: require-config-source
	docker compose up --build

local-down:
	docker compose down -v

sample-run: build
	./$(BIN_DIR)/stage-runner --config autodev.config.json local --issue hack/sample-issue.json

meta-validate: build build-stage-images
	./$(BIN_DIR)/stage-runner --config autodev.meta.json materialize --issue hack/e2e-pipeline-issue.json
	./$(BIN_DIR)/stage-runner --config autodev.meta.json local --issue hack/e2e-pipeline-issue.json --smoke-secrets hack/smoke-secrets.json

build-stage-images: guard-deprecated-invariants install-test-config require-config-source
	python3 tools/build_stage_images.py --config-source $(CONFIG_SOURCE)

build-cognition:
	go build -o tooling/cognition/bin/cognition cmd/cognition/main.go
build-all:
	go build -o tooling/cognition/bin/cognition cmd/cognition/main.go
