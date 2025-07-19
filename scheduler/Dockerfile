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

ENV SYSTEM_MANAGER_URL=system_manager
ENV SYSTEM_MANAGER_PORT=10000

ENV REDIS_ADDR=redis://:cloudRedis@redis:6379

ENV MY_PORT=10004
EXPOSE 10004

ENTRYPOINT ["./scheduler"]