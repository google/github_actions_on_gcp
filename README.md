# GitHub Actions on Google Cloud Platform

## Prerequisites

### Development Tools

```shell
$ curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.64.7
go install mvdan.cc/gofumpt@latest
go install github.com/daixiang0/gci@latest
```

## Deploy Prototype

```shell
gcloud run deploy webhook-go \
    --region=us-west1 \
    --source . \
    --update-secrets=${WEBHOOK_KEY_MOUNT_PATH}=${KEY_NAME}:latest \
    --allow-unauthenticated \
    --set-env-vars=APP_ID=${APP_ID},PROJECT_ID=${PROJECT_ID},KMS_APP_PRIVATE_KEY_ID=${KMS_APP_PRIVATE_KEY_ID},BUILD_LOCATION=${BUILD_LOCATION},WEBHOOK_KEY_MOUNT_PATH=${WEBHOOK_KEY_MOUNT_PATH}
```
