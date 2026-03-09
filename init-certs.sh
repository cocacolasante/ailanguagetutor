#!/bin/sh
# ── init-certs.sh ─────────────────────────────────────────────────────────────
# Run ONCE on a fresh server to obtain the initial Let's Encrypt certificate.
# After this script succeeds, certbot auto-renews every 12h inside Docker.
#
# Prerequisites:
#   - DNS A record for fluentica.app pointing to this server's IP
#   - Port 80 open in firewall
#   - .env file with EMAIL_FROM set (used as Let's Encrypt contact email)
#
# Usage:
#   chmod +x init-certs.sh
#   ./init-certs.sh your@email.com
# ------------------------------------------------------------------------------

set -e

EMAIL="${1:-admin@fluentica.app}"
DOMAIN="fluentica.app"

echo "==> Requesting certificate for $DOMAIN"
echo "    Contact email: $EMAIL"

# Step 1: Start nginx with the bootstrap (HTTP-only) config so the ACME
#         challenge endpoint is reachable before SSL certs exist.
cp nginx/conf.d/fluentica-init.conf.disabled nginx/conf.d/fluentica-init.conf
# Temporarily disable the HTTPS config so nginx won't error on missing certs
mv nginx/conf.d/fluentica.conf nginx/conf.d/fluentica.conf.bak 2>/dev/null || true

docker compose up -d nginx

echo "==> Nginx started in bootstrap mode — waiting 3s..."
sleep 3

# Step 2: Issue certificate via certbot webroot challenge
# Note: --entrypoint overrides the renewal-loop entrypoint defined in compose
docker compose run --rm --entrypoint certbot certbot certonly \
  --webroot \
  --webroot-path /var/www/certbot \
  --email "$EMAIL" \
  --agree-tos \
  --no-eff-email \
  -d "$DOMAIN"

echo "==> Certificate issued successfully."

# Step 3: Download recommended TLS parameters from Let's Encrypt if not present
CERT_MOUNT="$(docker volume inspect ailanguagetutor_certbot_certs --format '{{.Mountpoint}}' 2>/dev/null)"
if [ ! -f "${CERT_MOUNT}/options-ssl-nginx.conf" ]; then
  docker compose run --rm --entrypoint sh certbot -c "
    wget -qO /etc/letsencrypt/options-ssl-nginx.conf \
      https://raw.githubusercontent.com/certbot/certbot/master/certbot-nginx/certbot_nginx/_internal/tls_configs/options-ssl-nginx.conf &&
    openssl dhparam -out /etc/letsencrypt/ssl-dhparams.pem 2048
  "
fi

# Step 4: Restore full HTTPS nginx config and remove bootstrap config
mv nginx/conf.d/fluentica.conf.bak nginx/conf.d/fluentica.conf 2>/dev/null || true
rm -f nginx/conf.d/fluentica-init.conf

# Step 5: Reload nginx with full HTTPS config and start certbot renewal daemon
docker compose up -d --force-recreate nginx certbot

echo ""
echo "==> Done! Fluentica is live at https://$DOMAIN"
echo "    Certificates auto-renew every 12h via the certbot container."
