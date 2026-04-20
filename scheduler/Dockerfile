FROM golang:alpine AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/scheduler ./cmd

FROM debian:bullseye-slim

COPY --from=build-stage /bin/scheduler /scheduler

ENV RESOURCE_ABSTRACTOR_URL=localhost
ENV RESOURCE_ABSTRACTOR_PORT=11011

ENV REDIS_ADDR=redis://:rootRedis@redis:6379

ENTRYPOINT ["./scheduler"]