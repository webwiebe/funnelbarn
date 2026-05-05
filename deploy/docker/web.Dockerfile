FROM node:22-alpine AS build-web

WORKDIR /app

COPY web/package.json web/package-lock.json* ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM node:22-alpine AS build-sdk

WORKDIR /sdk

COPY sdks/js/package.json sdks/js/package-lock.json* ./
RUN npm ci

COPY sdks/js/ ./
RUN npm run build

FROM nginx:alpine

COPY --from=build-web /app/dist /usr/share/nginx/html
COPY --from=build-sdk /sdk/dist/iife/funnelbarn.js /usr/share/nginx/html/sdk.js
COPY deploy/docker/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 3000

CMD ["nginx", "-g", "daemon off;"]
