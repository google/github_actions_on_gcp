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
