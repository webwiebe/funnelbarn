FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY sdks/go ./sdks/go

RUN CGO_ENABLED=1 go build -o /out/funnelbarn ./cmd/funnelbarn

FROM alpine:3.20 AS litestream
ARG LITESTREAM_VERSION=0.3.13
RUN apk add --no-cache ca-certificates wget && \
    wget -qO /tmp/litestream.tar.gz \
      "https://github.com/benbjohnson/litestream/releases/download/v${LITESTREAM_VERSION}/litestream-v${LITESTREAM_VERSION}-linux-amd64.tar.gz" && \
    tar -C /usr/local/bin -xzf /tmp/litestream.tar.gz litestream

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates sqlite
RUN addgroup -S funnelbarn && adduser -S funnelbarn -G funnelbarn

COPY --from=build /out/funnelbarn /usr/local/bin/funnelbarn
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY deploy/docker/litestream.yml /etc/litestream.yml
COPY deploy/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER funnelbarn

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
