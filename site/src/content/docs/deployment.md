---
title: Deployment Guide
description: Deploy FunnelBarn with Docker, Homebrew, APT, or a systemd service.
order: 10
---

# Deployment Guide

FunnelBarn is a single Go binary with no external runtime dependencies. All state lives in a SQLite file. Choose the deployment method that fits your infrastructure.

## Docker (recommended)

Docker is the simplest way to deploy and update FunnelBarn in production.

### Quick start

```bash
docker run -d \
  --name funnelbarn \
  --restart unless-stopped \
  -e FUNNELBARN_API_KEY=your-long-random-api-key \
  -e FUNNELBARN_ADMIN_USERNAME=admin \
  -e FUNNELBARN_ADMIN_PASSWORD=changeme \
  -e FUNNELBARN_SESSION_SECRET=$(openssl rand -hex 32) \
  -e FUNNELBARN_ALLOWED_ORIGINS=https://yoursite.example.com \
  -p 8080:8080 \
  -v funnelbarn-data:/var/lib/funnelbarn \
  ghcr.io/webwiebe/funnelbarn/service:latest
```

The `-v funnelbarn-data:/var/lib/funnelbarn` flag mounts a named Docker volume for the SQLite database and spool directory. Data persists across container restarts and upgrades.

### Docker Compose

```yaml
# docker-compose.yml
services:
  funnelbarn:
    image: ghcr.io/webwiebe/funnelbarn/service:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      FUNNELBARN_API_KEY: ${FUNNELBARN_API_KEY}
      FUNNELBARN_ADMIN_USERNAME: admin
      FUNNELBARN_ADMIN_PASSWORD: ${FUNNELBARN_ADMIN_PASSWORD}
      FUNNELBARN_SESSION_SECRET: ${FUNNELBARN_SESSION_SECRET}
      FUNNELBARN_ALLOWED_ORIGINS: https://yoursite.example.com
    volumes:
      - funnelbarn-data:/var/lib/funnelbarn

volumes:
  funnelbarn-data:
```

Store secrets in a `.env` file (not committed to version control):

```
FUNNELBARN_API_KEY=a-long-random-secret
FUNNELBARN_ADMIN_PASSWORD=another-secret
FUNNELBARN_SESSION_SECRET=yet-another-secret
```

Start with:

```bash
docker compose up -d
```

### Updating

```bash
docker compose pull
docker compose up -d
```

---

## Homebrew (macOS)

```bash
brew tap webwiebe/funnelbarn
brew install funnelbarn
```

Configure via a config file or environment variables, then start:

```bash
# Create config directory
mkdir -p ~/.config/funnelbarn

# Write config
cat > ~/.config/funnelbarn/funnelbarn.conf << 'EOF'
FUNNELBARN_API_KEY=mysecret
FUNNELBARN_ADMIN_USERNAME=admin
FUNNELBARN_ADMIN_PASSWORD=changeme
FUNNELBARN_SESSION_SECRET=another-secret
EOF

# Start
funnelbarn
```

To run as a background service:

```bash
brew services start funnelbarn
```

---

## APT (Debian / Ubuntu)

```bash
curl -fsSL https://webwiebe.nl/apt/install.sh | sudo bash
sudo apt install funnelbarn
```

The APT package installs:
- The binary to `/usr/bin/funnelbarn`
- A systemd unit at `/lib/systemd/system/funnelbarn.service`
- A config directory at `/etc/funnelbarn/`

Edit the config file before starting:

```bash
sudo nano /etc/funnelbarn/funnelbarn.conf
```

Minimum required configuration:

```ini
FUNNELBARN_API_KEY=your-long-random-api-key
FUNNELBARN_ADMIN_USERNAME=admin
FUNNELBARN_ADMIN_PASSWORD=changeme
FUNNELBARN_SESSION_SECRET=another-long-random-secret
```

---

## systemd service

If you are not using the APT package, you can create a systemd unit manually.

### 1. Create a dedicated user

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin funnelbarn
sudo mkdir -p /var/lib/funnelbarn /etc/funnelbarn
sudo chown funnelbarn:funnelbarn /var/lib/funnelbarn
```

### 2. Install the binary

Download the latest release from GitHub and place it at `/usr/local/bin/funnelbarn`:

```bash
curl -L https://github.com/webwiebe/funnelbarn/releases/latest/download/funnelbarn-linux-amd64 \
  -o /usr/local/bin/funnelbarn
chmod +x /usr/local/bin/funnelbarn
```

### 3. Write the config

```bash
sudo tee /etc/funnelbarn/funnelbarn.conf << 'EOF'
FUNNELBARN_ADDR=:8080
FUNNELBARN_DB_PATH=/var/lib/funnelbarn/funnelbarn.db
FUNNELBARN_SPOOL_DIR=/var/lib/funnelbarn/spool
FUNNELBARN_API_KEY=your-long-random-api-key
FUNNELBARN_ADMIN_USERNAME=admin
FUNNELBARN_ADMIN_PASSWORD_BCRYPT=$2b$12$...
FUNNELBARN_SESSION_SECRET=another-long-random-secret
FUNNELBARN_ALLOWED_ORIGINS=https://yoursite.example.com
EOF
sudo chmod 600 /etc/funnelbarn/funnelbarn.conf
sudo chown root:funnelbarn /etc/funnelbarn/funnelbarn.conf
```

### 4. Create the systemd unit

```ini
# /etc/systemd/system/funnelbarn.service
[Unit]
Description=FunnelBarn analytics server
After=network.target

[Service]
Type=simple
User=funnelbarn
Group=funnelbarn
ExecStart=/usr/local/bin/funnelbarn
EnvironmentFile=/etc/funnelbarn/funnelbarn.conf
Restart=on-failure
RestartSec=5s
WorkingDirectory=/var/lib/funnelbarn

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/funnelbarn
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

### 5. Start and enable

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now funnelbarn
sudo systemctl status funnelbarn
```

### 6. View logs

```bash
sudo journalctl -u funnelbarn -f
```

---

## Reverse proxy (nginx)

Place FunnelBarn behind nginx to terminate TLS and serve on port 443:

```nginx
server {
    listen 443 ssl http2;
    server_name funnelbarn.example.com;

    ssl_certificate     /etc/letsencrypt/live/funnelbarn.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/funnelbarn.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## Backups

Back up the SQLite database file regularly. The default path is `.data/funnelbarn.db` (relative to the working directory) or `/var/lib/funnelbarn/funnelbarn.db` on Linux installs.

```bash
# Simple file copy (safe while FunnelBarn is running — SQLite WAL mode)
cp /var/lib/funnelbarn/funnelbarn.db /backups/funnelbarn-$(date +%Y%m%d).db
```

For zero-downtime online backup:

```bash
sqlite3 /var/lib/funnelbarn/funnelbarn.db ".backup /backups/funnelbarn-$(date +%Y%m%d).db"
```
