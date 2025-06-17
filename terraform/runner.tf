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

resource "google_project_service" "runner" {
  for_each = toset([
    "cloudbuild.googleapis.com",
    "logging.googleapis.com",
  ])

  project = var.runner_project_id

  service                    = each.value
  disable_on_destroy         = false
  disable_dependent_services = false
}

resource "google_service_account" "runner_sa" {
  project = var.runner_project_id

  account_id   = "${var.name}-runner-sa"
  display_name = "${var.name}-runner-sa Cloud Build Service Account"
}

# Allow the runner service account to write logs
resource "google_project_iam_member" "write_logs_permission" {
  project = var.runner_project_id

  role   = "roles/logging.logWriter"
  member = google_service_account.runner_sa.member
}

# Allow the webhook project to call CreateBuild to kick off the runner
resource "google_project_iam_member" "build_trigger_permission" {
  project = var.runner_project_id

  role   = "roles/cloudbuild.builds.editor"
  member = google_service_account.run_service_account.member
}

# Allow the webhook project to run as the runner service account
resource "google_service_account_iam_member" "build_runner_permission" {
  service_account_id = google_service_account.runner_sa.name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.run_service_account.member
}
