#!/bin/sh
set -eu

# If Litestream env vars are set, run under Litestream supervision.
# Otherwise, start funnelbarn directly.
if [ -n "${LITESTREAM_ACCESS_KEY_ID:-}" ] && [ -n "${LITESTREAM_SECRET_ACCESS_KEY:-}" ]; then
    exec litestream replicate -config /etc/litestream.yml -exec "funnelbarn"
else
    exec funnelbarn
fi
