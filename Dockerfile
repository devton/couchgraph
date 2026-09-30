FROM golang:1.26-alpine AS builder

WORKDIR /app

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /couchgraph ./cmd/server

# ── Final image ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=builder /couchgraph /couchgraph

EXPOSE 8080

ENTRYPOINT ["/couchgraph"]
