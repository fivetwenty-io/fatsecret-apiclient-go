.PHONY: fmt vet staticcheck lint govulncheck gosec test test-race coverage \
        generate verify-generated check ci

# Hand-authored Go files, excluding generated pkg/api/* packages and *.pb.go.
GO_HAND_AUTHORED := $(shell find . -name '*.go' \
	-not -path './pkg/api/*' \
	-not -name '*.pb.go')

# Hand-authored packages for the coverage threshold: exclude the generated
# pkg/api/* service packages, the runnable examples, and the test-only mock.
COVER_PKGS := $(shell go list ./... | grep -vE 'pkg/api/|/examples|pkg/client/mock')

fmt:
	gofmt -l -w $(GO_HAND_AUTHORED)

vet:
	go vet ./...

staticcheck:
	staticcheck ./...

lint:
	golangci-lint run ./...

govulncheck:
	govulncheck ./...

gosec:
	gosec -exclude-generated ./...

test:
	go test ./...

test-race:
	go test -race ./...

# coverage: compute statement coverage over hand-authored packages only
# (generated pkg/api/*, examples, and the test-only mock are excluded via
# COVER_PKGS so the profile total reflects hand-authored code). Threshold: 80%.
coverage:
	go test -coverprofile=coverage.out $(COVER_PKGS)
	@go tool cover -func=coverage.out | \
	  awk '/total:/{total=$$3} END { \
	    gsub(/%/, "", total); \
	    if (total+0 < 80) { \
	      print "FAIL: coverage " total "% is below 80% threshold (hand-authored code)"; \
	      exit 1 \
	    } else { \
	      print "OK: coverage " total "% (hand-authored code)" \
	    } \
	  }'

generate:
	go run ./cmd/fsgen --spec spec/fatsecret.yaml --out .

# verify-generated: regenerate into a temp dir and diff against committed output.
# Fails on any drift; exits 0 only when generated files are up to date.
verify-generated:
	@TMPDIR=$$(mktemp -d) && \
	  go run ./cmd/fsgen --spec spec/fatsecret.yaml --out $$TMPDIR && \
	  diff -rq --exclude='*.go.bak' --exclude='doc.go' $$TMPDIR/pkg/api ./pkg/api && \
	  diff -q $$TMPDIR/pkg/compatibility/matrix.go ./pkg/compatibility/matrix.go && \
	  rm -rf $$TMPDIR && \
	  echo "OK: generated output is up to date." || \
	  (rm -rf $$TMPDIR; echo "ERROR: Run 'make generate' and commit the result."; exit 1)

check: fmt vet staticcheck lint test

ci: check test-race coverage govulncheck gosec verify-generated
