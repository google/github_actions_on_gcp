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
    --update-secrets=${WEBHOOK_KEY_PATH}=${KEY_NAME}:latest \
    --allow-unauthenticated \
    --set-env-vars=APP_ID=${APP_ID},TRIGGER_ID=${TRIGGER_ID},PROJECT_ID=${PROJECT_ID},KEY_ID=${KEY_ID},TRIGGER_NAME=${TRIGGER_NAME},LOCATION=${LOCATION},WEBHOOK_KEY_PATH=${WEBHOOK_KEY_PATH}
```
