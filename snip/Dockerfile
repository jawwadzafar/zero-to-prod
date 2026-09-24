# syntax=docker/dockerfile:1
# Multi-stage build (chapter 9.2): compile in a big image with the Go
# toolchain, then copy only the finished binaries into a tiny runtime image.

# ---- build stage ----
# --platform=$BUILDPLATFORM runs this stage on the builder's own CPU type, and
# Go cross-compiles for the target (chapter 9.4): an arm64 image builds fast on
# an amd64 machine, with no emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
# Download dependencies first, in their own layer, so they're cached until
# go.mod/go.sum change — code edits don't re-download the world.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 makes fully static binaries that run on an empty base image.
# -trimpath and -ldflags "-s -w" drop local paths and debug symbols (smaller).
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/snip ./cmd/snip && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/snip-worker ./cmd/snip-worker

# ---- runtime stage ----
# distroless/static: no shell, no package manager, just CA certificates and
# timezone data. Less software inside = less to attack (chapter 9.4).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/snip /out/snip-worker /usr/local/bin/
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/snip"]
