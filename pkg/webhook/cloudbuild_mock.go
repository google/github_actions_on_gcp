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

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"

	"github.com/googleapis/gax-go/v2"
)

type MockCloudBuildClient struct {
	createBuildReq *cloudbuildpb.CreateBuildRequest
	createBuildErr error
}

func (m *MockCloudBuildClient) CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest, opts ...gax.CallOption) error {
	m.createBuildReq = req
	if m.createBuildErr != nil {
		return m.createBuildErr
	}
	return nil
}

func (m *MockCloudBuildClient) Close() error {
	return nil
}
