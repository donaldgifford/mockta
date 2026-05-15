# mockta — docker recipes
#
# Docker image build/push pipeline via docker buildx bake. Reads
# docker-bake.hcl for target definitions. Import this from the main
# justfile with `import 'docker.just'`.

# ─── Docker ─────────────────────────────────────────────────────────

# Local single-arch build via docker buildx bake
[group('docker')]
docker-build:
    @docker buildx bake

# Multi-arch build and push via docker buildx bake
[group('docker')]
docker-buildx:
    @docker buildx bake release

# Push the image (requires prior build)
[group('docker')]
docker-push:
    @docker buildx bake --push

# CI build (linux/amd64 only, for fast PR validation)
[group('docker')]
docker-ci:
    @docker buildx bake ci

# Smoke-test the locally built image. Loads it into the daemon (the
# default bake target uses the registry exporter via --output=type=docker
# behavior for single-arch); we override with --load so the image is
# directly addressable. Then probes /health and /api/v1/org.
[group('docker')]
docker-smoke:
    @docker buildx bake --load
    @docker rm -f mockta-smoke 2>/dev/null || true
    @docker run -d --name mockta-smoke -p 18080:8080 -p 19090:9090 \
        -e MOCKTA_ADMIN_TOKEN=smoke-token \
        ghcr.io/donaldgifford/mockta:dev
    @sleep 2
    @echo "→ /health"
    @curl -sf http://localhost:19090/health
    @echo
    @echo "→ /api/v1/org"
    @curl -sf -H 'Authorization: Bearer smoke-token' \
        http://localhost:18080/api/v1/org
    @echo
    @docker rm -f mockta-smoke
    @echo "✓ smoke OK"

# Verify the locally built image is at or below the 15 MB v0 budget.
# Phase 6 success criterion. Reads the size from `docker image inspect`
# and compares against the threshold; non-zero exit fails the gate.
[group('docker')]
docker-size:
    @docker buildx bake --load
    @{ \
        size=$(docker image inspect ghcr.io/donaldgifford/mockta:dev \
            --format '{{ "{{.Size}}" }}'); \
        max=$((15 * 1024 * 1024)); \
        echo "image size: $size bytes (limit: $max)"; \
        if [ "$size" -gt "$max" ]; then \
            echo "✗ image exceeds 15 MB budget"; \
            exit 1; \
        fi; \
        echo "✓ within 15 MB budget"; \
    }
