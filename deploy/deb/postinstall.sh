#!/bin/bash
set -e

# Create system user and group if they don't exist.
if ! getent group trailpost > /dev/null 2>&1; then
    groupadd --system trailpost
fi
if ! getent passwd trailpost > /dev/null 2>&1; then
    useradd --system --gid trailpost --no-create-home \
        --home-dir /var/lib/trailpost \
        --shell /usr/sbin/nologin \
        --comment "Trailpost service account" trailpost
fi

# Ensure state directories exist with correct ownership.
install -d -o trailpost -g trailpost -m 0750 /var/lib/trailpost
install -d -o trailpost -g trailpost -m 0750 /var/lib/trailpost/spool

# Ensure config directory exists.
install -d -m 0755 /etc/trailpost

# Drop a sample config if no config exists yet.
if [ ! -f /etc/trailpost/trailpost.conf ]; then
    cp /etc/trailpost/trailpost.conf.example /etc/trailpost/trailpost.conf
    chmod 0640 /etc/trailpost/trailpost.conf
    chown root:trailpost /etc/trailpost/trailpost.conf
    echo "Trailpost: sample config installed at /etc/trailpost/trailpost.conf — edit before starting."
fi

# Reload systemd and enable the service.
if command -v systemctl > /dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable trailpost.service || true
    echo "Trailpost: service enabled. Start with: sudo systemctl start trailpost"
fi
