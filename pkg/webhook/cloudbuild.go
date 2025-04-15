// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"context"
	"fmt"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"google.golang.org/api/option"

	"github.com/googleapis/gax-go/v2"
)

// CloudBuild provides a client and dataset identifiers.
type CloudBuild struct {
	client *cloudbuild.Client
}

// NewCloudBuild creates a new instance of a CloudBuild client.
func NewCloudBuild(ctx context.Context, opts ...option.ClientOption) (*CloudBuild, error) {
	client, err := cloudbuild.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new key management client: %w", err)
	}

	return &CloudBuild{
		client: client,
	}, nil
}

func (cb *CloudBuild) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest, opts ...gax.CallOption) error {
	if _, err := cb.client.CreateBuild(ctx, req); err != nil {
		return fmt.Errorf("failed to create cloud build build: %w", err)
	}
	return nil
}

// Close releases any resources held by the CloudBuild client.
func (cb *CloudBuild) Close() error {
	if err := cb.client.Close(); err != nil {
		return fmt.Errorf("failed to close CloudBuild client: %w", err)
	}
	return nil
}
