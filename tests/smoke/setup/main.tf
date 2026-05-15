# mockta smoke-test setup module.
#
# Starts a mockta container as a sidecar via the docker provider,
# exposes random host ports for the API (:8080) and admin (:9090)
# listeners, and waits for the in-binary healthcheck to pass. The
# top-level smoke.tftest.hcl wires the outputs into the
# module-under-test as variables.
#
# Image tag is parameterized so CI can build a per-PR image (e.g.
# `ghcr.io/donaldgifford/mockta:sha-<commit>`) and run the smoke
# fixture against it without modifying this file.

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

resource "docker_container" "mockta" {
  image = var.mockta_image
  name  = "mockta-smoke-${terraform.workspace}"

  env = [
    "MOCKTA_ADMIN_TOKEN=${var.admin_token}",
    "MOCKTA_ORG_NAME=${var.org_name}",
  ]

  ports {
    internal = 8080
    external = 0
  }
  ports {
    internal = 9090
    external = 0
  }

  # Terraform 1.7+ blocks the run until the container reports healthy.
  # The Dockerfile's HEALTHCHECK calls `mockta healthcheck`, which
  # probes :9090/health and exits non-zero on failure.
  healthcheck {
    test         = ["CMD", "/mockta", "healthcheck"]
    interval     = "1s"
    retries      = 30
    start_period = "2s"
  }
}

output "mockta_base_url" {
  value       = "http://localhost:${docker_container.mockta.ports[0].external}"
  description = "Provider base_url — points at the API listener."
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
