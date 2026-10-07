FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /app

# Download dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Multi-arch compilation
ARG TARGETOS TARGETARCH
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o /couchgraph ./cmd/couchgraph && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o /couchgraph-server ./cmd/server

# ── Final image ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=builder /couchgraph /couchgraph
COPY --from=builder /couchgraph-server /couchgraph-server

EXPOSE 8080

ENTRYPOINT ["/couchgraph"]
CMD ["serve"]

