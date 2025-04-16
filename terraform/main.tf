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

  account_id   = "${var.name}-sa"
  display_name = "${var.name}-sa Cloud Run Service Account"
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
  purpose  = "ASYMMETRIC_SIGN"

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

resource "google_kms_crypto_key_version" "app_private_key_version" {
  crypto_key = google_kms_crypto_key.webhook_app_private_key.id
  # This is how GitHub App private keys import as of 2025-02-25.
  algorithm = "RSA_SIGN_PKCS1_2048_SHA256"

  depends_on = [
    google_project_service.default["cloudkms.googleapis.com"],
  ]
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
  source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref=ebaccaa0c906e89813e3b0b71fc5fc6be9ef0cdb"

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

  envvars = merge(
    var.envvars,
    {
      "KMS_APP_PRIVATE_KEY_ID" : google_kms_crypto_key_version.app_private_key_version.id
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
# cloud run sevice
resource "google_service_account_iam_member" "run_sa_ci_binding" {
  service_account_id = google_service_account.run_service_account.name
  role               = "roles/iam.serviceAccountUser"
  member             = var.ci_service_account_member
}
