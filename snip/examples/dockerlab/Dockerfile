# syntax=docker/dockerfile:1
# The improved Dockerfile (chapter 9.2): multi-stage, cache-friendly, non-root.

# ---- build stage: has the Go toolchain ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
# (With dependencies you'd COPY go.sum too and RUN go mod download here,
# so they're cached in their own layer. This program has none.)
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server .

# ---- runtime stage: only the finished program ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/server"]
