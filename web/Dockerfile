# syntax=docker/dockerfile:1
# snip's web UI in an nginx image (chapter 16.6). Two stages, the same idea as
# snip's own Dockerfile (chapter 9.2): build with Node, then copy only the
# finished static files into nginx. No Node in the final image.

# ---- build stage ----
# The output is plain files that run in any browser, so build on the machine's
# own platform even when the image is for another (chapter 9.4's arm64 build).
FROM --platform=$BUILDPLATFORM node:26-slim AS build
WORKDIR /web
# Dependencies first, in their own layer: cached until the lockfile changes.
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# ---- runtime stage ----
# nginx's own unprivileged image: it runs as user 101, not root, and keeps its
# pid and temporary files in /tmp, so the rest of the filesystem can be
# read-only (chapter 9.4).
FROM nginxinc/nginx-unprivileged:1.31-alpine
# How /app/ is served, shared by every front door (nginx/app.conf), and a
# server that serves only the UI (nginx/default.conf, used on Kubernetes).
# The compose stack mounts snip/deploy/nginx/nginx.conf over default.conf to
# add the proxy to snip's API; it includes the same app.conf.
COPY nginx/app.conf /etc/nginx/snippets/app.conf
COPY nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html/app
USER 101
EXPOSE 8080
