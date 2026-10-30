# syntax=docker/dockerfile:1
# snip's front door with the web UI inside (chapter 16.6). Two stages, the
# same idea as snip's own Dockerfile (chapter 9.2): build with Node, then copy
# only the finished static files into nginx. No Node in the final image.

# ---- build stage ----
FROM node:26-slim AS build
WORKDIR /web
# Dependencies first, in their own layer: cached until the lockfile changes.
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# ---- runtime stage ----
# nginx's config is not baked in: docker-compose.yml mounts
# snip/deploy/nginx/nginx.conf, so the routing stays in one file you can read.
FROM nginx:1.30-alpine
COPY --from=build /web/dist /usr/share/nginx/html/app
