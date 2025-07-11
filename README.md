# GitHub Actions on Google Cloud Platform

## Prerequisites

### Development Tools

```shell
$ curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.64.7
go install mvdan.cc/gofumpt@latest
go install github.com/daixiang0/gci@latest
```

## Importing GitHub App Private Key

```shell
go run github.com/abcxyz/github-token-minter/cmd/minty@main \
    private-key import \
    -key=${KEY_NAME} \
    -key-ring=${KEY_RING_NAME} \
    -project-id=${PROJECT_ID} \
    -private-key=@${KEY_FILE_NAME}
```

## Setup Steps

### GitHub App Creation

You need a GitHub App in your GitHub org.

To do this go to your org settings page:
https://github.com/organizations/${YOUR_ORG}/settings/profile

1. Expand Developer settings (last option on left sidebar) and click GitHub Apps.
2. Click "New GitHub App" on top right.
3. Give your App a name and Homepage URL (it doesn't matter what you have there).
4. Note where "Webhook" is. Uncheck "Active" for now, we will configure later.
5. Expand "Repository Permissions". Add following:
    - Actions: Read-only
    - Administration: Read and Write
    - Metadata: Read-Only
6. Expand "Organization Permissions". Add following:
    - Administration: Read and Write # **TODO: is this needed?**
    - Self-hosted runners: Read and Write
7. Click "Create GitHub App" at bottom of screen.
8. Find your app listed in Developer Settings in your org settings page.
    - Take note of **App ID**
    - Click Edit
    - Scroll down to almost the bottom for "Private Keys"
    - Click "Generate a private key"
    - A .pem file will be downloaded. This is a secret. Keep it safe.

### GitHub App Installation

Now that the GitHub App exists, it needs to be added to your org.

1. Navigate to Org setting page https://github.com/organizations/${YOUR_ORG}/settings/profile
2. Expand Developer settings (last option on left sidebar) and click GitHub Apps.
3. Select Your App
4. On the left bar, select "Install App"
5. Select your org.

### Validate JIT Config Locally (Optional)
Prereqs: Sudoless docker installed. Not currently set up to work with GitHub Enterprise Server.

Now we have an app and a .pem key, we should be able to create JIT configs. These
are one-time tokens that allow a runner to register itself with GitHub.

`test_local.sh` is set up for this. You just need to change a few values in
the `go run` command under `# Generate JIT Config`:
1. Set **app-id** to the app id found in Step 8. of GitHub App Creation.
2. Set `private-key` to the path of your `.pem` file you downloaded.
3. Set `org` to the name of your GitHub org.
4. Set `runner-group-id` to the value of your runner group. You can just use `1` which is the default runner group added to each org.

Now run the script. You should see it build the image,
then see output for runner startup and finally see it waiting for a request.

If this doesn't happen, something went wrong, you should figure it out before
continuing.

### Setup GCP Infrastructure

TODO

## CI/CD Testing Setup

The CI/CD pipeline for this project includes a workflow to test pull requests that modify the webhook. This workflow deploys a temporary, isolated instance of the webhook to the `autopush` environment and runs a series of tests against it.

### PR Test Webhook Secret

To enable secure end-to-end testing, a dedicated webhook secret named `webhook-pr-test-secret` is used. This secret is managed as a permanent resource in the `google-infra-gcp` repository and is stored in Google Cloud Secret Manager in the `action-dispatcher-webhook-a-18` project.

The `github-automation-bot` service account has the `roles/secretmanager.secretAccessor` permission for this secret, allowing the CI/CD pipeline to fetch it and use it to sign mock webhook payloads.

#### Secret Rotation

If this secret needs to be rotated, follow these steps:

1.  **Generate a new secret value:**
    ```shell
    openssl rand -hex 32
    ```

2.  **Add the new value as a new version to the existing secret:**
    You must have the `secretmanager.secretVersionAdder` IAM role on the `action-dispatcher-webhook-a-18` project to perform this action.

    ```shell
    printf "YOUR_NEW_SECRET_VALUE\n" | gcloud secrets versions add "webhook-pr-test-secret" --data-file=- --project="action-dispatcher-webhook-a-18"
    ```

3.  The CI/CD workflow will automatically pick up the `latest` version of the secret.
