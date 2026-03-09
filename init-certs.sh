#!/bin/sh
# ── init-certs.sh ─────────────────────────────────────────────────────────────
# Run ONCE on a fresh server to obtain the initial Let's Encrypt certificate.
# After this script succeeds, certbot auto-renews every 12h inside Docker.
#
# Prerequisites:
#   - DNS A record for fluentica.app pointing to this server's IP
#   - DNS A record for www.fluentica.app pointing to this server's IP
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
WWW_DOMAIN="www.fluentica.app"

echo "==> Requesting certificate for $DOMAIN and $WWW_DOMAIN"
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
docker compose run --rm certbot certbot certonly \
  --webroot \
  --webroot-path /var/www/certbot \
  --email "$EMAIL" \
  --agree-tos \
  --no-eff-email \
  -d "$DOMAIN" \
  -d "$WWW_DOMAIN"

echo "==> Certificate issued successfully."

# Step 3: Download recommended TLS parameters from Let's Encrypt if not present
if [ ! -f "$(docker volume inspect ailanguagetutor_certbot_certs --format '{{.Mountpoint}}')/options-ssl-nginx.conf" ] 2>/dev/null; then
  docker compose run --rm certbot sh -c "
    curl -s https://raw.githubusercontent.com/certbot/certbot/master/certbot-nginx/certbot_nginx/_internal/tls_configs/options-ssl-nginx.conf \
      -o /etc/letsencrypt/options-ssl-nginx.conf &&
    openssl dhparam -out /etc/letsencrypt/ssl-dhparams.pem 2048
  "
fi

# Step 4: Restore full HTTPS nginx config and remove bootstrap config
mv nginx/conf.d/fluentica.conf.bak nginx/conf.d/fluentica.conf 2>/dev/null || true
rm -f nginx/conf.d/fluentica-init.conf

# Step 5: Reload nginx with full HTTPS config
docker compose up -d --force-recreate nginx

echo ""
echo "==> Done! Fluentica is live at https://$DOMAIN"
echo "    Certificates auto-renew every 12h via the certbot container."
