#!/bin/sh
# Runs the back-end checks in Docker, so no local Go installation is needed.
# CI (.github/workflows/ci.yml) calls the same script, so a green local run
# means a green CI run.
#
#   scripts/check.sh            all checks below, in this order
#   scripts/check.sh fmt        gofmt
#   scripts/check.sh vet        go vet
#   scripts/check.sh lint       golangci-lint (.golangci.yml)
#   scripts/check.sh test       go test -race
#   scripts/check.sh smoke      build-site against example-instance/
#   scripts/check.sh vuln       govulncheck
#
# Works from any directory, in Git Bash on Windows as well.
set -eu

# Tool versions. The Go version comes from backend/go.mod.
GOLANGCI_LINT_VERSION=v2.15.0
GOVULNCHECK_VERSION=v1.10.0

# Keep Git Bash from rewriting container paths such as /src.
export MSYS_NO_PATHCONV=1

repo=$(cd "$(dirname "$0")/.." && pwd)
# Docker Desktop on Windows needs C:/... rather than /c/...
host_repo=$(cd "$repo" && { pwd -W 2>/dev/null || pwd; })

go_version=$(sed -n 's/^go \([0-9.]*\).*/\1/p' "$repo/backend/go.mod")
# Debian rather than Alpine: the race detector needs cgo and gcc.
go_image="golang:$go_version"

go_run() {
	docker run --rm \
		-v "$host_repo:/src" \
		-v ol-gocache:/root/.cache/go-build \
		-v ol-gomod:/go/pkg/mod \
		-w /src/backend \
		"$go_image" "$@"
}

check_fmt() {
	go_run sh -c 'out=$(gofmt -l .); if [ -n "$out" ]; then echo "Not gofmt-formatted:"; echo "$out"; exit 1; fi'
}

check_vet() {
	go_run go vet ./...
}

check_lint() {
	docker run --rm \
		-v "$host_repo:/src" \
		-v ol-gomod:/go/pkg/mod \
		-v ol-lintcache:/root/.cache \
		-w /src/backend \
		"golangci/golangci-lint:$GOLANGCI_LINT_VERSION" \
		golangci-lint run --config ../.golangci.yml ./...
}

check_test() {
	go_run go test -race ./...
}

check_smoke() {
	go_run sh -c 'go run ./cmd/build-site -web ../web -out /tmp/site ../example-instance && test -f /tmp/site/index.html'
}

check_vuln() {
	go_run go run "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION" ./...
}

run() {
	echo "==> $1"
	"check_$1"
}

if [ $# -eq 0 ]; then
	set -- fmt vet lint test smoke vuln
fi

for step in "$@"; do
	case $step in
	fmt | vet | lint | test | smoke | vuln) run "$step" ;;
	*)
		echo "unknown check: $step (fmt, vet, lint, test, smoke, vuln)" >&2
		exit 2
		;;
	esac
done
echo "==> all checks passed"
