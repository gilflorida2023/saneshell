VERSION := $(shell cat VERSION 2>/dev/null || echo "0.1.0")
PROTOCOL_VERSION := 1
GO := $(HOME)/.go/bin/go
GRADLEW := $(shell pwd)/intel/gradlew

# Detect architecture
ARCH := $(shell uname -m)
ifeq ($(ARCH),x86_64)
  GOARCH := amd64
  GRAAL_ARCH := amd64
else ifeq ($(ARCH),aarch64)
  GOARCH := arm64
  GRAAL_ARCH := aarch64
else
  $(error Unsupported arch: $(ARCH))
endif

.PHONY: all build build-go build-java test test-go test-java \
        package install clean distclean version-check update-readme \
        help tag

all: build

# ── Help ──────────────────────────────────────────────────
help:
	@echo "saneshell build system"
	@echo ""
	@echo "Targets:"
	@echo "  make build          — build Go + Java (GraalVM native)"
	@echo "  make build-go       — build Go binary only"
	@echo "  make build-java     — build Java daemon only"
	@echo "  make test           — run all tests"
	@echo "  make version-check  — validate versions between components"
	@echo "  make update-readme  — inject VERSION into README.md"
	@echo "  make package        — build .deb package"
	@echo "  make install        — install to /usr/local"
	@echo "  make clean          — remove build artifacts"
	@echo "  make tag            — git tag v$(VERSION)"

# ── Version Validation ────────────────────────────────────
version-check:
	@echo "Checking version consistency..."
	@if [ "$(VERSION)" = "" ]; then echo "VERSION file empty"; exit 1; fi
	@grep -q "ProtocolVersion = $(PROTOCOL_VERSION)" internal/ipc/protocol.go || \
		(echo "Protocol mismatch: Go has ProtocolVersion != $(PROTOCOL_VERSION) in internal/ipc/protocol.go"; exit 1)
	@if [ -f README.md ]; then \
		grep -q "> **Version:** \`$(VERSION)\`" README.md || \
		echo "  warning: README.md version may be stale (run 'make update-readme')"; \
	fi
	@echo "  version:  $(VERSION)"
	@echo "  protocol: $(PROTOCOL_VERSION)"
	@echo "  arch:     $(ARCH)"
	@echo "  ✓ versions consistent"

# ── Go Build ──────────────────────────────────────────────
build-go: version-check
	@mkdir -p dist
	CGO_ENABLED=0 GOARCH=$(GOARCH) $(GO) build \
		-ldflags="-s -w -X main.version=$(VERSION) -X main.protocolVersion=$(PROTOCOL_VERSION)" \
		-o dist/saneshell ./cmd/saneshell
	@echo "  ✓ dist/saneshell $(VERSION) ($(GOARCH))"

# ── Java Build ────────────────────────────────────────────
ifeq ($(shell command -v java 2>/dev/null),)
build-java:
	@echo "  warning: Java not found, skipping intel daemon"
else
build-java: version-check
	@mkdir -p dist
	cd intel && $(GRADLEW) --no-daemon -Pversion=$(VERSION) -PprotocolVersion=$(PROTOCOL_VERSION) nativeCompile 2>/dev/null || \
		$(GRADLEW) --no-daemon -Pversion=$(VERSION) -PprotocolVersion=$(PROTOCOL_VERSION) shadowJar 2>/dev/null || true
	@if [ -f intel/build/native/nativeCompile/saneshell-intel ]; then \
		cp intel/build/native/nativeCompile/saneshell-intel dist/; \
		echo "  ✓ dist/saneshell-intel (native)"; \
	elif [ -f intel/build/libs/saneshell-intel-all.jar ]; then \
		cp intel/build/libs/saneshell-intel-all.jar dist/; \
		echo "  ✓ dist/saneshell-intel.jar"; \
	else \
		echo "  warning: intel daemon build skipped"; \
	fi
endif

# ── Combined Build ────────────────────────────────────────
build: build-go build-java
	@echo '{"core":"$(VERSION)","intel":"$(VERSION)","protocol":$(PROTOCOL_VERSION)}' > dist/version.json
	@echo "  ✓ build complete: dist/"

# ── Testing ──────────────────────────────────────────────
test-go:
	$(GO) test -v ./...

test-java:
	@if [ -f intel/build.gradle.kts ]; then \
		cd intel && $(GRADLEW) --no-daemon test 2>/dev/null || true; \
	fi

test: test-go test-java

# ── README Version Injection ─────────────────────────────
update-readme: VERSION
	@if [ -f README.md ]; then \
		sed -i 's/> **Version:** `.*`/> **Version:** `$(VERSION)`/' README.md; \
		echo "  ✓ README.md updated to $(VERSION)"; \
	else \
		echo "  warning: README.md not found"; \
	fi

# ── Packaging ────────────────────────────────────────────
package: build
	@if [ -d debian ]; then \
		cp dist/saneshell debian/saneshell/usr/bin/ 2>/dev/null || true; \
		cp dist/saneshell-intel debian/saneshell/usr/lib/saneshell/ 2>/dev/null || true; \
		cp dist/version.json debian/saneshell/usr/lib/saneshell/ 2>/dev/null || true; \
		cd debian && dpkg-buildpackage -us -uc -b 2>/dev/null || \
			echo "  warning: dpkg-buildpackage failed (missing build deps?)"; \
	else \
		echo "  warning: debian/ directory not found"; \
	fi

# ── Install ──────────────────────────────────────────────
install: build
	install -Dm755 dist/saneshell /usr/local/bin/saneshell
	@if [ -f dist/saneshell-intel ]; then \
		install -Dm755 dist/saneshell-intel /usr/local/lib/saneshell/saneshell-intel; \
	fi
	install -Dm644 dist/version.json /usr/local/lib/saneshell/version.json
	@if [ -f debian/saneshell.service ]; then \
		install -Dm644 debian/saneshell.service /etc/systemd/user/saneshell-intel.service; \
	fi
	@echo "  ✓ installed to /usr/local"

# ── Clean ────────────────────────────────────────────────
clean:
	rm -rf dist
	$(GO) clean
	@if [ -f intel/build.gradle.kts ]; then \
		cd intel && $(GRADLEW) --no-daemon clean 2>/dev/null || true; \
	fi

distclean: clean
	rm -f VERSION

# ── Version ──────────────────────────────────────────────
version:
	@echo "$(VERSION)"

# ── Git Tag ──────────────────────────────────────────────
tag:
	@if git rev-parse --git-dir >/dev/null 2>&1; then \
		git tag -a v$(VERSION) -m "Release v$(VERSION)"; \
		echo "  ✓ tagged v$(VERSION)"; \
	else \
		echo "  warning: not a git repository"; \
	fi
