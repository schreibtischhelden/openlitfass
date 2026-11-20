# openlitfass runtime image: the two compiled tools (fetch,
# build-site) plus the core frontend, nothing else. An instance's workflow
# runs this image against its own checked-out instance directory - see
# docs/instance.md.
#
# Build context is the repository root:
#   docker build -t openlitfass -f Containerfile .
# (CI builds with buildah, not docker - see .forgejo/workflows/build-image.yml.)

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY backend/go.mod ./
COPY backend/cmd/ cmd/
COPY backend/internal/ internal/
# CGO_ENABLED=0: fully static binaries, no libc dependency.
# -trimpath -ldflags="-s -w": drop local build paths and debug symbols.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/fetch ./cmd/build-site

# Runtime stage. Deliberately alpine, not scratch: Forgejo Actions "run:"
# steps are executed as `sh -c "<command>"` inside the job's container, so
# the image needs *some* shell. git is included so the instance workflow can
# commit the updated data from inside this same container, and nodejs
# because actions/checkout is a JavaScript action that runs inside the job
# container.
#
# No tzdata package needed: internal/model embeds the IANA timezone database
# at compile time (blank `time/tzdata` import).
FROM alpine:3.20
RUN apk add --no-cache ca-certificates git nodejs
COPY --from=build /out/fetch /out/build-site /usr/local/bin/
COPY web/ /opt/openlitfass/web/
ENV WEB_DIR=/opt/openlitfass/web
