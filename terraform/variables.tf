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
  default     = "action-dispatcher"
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
    # BUILD_LOCATION           = ""
    # PROJECT_ID               = ""
    # WEBHOOK_KEY_MOUNT_PATH   = "/etc/secrets/webhook/key"
  }
  description = "Environment variables for the Cloud Run service (plain text)."
}

variable "kms_keyring_name" {
  description = "Keyring name."
  type        = string
  default     = "webhook-keyring"
}

variable "kms_key_location" {
  description = "The location where kms key will be created."
  type        = string
  default     = "global"
}

variable "kms_key_name" {
  description = "Name of the key containing the GitHub App secret key."
  type        = string
  default     = "webhook-github-app-secret-key"
}

variable "kms_key_purpose" {
  description = "Purpose of the GitHub App secret key."
  type        = string
  default     = "ASYMMETRIC_SIGN"
}

variable "kms_key_algorithm" {
  description = "Algorithm of the GitHub App secret key."
  type        = string
  # This is how GitHub App private keys import as of 2025-02-25.
  default = "RSA_SIGN_PKCS1_2048_SHA256"
}

variable "kms_key_version" {
  description = "Version of the KMS key to use."
  type        = string
  default     = "1"
  validation {
    condition     = can(regex("^\\d+$", var.kms_key_version))
    error_message = "The KMS key version must be a positive integer."
  }
}

variable "runner_project_ids" {
  description = "The project IDs to be used as a runner."
  type        = list(string)

  validation {
    condition     = length(var.runner_project_ids) == 1 && var.runner_project_ids[0] != ""
    error_message = "Exactly one runner project must be specified."
  }
}
