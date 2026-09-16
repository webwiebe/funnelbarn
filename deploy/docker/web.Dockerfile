FROM node:22-alpine AS build

# corepack picks the pnpm version from root package.json's "packageManager"
# field. Disable its download prompt since this runs non-interactively.
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable

WORKDIR /workspace

# Copy the workspace manifests and every member's package.json first, so
# `pnpm install` can resolve the workspace graph and be cached separately
# from the source. All workspace members must be present here or
# --frozen-lockfile fails, even for members the web build does not need.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY web/package.json ./web/
COPY sdks/js/package.json ./sdks/js/
COPY site/package.json ./site/
COPY tools/replay/package.json ./tools/replay/

# Only fetch/link what @funnelbarn/web needs and its dependencies (the
# trailing "..." pulls in @funnelbarn/js so it builds first).
RUN pnpm install --frozen-lockfile --filter "@funnelbarn/web..."

COPY sdks/js/ ./sdks/js/
COPY web/ ./web/
RUN pnpm --filter "@funnelbarn/web..." build

FROM nginx:alpine

COPY --from=build /workspace/web/dist /usr/share/nginx/html
COPY --from=build /workspace/sdks/js/dist/iife/funnelbarn.js /usr/share/nginx/html/sdk.js
COPY deploy/docker/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 3000

CMD ["nginx", "-g", "daemon off;"]
