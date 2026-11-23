#!/bin/sh
# Runs the back-end checks in Docker, so no local Go installation is needed.
# CI (.github/workflows/ci.yml) calls the same script, so a green local run
# means a green CI run.
#
#   scripts/check.sh            the back-end checks, in this order:
#   scripts/check.sh fmt        gofmt
#   scripts/check.sh vet        go vet
#   scripts/check.sh lint       golangci-lint (.golangci.yml)
#   scripts/check.sh test       go test -race
#   scripts/check.sh smoke      build-site against example-instance/
#   scripts/check.sh vuln       govulncheck
#
#   scripts/check.sh workflows  actionlint and zizmor on the workflows
#   scripts/check.sh image      build the image, scan it and the
#                               Containerfile with Trivy
#
#   scripts/check.sh bench      run the benchmarks on BENCH_BASE (default
#                               origin/main) and on the working tree,
#                               compare them with benchstat; never fails,
#                               warns about slowdowns above BENCH_THRESHOLD
#                               percent (default 15)
#
# Works from any directory, in Git Bash on Windows as well. Set GH_TOKEN to
# let zizmor run its online audits too. With CHECK_SARIF_DIR set (CI),
# workflows and image also write SARIF reports there for code scanning.
set -eu

# Tool versions. The Go version comes from backend/go.mod. Dependabot does
# not see these; update them by hand.
GOLANGCI_LINT_VERSION=v2.15.0
GOVULNCHECK_VERSION=v1.10.0
ACTIONLINT_VERSION=1.7.12
ZIZMOR_VERSION=1.33.1
TRIVY_VERSION=0.76.2
BENCHSTAT_VERSION=v0.0.0-20261117185222-4febdb082b2c # golang.org/x/perf has no tags

# Image tag for the local scan; CI's image.yml scans its own build.
CHECK_IMAGE=${CHECK_IMAGE:-openlitfass:check}

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
		-w "${GO_WORKDIR:-/src/backend}" \
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

workflow_files() {
	echo .github/workflows/*.yml example-instance/.github/workflows/*.yml
}

check_workflows() {
	# shellcheck disable=SC2046 # word splitting of the file list is wanted
	docker run --rm -v "$host_repo:/src" -w /src \
		"rhysd/actionlint:$ACTIONLINT_VERSION" -color $(cd "$repo" && workflow_files)
	# Without a token zizmor skips the audits that query GitHub.
	if [ -n "${GH_TOKEN:-}" ]; then
		set -- -e GH_TOKEN "ghcr.io/zizmorcore/zizmor:$ZIZMOR_VERSION"
	else
		set -- "ghcr.io/zizmorcore/zizmor:$ZIZMOR_VERSION" --offline
	fi
	if [ -n "${CHECK_SARIF_DIR:-}" ]; then
		docker run --rm -v "$host_repo:/src" -w /src "$@" \
			--min-severity low --format sarif .github example-instance/.github \
			>"$CHECK_SARIF_DIR/zizmor.sarif" || true
	fi
	docker run --rm -v "$host_repo:/src" -w /src "$@" \
		--min-severity low .github example-instance/.github
}

trivy() {
	if [ -n "${CHECK_SARIF_DIR:-}" ]; then
		set -- -v "$CHECK_SARIF_DIR:/out" "aquasec/trivy:$TRIVY_VERSION" "$@"
	else
		set -- "aquasec/trivy:$TRIVY_VERSION" "$@"
	fi
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$host_repo:/src:ro" \
		-v ol-trivycache:/root/.cache/trivy \
		"$@"
}

check_image() {
	docker build -q -t "$CHECK_IMAGE" -f "$host_repo/Containerfile" "$host_repo" >/dev/null
	if [ -n "${CHECK_SARIF_DIR:-}" ]; then
		trivy image --scanners vuln --ignore-unfixed --format sarif --output /out/trivy-image.sarif "$CHECK_IMAGE"
	fi
	# Only what can be fixed by an update; unfixed CVEs would block forever.
	trivy image --scanners vuln --severity CRITICAL,HIGH --ignore-unfixed --exit-code 1 "$CHECK_IMAGE"
	trivy config --severity CRITICAL,HIGH --exit-code 1 /src/Containerfile
}

check_bench() {
	base=${BENCH_BASE:-origin/main}
	count=${BENCH_COUNT:-10}
	threshold=${BENCH_THRESHOLD:-15}
	work="$repo/.bench" # gitignored; results stay for a look afterwards
	rm -rf "$work"
	mkdir -p "$work/base"
	(cd "$repo" && git archive "$base" backend web) | tar -x -C "$work/base"

	bench="go test -run '^\$' -bench . -count $count ./..."
	echo "base: $base"
	GO_WORKDIR=/src/.bench/base/backend go_run sh -c "$bench > /src/.bench/old.txt"
	echo "head: working tree"
	go_run sh -c "$bench > /src/.bench/new.txt"
	go_run go run "golang.org/x/perf/cmd/benchstat@$BENCHSTAT_VERSION" \
		base=/src/.bench/old.txt head=/src/.bench/new.txt >"$work/benchstat.txt"
	cat "$work/benchstat.txt"

	# Slower by more than the threshold in the time table, and significant
	# (benchstat prints "~" instead of a delta when it is not).
	prefix="warning:"
	if [ "${GITHUB_ACTIONS:-}" = true ]; then prefix="::warning::"; fi
	awk -v t="$threshold" -v p="$prefix" '
		/sec\/op/ { s = 1; next }
		/B\/op|allocs\/op/ { s = 0 }
		s && match($0, /\+[0-9.]+% \(p=/) {
			d = substr($0, RSTART + 1, RLENGTH - 1) + 0
			if (d > t) { print p " benchmark slower by more than " t "%: " $1 " (+" d "%)" }
		}' "$work/benchstat.txt"
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
	fmt | vet | lint | test | smoke | vuln | workflows | image | bench) run "$step" ;;
	*)
		echo "unknown check: $step (fmt, vet, lint, test, smoke, vuln, workflows, image, bench)" >&2
		exit 2
		;;
	esac
done
echo "==> all checks passed"
