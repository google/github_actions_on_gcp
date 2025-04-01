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

resource "google_project_service" "default" {
  for_each = toset([
    "cloudresourcemanager.googleapis.com",
    "logging.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com",
    "sts.googleapis.com",
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
  secrets               = ["github-application-id", "github-app-private-key-id"]
  service_account_email = google_service_account.run_service_account.email
  service_iam = {
    admins     = var.service_iam.admins
    developers = toset(concat(var.service_iam.developers, [var.ci_service_account_member]))
    invokers   = toset(var.service_iam.invokers)
  }

  envvars = var.envvars

  secret_envvars = {
    # FIXME(pberruti): Does this really have to be a secret instead of a regular ENV var?
    "APP_ID" : {
      name : "github-application-id",
      version : "latest",
    },
    # FIXME(pberruti): There's probably a better way to do this with a Terraform definition for a KMS key - it also might not be sensitive enough to require a secret?
    "KEY_ID" : {
      name : "github-app-private-key-id",
      version : "latest",
    }
    # FIXME(pberruti): What about a mounted Secret Manager key?
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
