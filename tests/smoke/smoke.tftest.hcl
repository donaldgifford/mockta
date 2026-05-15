# Smoke fixture for mockta — exercises the okta/okta provider
# against a containerized mockta sidecar.
#
# This file is consumed by `terraform test`. The CI workflow builds
# the mockta image (via docker buildx bake), loads it into the
# local daemon, then runs this fixture. Each `run` block walks one
# Terraform lifecycle phase; assertions confirm the provider sees
# the IDs mockta returns.

variables {
  mockta_image = "ghcr.io/donaldgifford/mockta:dev"
}

# Boot the sidecar and capture its outputs. The setup module reads
# the TLS material from /tmp/mockta-smoke-certs/ — CI populates that
# directory before running the test; the same directory is what
# `just test-smoke` uses locally.
run "setup" {
  module {
    source = "./setup"
  }

  variables {
    mockta_image = var.mockta_image
  }
}

# Apply the module-under-test against the running sidecar.
run "apply_module" {
  command = apply

  module {
    source = "./module"
  }

  variables {
    okta_base_url  = run.setup.mockta_base_url
    okta_org_name  = run.setup.okta_org_name
    okta_api_token = run.setup.okta_api_token
  }

  assert {
    condition     = output.user_id != ""
    error_message = "okta_user.alice.id is empty — provider didn't get an ID back"
  }

  assert {
    condition     = output.group_id != ""
    error_message = "okta_group.engineers.id is empty"
  }

  assert {
    condition     = output.app_id != ""
    error_message = "okta_app_saml.acme_saml.id is empty"
  }
}
