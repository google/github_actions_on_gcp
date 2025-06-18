# Copyright 2023 The Authors (see AUTHORS file)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

resource "random_id" "default" {
  byte_length = 2
}

resource "google_project_service" "default" {
  for_each = toset([
    "cloudbuild.googleapis.com",
    "cloudkms.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "logging.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com",
    "sts.googleapis.com",
    "secretmanager.googleapis.com",
  ])

  project = var.project_id

  service                    = each.value
  disable_on_destroy         = false
  disable_dependent_services = false # To keep, or not to keep? From github-wif module
}

resource "google_service_account" "run_service_account" {
  project = var.project_id

  account_id   = "${var.name}-webhook-sa"
  display_name = "${var.name}-webhook-sa Cloud Run Service Account"
}

resource "google_kms_key_ring" "webhook_keyring" {
  project = var.project_id

  name     = "${var.kms_keyring_name}-${random_id.default.hex}"
  location = var.kms_key_location

  depends_on = [
    google_project_service.default["cloudkms.googleapis.com"],
  ]
}

resource "google_kms_crypto_key" "webhook_app_private_key" {
  name     = "${var.kms_key_name}-${random_id.default.hex}"
  key_ring = google_kms_key_ring.webhook_keyring.id
  purpose  = var.kms_key_purpose

  version_template {
    algorithm = var.kms_key_algorithm
  }

  # There's no guarantee that the underlying crypto key version is actually created,
  # instead manually create the version
  skip_initial_version_creation = "true"

  depends_on = [
    google_project_service.default["cloudkms.googleapis.com"],
  ]

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_kms_crypto_key_iam_member" "webhook_app_private_key_public_key_viewer" {
  crypto_key_id = google_kms_crypto_key.webhook_app_private_key.id
  role          = "roles/cloudkms.publicKeyViewer"
  member        = "serviceAccount:${google_service_account.run_service_account.email}"
}

resource "google_kms_crypto_key_iam_member" "webhook_app_private_key_signer" {
  crypto_key_id = google_kms_crypto_key.webhook_app_private_key.id
  role          = "roles/cloudkms.signer"
  member        = "serviceAccount:${google_service_account.run_service_account.email}"
}

module "gclb" {
  count = var.enable_gclb ? 1 : 0

  source = "git::https://github.com/abcxyz/terraform-modules.git//modules/gclb_cloud_run_backend?ref=ebaccaa0c906e89813e3b0b71fc5fc6be9ef0cdb"

  project_id = var.project_id

  name             = var.name
  run_service_name = module.cloud_run.service_name
  domains          = var.domains
}

module "cloud_run" {
  source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref=1467eaf0115f71613727212b0b51b3f99e699842"

  project_id = var.project_id

  name                  = var.name
  image                 = var.image
  ingress               = var.enable_gclb ? "internal-and-cloud-load-balancing" : "all"
  min_instances         = 1
  secrets               = ["webhook-secret-file"]
  service_account_email = google_service_account.run_service_account.email
  args                  = ["webhook", "server"]
  service_iam = {
    admins     = var.service_iam.admins
    developers = toset(concat(var.service_iam.developers, [var.ci_service_account_member]))
    invokers   = toset(var.service_iam.invokers)
  }

  additional_service_annotations = {
    # GitHub webhooks call without authorization so the service
    # must allow unauthenticated requests to come through
    "run.googleapis.com/invoker-iam-disabled" : true
  }

  envvars = merge(
    var.envvars,
    {
      "KMS_APP_PRIVATE_KEY_ID" : format("%s/cryptoKeyVersions/%s", google_kms_crypto_key.webhook_app_private_key.id, var.kms_key_version)
      "RUNNER_PROJECT_ID" : var.runner_project_ids[0]
      "RUNNER_SERVICE_ACCOUNT" : google_service_account.runner_service_accounts[0]
    }
  )

  secret_envvars = {}

  secret_volumes = {
    "${var.envvars["WEBHOOK_KEY_MOUNT_PATH"]}" : {
      name : "webhook-secret-file",
      version : "latest",
    }
  }
}

# allow the ci service account to act as the cloud run service account
# this allows the ci service account to deploy new revisions for the
# cloud run service
resource "google_service_account_iam_member" "run_sa_ci_binding" {
  service_account_id = google_service_account.run_service_account.name
  role               = "roles/iam.serviceAccountUser"
  member             = var.ci_service_account_member
}
