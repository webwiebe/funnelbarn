#!/bin/bash
set -e

if command -v systemctl > /dev/null 2>&1; then
    systemctl stop funnelbarn.service || true
    systemctl disable funnelbarn.service || true
    systemctl daemon-reload || true
fi
