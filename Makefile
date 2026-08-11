.PHONY: help build install test race cover badge golden screenshots lint vuln release-check snapshot fmt clean

BADGE   := docs/coverage.svg
PROFILE := coverage.out

help: ## Show this help
	@grep -hE '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/' | expand -t 12

build: ## Build the lunette binary
	go build -o lunette .

install: ## Install lunette into GOBIN
	go install .

test: ## Run the test suite
	go test ./...

race: ## Run the test suite under the race detector
	go test -race ./...

cover: ## Run tests with coverage and print the total
	go test -coverprofile=$(PROFILE) ./...
	go tool cover -func=$(PROFILE) | tail -1

golden: ## Rewrite the golden TUI frames after an intended layout change
	go test ./internal/tui/ -run TestGoldenFrames -update
	@echo "review the diff before committing: git diff internal/tui/testdata/golden"

screenshots: ## Regenerate the README screenshots
	./scripts/screenshots.sh

badge: cover ## Regenerate docs/coverage.svg from the coverage profile
	./scripts/coverage-badge.sh $(PROFILE) $(BADGE)

lint: ## Vet and check formatting
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "unformatted files:"; gofmt -l .; exit 1; }

release-check: ## Validate the release configuration
	goreleaser check

snapshot: ## Build the release archives locally, publishing nothing
	goreleaser release --snapshot --clean --skip=publish,validate
	@ls -1 dist/*.tar.gz dist/checksums.txt

vuln: ## Report known vulnerabilities that this code can actually reach
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmt: ## Format the Go sources
	gofmt -w .

clean: ## Remove build and coverage artefacts
	rm -rf lunette lunette-*-* $(PROFILE) dist
