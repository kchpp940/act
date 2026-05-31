PREFIX ?= /usr/local
RELEASE_JSON := release.json
SCENARIO ?= release
VERSION_FILE := $(shell jq -r '.version_file' $(RELEASE_JSON) 2>/dev/null || echo VERSION)
VERSION ?= $(shell cat $(VERSION_FILE) 2>/dev/null)
GIT_VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null | sed -e 's/^v//')
ifeq ($(VERSION),)
VERSION := $(GIT_VERSION)
endif
IS_SNAPSHOT = $(if $(findstring -, $(VERSION)),true,false)
MAJOR_VERSION = $(word 1, $(subst ., ,$(VERSION)))
MINOR_VERSION = $(word 2, $(subst ., ,$(VERSION)))
PATCH_VERSION = $(word 3, $(subst ., $(word 1,$(subst -, , $(VERSION)))))
NEW_VERSION ?= $(MAJOR_VERSION).$(MINOR_VERSION).$(shell echo $$(( $(PATCH_VERSION) + 1)) )
LDFLAGS ?= $(shell jq -r '.build.ldflags' $(RELEASE_JSON) 2>/dev/null | sed 's/{{ .Version }}/$(VERSION)/')
GOVULNCHECK_PACKAGE ?= golang.org/x/vuln/cmd/govulncheck@v1

fix = false
ifeq (true,$(fix))
	FIX = --fix
endif

ACT ?= go run main.go

HAS_TOKEN = $(if $(test -e ~/.config/github/token),true,false)
ifeq (true,$(HAS_TOKEN))
	export GITHUB_TOKEN := $(shell cat ~/.config/github/token)
endif

.PHONY: pr
pr: tidy format-all lint release-check test

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o dist/local/act main.go

.PHONY: format
format:
	go fmt ./...

.PHONY: format-all
format-all:
	go fmt ./...
	npx prettier --write .

.PHONY: test
test:
	go test ./...
	$(ACT)

.PHONY: lint-go
lint-go:
	golangci-lint run $(FIX)

.PHONY: lint-js
lint-js:
	npx standard $(FIX)

.PHONY: lint-md
lint-md:
	npx markdownlint . $(FIX)

.PHONY: lint-rest
lint-rest:
	docker run --rm -it \
		-v $(PWD):/tmp/lint \
		-e GITHUB_STATUS_REPORTER=false \
		-e GITHUB_COMMENT_REPORTER=false \
		megalinter/megalinter-go:v5

.PHONY: lint
lint: lint-go lint-rest

.PHONY: lint-fix
lint-fix: lint-md lint-go

.PHONY: fix
fix:
	make lint-fix fix=true

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: install
install: build
	@cp dist/local/act $(PREFIX)/bin/act
	@chmod 755 $(PREFIX)/bin/act
	@act --version

.PHONY: installer
installer:
	@GO111MODULE=off go get github.com/goreleaser/godownloader
	godownloader -r nektos/act -o install.sh

.PHONY: promote
promote:
	@git fetch --tags
	@echo "VERSION:$(VERSION) IS_SNAPSHOT:$(IS_SNAPSHOT) NEW_VERSION:$(NEW_VERSION)"
ifeq (false,$(IS_SNAPSHOT))
	@echo "Unable to promote a non-snapshot"
	@exit 1
endif
ifneq ($(shell git status -s),)
	@echo "Unable to promote a dirty workspace"
	@exit 1
endif
	echo -n $(NEW_VERSION) > VERSION
	git add VERSION
	git commit -m "chore: bump VERSION to $(NEW_VERSION)"
	git tag -a -m "releasing v$(NEW_VERSION)" v$(NEW_VERSION)
	git push origin master
	git push origin v$(NEW_VERSION)

.PHONY: snapshot
snapshot:
	goreleaser build \
		--clean \
		--single-target \
		--snapshot

.PHONY: clean all

.PHONY: release-check
release-check:
	@CHECK_ALL_SCENARIOS=true ./scripts/release_check.sh check

.PHONY: release-gen
release-gen:
	@echo "Regenerating release artifacts from release.json (scenario: $(SCENARIO))..."
	@SCENARIO=$(SCENARIO) ./scripts/release_gen.sh goreleaser .goreleaser.yml
	@echo "  Generated .goreleaser.yml"
	@./scripts/release_gen.sh install-platforms > /tmp/act_platforms.sh && \
		./scripts/release_gen.sh install-adjust-format > /tmp/act_format.sh && \
		./scripts/release_gen.sh install-adjust-os > /tmp/act_adjust_os.sh && \
		./scripts/release_gen.sh install-adjust-arch > /tmp/act_adjust_arch.sh && \
		python3 scripts/replace_section.py install.sh get_binaries /tmp/act_platforms.sh && \
		python3 scripts/replace_section.py install.sh adjust_format /tmp/act_format.sh && \
		python3 scripts/replace_section.py install.sh adjust_os /tmp/act_adjust_os.sh && \
		python3 scripts/replace_section.py install.sh adjust_arch /tmp/act_adjust_arch.sh && \
		rm -f /tmp/act_platforms.sh /tmp/act_format.sh /tmp/act_adjust_os.sh /tmp/act_adjust_arch.sh
	@echo "  Generated install.sh (platform-dependent functions)"
	@./scripts/release_gen.sh readme-snippet /tmp/readme_snippet.md && \
		python3 scripts/replace_readme_section.py README.md /tmp/readme_snippet.md && \
		rm -f /tmp/readme_snippet.md
	@echo "  Generated README.md (installation section)"
	@echo "Done. Run 'make release-check' to verify."

.PHONY: upgrade
upgrade:
	go get -u
	go mod tidy

# "$(shell go env GOROOT)/bin/go" allows us to use an outdated global go tool and use the build toolchain defined by the project
# go build auto upgrades to the same toolchain version as defined in the go.mod file
.PHONY: deps-tools
deps-tools: ## install tool dependencies
	"$(shell go env GOROOT)/bin/go" install $(GOVULNCHECK_PACKAGE)

.PHONY: security-check
security-check: deps-tools
	GOEXPERIMENT= "$(shell go env GOROOT)/bin/go" run $(GOVULNCHECK_PACKAGE) -show color ./...
