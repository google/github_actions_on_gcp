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

terraform {
  required_version = ">= 1.7"

  required_providers {
    google = {
      version = ">= 5.19"
      source  = "hashicorp/google"
    }
  }
}

# FIXME(pberruti): Decipher this and see if it needs addressing
# Warning: Available Write-only Attribute Alternative
# │
# │   with module.cloud_run.google_secret_manager_secret_version.secrets_default_version,
# │   on .terraform/modules/cloud_run/modules/cloud_run/main.tf line 265, in resource "google_secret_manager_secret_version" "secrets_default_version":
# │  265:   secret_data = "DEFAULT_VALUE"
# │
# │ The attribute secret_data has a write-only alternative secret_data_wo available. Use the write-only
# │ alternative of the attribute when possible.
