# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION="0.0.0-dev"
ARG COMMIT="unknown"

WORKDIR /src

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /mockta ./cmd/mockta

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /mockta /mockta

EXPOSE 8080 9090

USER nonroot:nonroot

# Healthcheck calls the in-binary subcommand so we don't need curl in
# the distroless image. The subcommand probes :9090/health and exits
# non-zero on failure. Docker uses the exit status; default interval
# and timeout match what Compose/K8s probes typically expect.
HEALTHCHECK --interval=10s --timeout=2s --start-period=5s --retries=3 \
  CMD ["/mockta", "healthcheck"]

ENTRYPOINT ["/mockta"]
CMD ["serve"]
