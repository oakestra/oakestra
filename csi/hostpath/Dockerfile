# ─── Build stage ────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /src

# Cache dependency downloads separately from the source build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /hostpath-csi .

# ─── Runtime stage ───────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:latest@sha256:61b7ccecebc7c474a531717de80a94709d20547cdcdaf740c25876f2a8e38b44

COPY --from=builder /hostpath-csi /usr/local/bin/hostpath-csi

# The default socket path matches the Node Engine's default configuration.
ENV CSI_ENDPOINT="unix:///var/lib/oakestra/csi/hostpath.sock"

ENV BASE_MOUNT_PATH="/mnt/oakestra/hostpath"

ENTRYPOINT ["/usr/local/bin/hostpath-csi"]
CMD ["--endpoint", "unix:///var/lib/oakestra/csi/hostpath.sock"]
