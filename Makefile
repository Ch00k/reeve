BINARY := reeve
BIN := bin/$(BINARY)
DIST_DIR := dist
INSTALL_DIR := $(HOME)/.local/bin
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
GO_BUILD := CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"

.PHONY: build test lint install dist clean release release-patch release-minor release-major

build:
	$(GO_BUILD) -o $(BIN) .

test:
	go test ./...

lint:
	golangci-lint run --fix

install: build
	install -m 0755 $(BIN) $(INSTALL_DIR)/$(BINARY)

# Release binaries for every platform, plus their checksums
dist:
	rm -rf $(DIST_DIR)
	for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		GOOS=$$os GOARCH=$$arch $(GO_BUILD) -o $(DIST_DIR)/$(BINARY)-$$os-$$arch . || exit 1; \
	done
	cd $(DIST_DIR) && shasum -a 256 $(BINARY)-* > SHA256SUMS

clean:
	rm -rf bin $(DIST_DIR)

release:
	@echo "Available release types:"
	@echo "  make release-patch  # Patch version (x.y.Z)"
	@echo "  make release-minor  # Minor version (x.Y.0)"
	@echo "  make release-major  # Major version (X.0.0)"

release-patch:
	./release.sh patch

release-minor:
	./release.sh minor

release-major:
	./release.sh major
