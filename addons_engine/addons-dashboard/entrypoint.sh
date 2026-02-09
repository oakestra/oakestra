#!/bin/sh

# Build full URLs from components
export MARKETPLACE_FULL_URL="${MARKETPLACE_PROTOCOL}://${MARKETPLACE_URL}:${MARKETPLACE_PORT}"
export ADDONS_ENGINE_FULL_URL="${ADDONS_ENGINE_PROTOCOL}://${ADDONS_ENGINE_URL}:${ADDONS_ENGINE_PORT}"
export RESOURCE_ABSTRACTOR_FULL_URL="${RESOURCE_ABSTRACTOR_PROTOCOL}://${RESOURCE_ABSTRACTOR_URL}:${RESOURCE_ABSTRACTOR_PORT}"

# Replace environment variables in config.json
envsubst < /usr/share/nginx/html/assets/config.template.json > /usr/share/nginx/html/assets/config.json

# Update nginx to listen on DASHBOARD_PORT
sed -i "s/listen 80;/listen ${DASHBOARD_PORT};/g" /etc/nginx/conf.d/default.conf

# Start nginx
exec nginx -g 'daemon off;'
