#!/bin/bash
set -e

# Create system user and group if they don't exist.
if ! getent group funnelbarn > /dev/null 2>&1; then
    groupadd --system funnelbarn
fi
if ! getent passwd funnelbarn > /dev/null 2>&1; then
    useradd --system --gid funnelbarn --no-create-home \
        --home-dir /var/lib/funnelbarn \
        --shell /usr/sbin/nologin \
        --comment "FunnelBarn service account" funnelbarn
fi

# Ensure state directories exist with correct ownership.
install -d -o funnelbarn -g funnelbarn -m 0750 /var/lib/funnelbarn
install -d -o funnelbarn -g funnelbarn -m 0750 /var/lib/funnelbarn/spool

# Ensure config directory exists.
install -d -m 0755 /etc/funnelbarn

# Drop a sample config if no config exists yet.
if [ ! -f /etc/funnelbarn/funnelbarn.conf ]; then
    cp /etc/funnelbarn/funnelbarn.conf.example /etc/funnelbarn/funnelbarn.conf
    chmod 0640 /etc/funnelbarn/funnelbarn.conf
    chown root:funnelbarn /etc/funnelbarn/funnelbarn.conf
    echo "FunnelBarn: sample config installed at /etc/funnelbarn/funnelbarn.conf — edit before starting."
fi

# Reload systemd and enable the service.
if command -v systemctl > /dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable funnelbarn.service || true
    echo "FunnelBarn: service enabled. Start with: sudo systemctl start funnelbarn"
fi
