# openLitfass runtime image: the compiled tools (fetch, build-site, serve)
# plus the core frontend, nothing else. An instance's workflow runs this
# image against its own checked-out instance directory - see
# docs/instance.md.
#
# Build context is the repository root:
#   docker build -t openlitfass -f Containerfile .
# CI: .github/workflows/image.yml (multi-arch amd64/arm64, ghcr.io).

# The build stage always runs on the build machine's own architecture and
# cross-compiles for the target (Go needs no emulation for that) - much
# faster than compiling under QEMU for arm64.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/cmd/ cmd/
COPY backend/internal/ internal/
# CGO_ENABLED=0: fully static binaries, no libc dependency.
# -trimpath -ldflags="-s -w": drop local build paths and debug symbols.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X openlitfass/backend/internal/app.Version=$VERSION" \
      -o /out/ ./cmd/fetch ./cmd/build-site ./cmd/serve

# Runtime stage. Deliberately alpine, not scratch: CI "run:" steps and
# `docker run … sh -c` need a shell. git so a workflow can commit updated
# data from inside the container; nodejs so JavaScript actions (e.g.
# actions/checkout) work in Forgejo container jobs.
#
# No tzdata package needed: internal/model embeds the IANA timezone database
# at compile time (blank `time/tzdata` import).
FROM alpine:3.20
RUN apk add --no-cache ca-certificates git nodejs
COPY --from=build /out/fetch /out/build-site /out/serve /usr/local/bin/
COPY web/ /opt/openlitfass/web/
ENV OL_WEB_DIR=/opt/openlitfass/web
LABEL org.opencontainers.image.title="openLitfass" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.description="Die digitale Litfaßsäule für deinen Kiez: fetch + build-site"

# Default: run one instance self-contained - mount the instance directory
# at /instance; the site is served on 8080 and refreshed every 6 hours
# (OL_FETCH_INTERVAL). CI workflows call `fetch` / `build-site` explicitly.
WORKDIR /instance
ENV OL_FETCH_INTERVAL=6h
EXPOSE 8080
# Optional: OL_METRICS_ADDR=:9090 serves /metrics on its own port (not public).
# Optional SEO extras, off by default (see docs/instance.md):
# OL_STATIC_PAGES=true (termine/, pre-rendered start page),
# OL_SITEMAP_GENERATOR=true (sitemap.xml, robots.txt).
ENV OL_STATIC_PAGES=false OL_SITEMAP_GENERATOR=false
HEALTHCHECK --interval=1m --timeout=5s --start-period=30s \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
CMD ["serve", "/instance"]
