FROM golang:1.25-alpine AS build-stage
LABEL org.opencontainers.image.source="https://github.com/oakestra/oakestra"

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 for a fully static binary (required by the distroless/static
# runtime image below, which has no libc). -trimpath/-ldflags="-s -w" strip
# debug info and local paths to keep the binary small.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/resource_abstractor .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build-stage /bin/resource_abstractor /resource_abstractor

ENV LOG_LEVEL=WARNING
ENV MONGO_URL=localhost
ENV MONGO_PORT=10007

# Serves both the root (11011) and cluster (11012) deployments from the
# same image, matching the Python service's Dockerfile.
EXPOSE 11011
EXPOSE 11012

ENTRYPOINT ["/resource_abstractor"]
