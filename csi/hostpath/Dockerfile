# ─── Build stage ────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /src

# Cache dependency downloads separately from the source build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /hostpath-csi .

# ─── Runtime stage ───────────────────────────────────────────────────────────
FROM alpine:3.21

# The plugin performs Linux bind-mounts via syscall; no extra tools needed.
# util-linux is included only for diagnostic convenience (findmnt, umount).
RUN apk add --no-cache util-linux

COPY --from=builder /hostpath-csi /usr/local/bin/hostpath-csi

# The default socket path matches the Node Engine's default configuration.
ENV CSI_ENDPOINT="unix:///var/lib/oakestra/csi/hostpath.sock"

ENV BASE_MOUNT_PATH="/mnt/oakestra/hostpath"

ENTRYPOINT ["/usr/local/bin/hostpath-csi"]
CMD ["--endpoint", "unix:///var/lib/oakestra/csi/hostpath.sock"]
