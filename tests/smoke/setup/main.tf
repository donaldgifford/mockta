terraform {
  required_version = ">= 1.7.0"
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

variable "mockta_image" {
  type        = string
  default     = "ghcr.io/donaldgifford/mockta:dev"
  description = "Container image tag to smoke-test."
}

variable "admin_token" {
  type        = string
  default     = "smoke-test-token"
  description = "MOCKTA_ADMIN_TOKEN value; arbitrary non-empty string."
}

variable "org_name" {
  type        = string
  default     = "acme-smoke"
  description = "MOCKTA_ORG_NAME value surfaced via /api/v1/org."
}

variable "tls_cert_path" {
  type        = string
  description = "Absolute path on the host to a PEM cert whose SAN covers acme-smoke.localhost. Caddy reverse-proxies HTTPS:443 to mockta:8080 using this cert; the okta provider verifies it against a CA the runner already trusts."
}

variable "tls_key_path" {
  type        = string
  description = "Absolute path on the host to the matching PEM key for tls_cert_path."
}

variable "caddy_image" {
  type        = string
  default     = "caddy:2.8-alpine"
  description = "Caddy image used for TLS termination in front of mockta."
}

resource "docker_network" "smoke" {
  name = "mockta-smoke-${terraform.workspace}"
}

resource "docker_container" "mockta" {
  image = var.mockta_image
  name  = "mockta-smoke-${terraform.workspace}"

  env = [
    "MOCKTA_ADMIN_TOKEN=${var.admin_token}",
    "MOCKTA_ORG_NAME=${var.org_name}",
  ]

  networks_advanced {
    name    = docker_network.smoke.name
    aliases = ["mockta"]
  }

  healthcheck {
    test         = ["CMD", "/mockta", "healthcheck"]
    interval     = "1s"
    retries      = 30
    start_period = "2s"
  }
}

resource "docker_image" "caddy" {
  name         = var.caddy_image
  keep_locally = true
}

# Caddy terminates TLS in front of mockta. The okta terraform
# provider hard-builds URLs as `https://<org>.<base_url>` and
# defaults the port to 443; it will not speak plain HTTP. So we
# put a TLS terminator on host :443 that reverse-proxies to
# mockta:8080 on the docker network. The cert/key are generated
# out-of-band in CI (or by `just docker-smoke-tls`) and bind-mounted
# in.
resource "docker_container" "caddy" {
  image = docker_image.caddy.image_id
  name  = "caddy-smoke-${terraform.workspace}"

  depends_on = [docker_container.mockta]

  networks_advanced {
    name = docker_network.smoke.name
  }

  ports {
    internal = 443
    external = 443
  }

  mounts {
    type      = "bind"
    source    = abspath("${path.module}/Caddyfile")
    target    = "/etc/caddy/Caddyfile"
    read_only = true
  }

  mounts {
    type      = "bind"
    source    = var.tls_cert_path
    target    = "/certs/server.crt"
    read_only = true
  }

  mounts {
    type      = "bind"
    source    = var.tls_key_path
    target    = "/certs/server.key"
    read_only = true
  }

  command = ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]

  healthcheck {
    test         = ["CMD", "wget", "-q", "--no-check-certificate", "-O", "-", "https://localhost/health"]
    interval     = "1s"
    retries      = 30
    start_period = "1s"
  }
}

output "mockta_base_url" {
  value       = "localhost"
  description = "Provider base_url. The okta provider builds <org_name>.<base_url> and dials :443, which is where Caddy listens."
}

output "okta_org_name" {
  value       = var.org_name
  description = "Echoed back so the smoke module can build the okta_*_id fields."
}

output "okta_api_token" {
  value       = var.admin_token
  sensitive   = true
  description = "Bearer token the provider must present on every call."
}
