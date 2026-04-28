#!/bin/bash
set -e

if command -v systemctl > /dev/null 2>&1; then
    systemctl stop trailpost.service || true
    systemctl disable trailpost.service || true
    systemctl daemon-reload || true
fi
