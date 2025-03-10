// Copyright 2025 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/logging"
	"github.com/google/go-github/v69/github"
	"github.com/googleapis/gax-go/v2"
	// "github.com/abcxyz/pkg/renderer"
)

const (
	SHA256SignatureHeader     = "X-Hub-Signature-256"
	EventTypeHeader           = "X-Github-Event"
	DeliveryIDHeader          = "X-Github-Delivery"
	ContentTypeHeader         = "Content-Type"
	serverGitHubWebhookSecret = "test-github-webhook-secret"
)

func TestHandleWebhook(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	cases := []struct {
		name                 string
		payloadType          string
		action               string
		payloadWebhookSecret string
		contentType          string
		expStatusCode        int
		expRespBody          string
	}{
		{
			name:                 "Foo",
			payloadType:          "workflow_job",
			action:               "queued",
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          "application/json",
			expStatusCode:        204,
			expRespBody:          "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			logger := logging.TestLogger(t)

			orgLogin := "google"
			repoName := "webhook"
			installationId := int64(123)
			event := &github.WorkflowJobEvent{
				Action: &tc.action,
				WorkflowJob: &github.WorkflowJob{
					Labels: []string{"self-hosted"},
				},
				Installation: &github.Installation{
					ID: &installationId,
				},
				Org: &github.Organization{
					Login: &orgLogin,
				},
				Repo: &github.Repository{
					Name: &repoName,
				},
			}

			payload, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}

			encodedJitConfig := "Hello"
			jit := &github.JITRunnerConfig{
				EncodedJITConfig: &encodedJitConfig,
			}
			jitPayload, err := json.Marshal(jit)
			if err != nil {
				t.Fatal(err)
			}

			fakeGitHub := func() *httptest.Server {
				mux := http.NewServeMux()
				mux.Handle("GET /app/installations/123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintf(w, `{"access_tokens_url": "http://%s/app/installations/123/access_tokens"}`, r.Host)
				}))
				mux.Handle("POST /app/installations/123/access_tokens", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(201)
					fmt.Fprintf(w, `{"token": "this-is-the-token-from-github"}`)
				}))
				mux.Handle("POST /repos/google/webhook/actions/runners/generate-jitconfig", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(201)
					fmt.Fprintf(w, "%s", string(jitPayload))
				}))

				return httptest.NewServer(mux)
			}()
			t.Cleanup(func() {
				fakeGitHub.Close()
			})

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
			req.Header.Add(DeliveryIDHeader, "delivery-id")
			req.Header.Add(EventTypeHeader, tc.payloadType)
			req.Header.Add(ContentTypeHeader, tc.contentType)
			req.Header.Add(SHA256SignatureHeader, fmt.Sprintf("sha256=%s", createSignature([]byte(tc.payloadWebhookSecret), payload)))

			resp := httptest.NewRecorder()

			rsaPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}

			app, err := githubauth.NewApp("app-id", rsaPrivateKey, githubauth.WithBaseURL(fakeGitHub.URL))
			if err != nil {
				t.Fatal(err)
			}

			// GitHub base URL expects a trailing slash
			baseURL, err := url.Parse(fmt.Sprintf("%s/", fakeGitHub.URL))
			if err != nil {
				t.Fatal(err)
			}

			cloudBuildClientStub := &CloudBuildClientStub{
				runBuildTriggerRet: &cloudbuild.RunBuildTriggerOperation{},
			}

			srv := &Server{
				logger:           logger,
				ctx:              ctx,
				webhookSecret:    []byte(tc.payloadWebhookSecret),
				appClient:        app,
				cloudBuildClient: cloudBuildClientStub,
				baseURL:          baseURL,
			}
			srv.handler(resp, req)

			if got, want := resp.Code, tc.expStatusCode; got != want {
				t.Errorf("expected %d to be %d", got, want)
			}

			if got, want := strings.TrimSpace(resp.Body.String()), tc.expRespBody; got != want {
				t.Errorf("expected %q to be %q", got, want)
			}
		})
	}
}

// createSignature creates a HMAC 256 signature for the test request payload.
func createSignature(key, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

type CloudBuildClientStub struct {
	runBuildTriggerRet *cloudbuild.RunBuildTriggerOperation
	runBuildTriggerErr error
}

func (c *CloudBuildClientStub) RunBuildTrigger(ctx context.Context, req *cloudbuildpb.RunBuildTriggerRequest, opts ...gax.CallOption) (*cloudbuild.RunBuildTriggerOperation, error) {
	if c.runBuildTriggerErr != nil {
		return nil, c.runBuildTriggerErr
	}
	return c.runBuildTriggerRet, nil
}
