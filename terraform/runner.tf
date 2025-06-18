# Copyright 2025 The Authors (see AUTHORS file)
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

locals {
  runner_services = toset([
    "cloudbuild.googleapis.com",
    "logging.googleapis.com",
  ])
}

resource "google_project_service" "runner" {
  for_each = {
    for pair in setproduct(var.runner_project_ids, local.runner_services) :
    "${pair[0]}-${pair[1]}" => {
      project_id = pair[0]
      service    = pair[1]
    }
  }

  project = each.value.project_id

  service                    = each.value.service
  disable_on_destroy         = false
  disable_dependent_services = false
}

resource "google_service_account" "runner_sa" {
  for_each = toset(var.runner_project_ids)

  project      = each.key
  account_id   = "${var.name}-runner-sa"
  display_name = "${var.name}-runner-sa Cloud Build Service Account"
}

# Allow the runner service account to write logs
resource "google_project_iam_member" "write_logs_permission" {
  for_each = toset(var.runner_project_ids)

  project = each.key
  role    = "roles/logging.logWriter"
  member  = google_service_account.runner_sa[each.key].member
}

# Allow the webhook project to call CreateBuild to kick off the runner
resource "google_project_iam_member" "build_trigger_permission" {
  for_each = toset(var.runner_project_ids)

  project = each.key
  role    = "roles/cloudbuild.builds.editor"
  member  = google_service_account.run_service_account.member
}

# Allow the webhook project to run as the runner service account
resource "google_service_account_iam_member" "build_runner_permission" {
  for_each = google_service_account.runner_sa

  service_account_id = each.value.name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.run_service_account.member
}