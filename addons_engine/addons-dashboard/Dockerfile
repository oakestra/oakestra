# Stage 1: Build the Angular application
FROM node:20-alpine AS build

WORKDIR /app

# Copy package files
COPY package.json package-lock.json* ./

# Install dependencies
RUN npm install

# Copy source code
COPY . .

# Build the application
RUN npm run build:prod

# Copy config template to build output
RUN cp src/assets/config.template.json dist/oakestra-addons-ui/assets/config.template.json || true

# Stage 2: Serve with nginx
FROM nginx:alpine

# Install envsubst (gettext package)
RUN apk add --no-cache gettext

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy built application from build stage
COPY --from=build /app/dist/oakestra-addons-ui /usr/share/nginx/html

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Environment variables with defaults
# Marketplace configuration
ENV MARKETPLACE_PROTOCOL=http
ENV MARKETPLACE_URL=localhost
ENV MARKETPLACE_PORT=11102

# Addons Engine configuration
ENV ADDONS_ENGINE_PROTOCOL=http
ENV ADDONS_ENGINE_URL=localhost
ENV ADDONS_ENGINE_PORT=11101

# Resource Abstractor configuration
ENV RESOURCE_ABSTRACTOR_PROTOCOL=http
ENV RESOURCE_ABSTRACTOR_URL=localhost
ENV RESOURCE_ABSTRACTOR_PORT=11011

# Dashboard port
ENV DASHBOARD_PORT=11103

# Expose dashboard port
EXPOSE ${DASHBOARD_PORT}

# Use custom entrypoint
ENTRYPOINT ["/entrypoint.sh"]
