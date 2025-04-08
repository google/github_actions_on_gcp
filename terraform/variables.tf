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

variable "project_id" {
  description = "The GCP project ID."
  type        = string
}

variable "name" {
  description = "The name of this component."
  type        = string
  default     = "github-actions-on-gcp"
  validation {
    condition     = can(regex("^[A-Za-z][0-9A-Za-z-]+[0-9A-Za-z]$", var.name))
    error_message = "Name can only contain letters, numbers, hyphens(-) and must start with letter."
  }
}

# This current approach allows the end-user to disable the GCLB in favor of calling the Cloud Run service directly.
# This was done to use tagged revision URLs for integration testing on multiple pull requests.
variable "enable_gclb" {
  description = "Enable the use of a Google Cloud load balancer for the Cloud Run service. By default this is true, this should only be used for integration environments where services will use tagged revision URLs for testing."
  type        = bool
  default     = true
}

variable "domains" {
  description = "Domain names for the Google Cloud Load Balancer."
  type        = list(string)
}

variable "image" {
  description = "Cloud Run service image name to deploy."
  type        = string
  default     = "gcr.io/cloudrun/hello:latest"
}

variable "service_iam" {
  description = "IAM member bindings for the Cloud Run service."
  type = object({
    admins     = list(string)
    developers = list(string)
    invokers   = list(string)
  })
  default = {
    admins     = []
    developers = []
    invokers   = []
  }
}

variable "ci_service_account_member" {
  type        = string
  description = "The service account member for deploying revisions to Cloud Run"
}

variable "github_owner_id" {
  description = "The ID of the GitHub organization. If specified, the WIF pool will limit traffic to a single GitHub organization."
  type        = string
  default     = ""
}

variable "github_enterprise_id" {
  description = "The ID of the GitHub enterprise. If specified, the WIF pool will limit traffic to a single GitHub enterprise."
  type        = string
  default     = ""
}

variable "alerts" {
  description = "The configuration block for service alerts and notifications"
  type = object({
    enabled             = bool
    channels_non_paging = map(any)
  })
  default = {
    enabled = false
    channels_non_paging = {
      email = {
        labels = {
          email_address = ""
        }
      }
    }
  }
}

variable "envvars" {
  type = map(string)
  default = {
    # GITHUB_APP_ID            = ""
    # KMS_APP_PRIVATE_KEY_ID   = ""
    # BUILD_TRIGGER_LOCATION   = ""
    # BUILD_TRIGGER_PROJECT_ID = ""
    # BUILD_TRIGGER_ID         = ""
    # BUILD_TRIGGER_NAME       = ""
    # WEBHOOK_KEY_MOUNT_PATH   = "/etc/secrets/webhook/key"
  }
  description = "Environment variables for the Cloud Run service (plain text)."
}
