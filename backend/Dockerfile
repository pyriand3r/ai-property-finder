# syntax=docker/dockerfile:1.7
#
# Backend-only build for Layer 0 — no frontend stage yet (the frontend
# doesn't exist until Layer 4.2). Once it does, embedding its dist/ output
# here needs a build context wide enough to reach both backend/ and the
# frontend source; revisit this Dockerfile at that point.

FROM golang:1.26-alpine AS build
WORKDIR /app
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/property-finder ./cmd/server

FROM alpine:3.21 AS runtime
RUN apk add --no-cache wget ca-certificates tzdata && \
    adduser -D -u 10001 app
RUN mkdir -p /data && chown -R app:app /data
COPY --from=build /out/property-finder /usr/local/bin/property-finder
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/property-finder"]
