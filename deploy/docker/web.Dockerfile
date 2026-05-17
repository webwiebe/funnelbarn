FROM node:22-alpine AS build-sdk

WORKDIR /workspace/sdks/js

COPY sdks/js/package.json sdks/js/package-lock.json* ./
RUN npm ci

COPY sdks/js/ ./
RUN npm run build

FROM node:22-alpine AS build-web

# Workspace root so that file:../sdks/js resolves correctly.
WORKDIR /workspace

# Copy the built SDK so the web package can resolve @funnelbarn/js.
COPY --from=build-sdk /workspace/sdks/js ./sdks/js

COPY web/package.json web/package-lock.json* ./web/
RUN cd web && npm ci

COPY web/ ./web/
RUN cd web && npm run build

FROM nginx:alpine

COPY --from=build-web /workspace/web/dist /usr/share/nginx/html
COPY --from=build-sdk /workspace/sdks/js/dist/iife/funnelbarn.js /usr/share/nginx/html/sdk.js
COPY deploy/docker/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 3000

CMD ["nginx", "-g", "daemon off;"]
