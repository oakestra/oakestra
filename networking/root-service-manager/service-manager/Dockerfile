# --- Builder Stage ---
FROM python:3.10-slim-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ git libffi-dev

COPY requirements.txt .

RUN pip install --no-cache-dir -r requirements.txt

RUN apt-get purge -y --auto-remove gcc g++ git libffi-dev && \
    apt-get clean

COPY . .

# --- Final Stage ---
FROM python:3.10-slim-bookworm AS final
LABEL org.opencontainers.image.source=https://github.com/oakestra/oakestra-net

WORKDIR /app

# artifacts from the builder stage
COPY --from=builder /app .
# installed dependencies
COPY --from=builder /usr/local/lib/python3.10/site-packages /usr/local/lib/python3.10/site-packages

# Set environment variables
ENV FLASK_DEBUG=FALSE
ENV CLOUD_MONGO_URL=localhost
ENV CLOUD_MONGO_PORT=10107
ENV MY_PORT=10100
ENV SYSTEM_MANAGER_URL=system_manager
ENV SYSTEM_MANAGER_PORT=10000

EXPOSE 10100

CMD ["python", "service_manager.py"]