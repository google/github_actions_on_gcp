// Copyright 2025 Google LLC
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

package cli

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/sethvargo/go-envconfig"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"

	"github.com/google/github_actions_on_gcp/pkg/webhook"
)

func TestWebhookServerCommand(t *testing.T) {
	t.Parallel()

	ctx := logging.WithLogger(t.Context(), logging.TestLogger(t))

	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		expErr   string
		fileMock *webhook.MockFileReader
	}{
		{
			name:   "too_many_args",
			args:   []string{"foo"},
			expErr: `unexpected arguments: ["foo"]`,
		},
		{
			name:   "invalid_config_build_location",
			env:    map[string]string{},
			expErr: `BUILD_LOCATION is required`,
		},
		{
			name: "invalid_config_github_app_id",
			env: map[string]string{
				"BUILD_LOCATION": "build-location",
			},
			expErr: `GITHUB_APP_ID is required`,
		},
		{
			name: "invalid_config_webhook_key_mount_path",
			env: map[string]string{
				"BUILD_LOCATION": "build-location",
				"GITHUB_APP_ID":  "github-app-id",
			},
			expErr: `WEBHOOK_KEY_MOUNT_PATH is required`,
		},
		{
			name: "invalid_config_webhook_key_name",
			env: map[string]string{
				"BUILD_LOCATION":         "build-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
			},
			expErr: `WEBHOOK_KEY_NAME is required`,
		},
		{
			name: "invalid_config_kms_app_private_key_id",
			env: map[string]string{
				"BUILD_LOCATION":         "build-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
			},
			expErr: `KMS_APP_PRIVATE_KEY_ID is required`,
		},
		{
			name: "invalid_config_project_id",
			env: map[string]string{
				"BUILD_LOCATION":         "build-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
			},
			expErr: `PROJECT_ID is required`,
		},
		{
			name: "invalid_runner_repository_id",
			env: map[string]string{
				"BUILD_LOCATION":         "build-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
				"PROJECT_ID":             "project-id",
			},
			expErr: `RUNNER_REPOSITORY_ID is required`,
		},
		{
			name: "happy_path",
			env: map[string]string{
				"BUILD_LOCATION":         "build-location",
				"GITHUB_APP_ID":          "github-app-id",
				"WEBHOOK_KEY_MOUNT_PATH": "github-webhook-key-mount-path",
				"WEBHOOK_KEY_NAME":       "key-name",
				"KMS_APP_PRIVATE_KEY_ID": "kms-app-private-key-id",
				"PROJECT_ID":             "project-id",
				"RUNNER_REPOSITORY_ID":   "runner-repo-id",
			},
			fileMock: &webhook.MockFileReader{
				ReadFileMock: &webhook.ReadFileResErr{
					Res: []byte("secret-value"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, done := context.WithCancel(ctx)
			defer done()

			var cmd WebhookServerCommand
			cmd.testFlagSetOpts = []cli.Option{cli.WithLookupEnv(envconfig.MultiLookuper(
				envconfig.MapLookuper(tc.env),
				envconfig.MapLookuper(map[string]string{
					// Make the test choose a random port.
					"PORT": "0",
				}),
			).Lookup)}

			// Provide mock implementation of dependencies
			cmd.testOSFileReaderOverride = tc.fileMock
			cmd.testCloudBuildClientOverride = &webhook.CloudBuild{}
			cmd.testKMSClientOverride = &webhook.MockKMSClient{
				CreateSignerMock: &webhook.CreateSignerRes{Res: &gcpkms.Signer{}},
			}

			_, _, _ = cmd.Pipe()

			srv, mux, err := cmd.RunUnstarted(ctx, tc.args)
			if diff := testutil.DiffErrString(err, tc.expErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			serverCtx, serverDone := context.WithCancel(ctx)
			defer serverDone()
			go func() {
				if err := srv.StartHTTPHandler(serverCtx, mux); err != nil {
					t.Error(err)
				}
			}()

			client := &http.Client{
				Timeout: 5 * time.Second,
			}

			uri := "http://" + srv.Addr() + "/healthz"
			req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if got, want := resp.StatusCode, http.StatusOK; got != want {
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				t.Errorf("expected status code %d to be %d: %s", got, want, string(b))
			}
		})
	}
}
