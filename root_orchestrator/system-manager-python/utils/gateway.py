import os

# Set only by override-gateway.yml; every gateway/mTLS code path is gated on it.
GATEWAY_ENABLED = os.environ.get("GATEWAY_ENABLED", "").lower() in ("true", "1", "yes")
