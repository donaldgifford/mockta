# Trivial module-under-test for the mockta smoke fixture.
#
# Creates one of each v0 resource type (user, group, app, membership)
# and outputs the IDs. The smoke.tftest.hcl asserts non-empty IDs to
# confirm the okta/okta provider can plan + apply against mockta
# without erroring.

terraform {
  required_version = ">= 1.7.0"
  required_providers {
    okta = {
      source  = "okta/okta"
      version = "~> 4.0"
    }
  }
}

variable "okta_base_url" {
  type        = string
  description = "Base URL of the mockta sidecar (no trailing slash)."
}

variable "okta_org_name" {
  type        = string
  description = "Org name the provider embeds in the host header."
}

variable "okta_api_token" {
  type        = string
  sensitive   = true
  description = "Bearer token mockta validates."
}

provider "okta" {
  org_name = var.okta_org_name
  # base_url is the host (no scheme); the provider builds the URL
  # internally. Point it at the mockta sidecar via host:port.
  base_url  = replace(var.okta_base_url, "http://", "")
  api_token = var.okta_api_token

  # Skip the version-check call — mockta doesn't implement it.
  http_proxy = ""
}

resource "okta_user" "alice" {
  first_name = "Alice"
  last_name  = "Smoke"
  login      = "alice@smoke.example"
  email      = "alice@smoke.example"
}

resource "okta_group" "engineers" {
  name        = "engineers-smoke"
  description = "Smoke-test group"
}

resource "okta_group_memberships" "alice_in_engineers" {
  group_id = okta_group.engineers.id
  users    = [okta_user.alice.id]
}

resource "okta_app_saml" "acme_saml" {
  label                    = "Acme SAML Smoke"
  sso_url                  = "https://smoke.example/sso"
  recipient                = "https://smoke.example/recipient"
  destination              = "https://smoke.example/destination"
  audience                 = "https://smoke.example/audience"
  subject_name_id_template = "$${user.userName}"
  subject_name_id_format   = "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified"
  signature_algorithm      = "RSA_SHA256"
  digest_algorithm         = "SHA256"
  authn_context_class_ref  = "urn:oasis:names:tc:SAML:2.0:ac:classes:PasswordProtectedTransport"
}

output "user_id" {
  value = okta_user.alice.id
}
output "group_id" {
  value = okta_group.engineers.id
}
output "app_id" {
  value = okta_app_saml.acme_saml.id
}
