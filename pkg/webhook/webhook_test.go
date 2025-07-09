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
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/abcxyz/pkg/githubauth"

	"github.com/google/go-github/v69/github"
)

const (
	SHA256SignatureHeader = "X-Hub-Signature-256"
	EventTypeHeader       = "X-Github-Event"
	DeliveryIDHeader      = "X-Github-Delivery"
	ContentTypeHeader     = "Content-Type"
	//nolint:gosec // this is a test value
	serverGitHubWebhookSecret = "test-github-webhook-secret"
)

func TestHandleWebhook(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	queuedTime := github.Timestamp{Time: now.Add(-15 * time.Minute)}
	inProgressTime := github.Timestamp{Time: now.Add(-10 * time.Minute)}
	completedTime := github.Timestamp{Time: now.Add(-5 * time.Minute)}
	runID := int64(456)
	jobID := int64(789)
	jobName := "build-job"

	queuedAction := "queued"
	contentType := "application/json"
	payloadType := "workflow_job"

	cases := []struct {
		name                 string
		payloadType          string
		action               string
		runnerLabels         []string
		payloadWebhookSecret string
		contentType          string
		createdAt            *github.Timestamp
		startedAt            *github.Timestamp
		completedAt          *github.Timestamp
		runID                *int64
		jobID                *int64
		jobName              *string
		expStatusCode        int
		expRespBody          string
		expImageTag          string
	}{
		{
			name:                 "Workflow Job Queued - Default Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{defaultRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          runnerStartedMsg,
			expImageTag:          "", // Expect default
		},
		{
			name:                 "Workflow Job Queued - Dynamic Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{defaultRunnerLabel, "pr-123-abc"},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          runnerStartedMsg,
			expImageTag:          "pr-123-abc",
		},
		{
			name:                 "Workflow Job Queued - No Matching Label",
			payloadType:          payloadType,
			action:               queuedAction,
			runnerLabels:         []string{"other-label"}, // No defaultRunnerLabel
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            nil,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          fmt.Sprintf("no action taken for labels: %s", []string{"other-label"}),
			expImageTag:          "", // No build created
		},
		{
			name:                 "Workflow Job In Progress",
			payloadType:          payloadType,
			action:               "in_progress",
			runnerLabels:         []string{defaultRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            &inProgressTime,
			completedAt:          nil,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "workflow job in progress event logged",
			expImageTag:          "", // No build created
		},
		{
			name:                 "Workflow Job Completed - Success",
			payloadType:          payloadType,
			action:               "completed",
			runnerLabels:         []string{defaultRunnerLabel},
			payloadWebhookSecret: serverGitHubWebhookSecret,
			contentType:          contentType,
			createdAt:            &queuedTime,
			startedAt:            &inProgressTime,
			completedAt:          &completedTime,
			runID:                &runID,
			jobID:                &jobID,
			jobName:              &jobName,
			expStatusCode:        200,
			expRespBody:          "workflow job completed event logged",
			expImageTag:          "", // No build created
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			orgLogin := "google"
			repoName := "webhook"
			installationID := int64(123)
			event := &github.WorkflowJobEvent{
				Action: &tc.action,
				WorkflowJob: &github.WorkflowJob{
					Labels:      tc.runnerLabels,
					CreatedAt:   tc.createdAt,
					StartedAt:   tc.startedAt,
					CompletedAt: tc.completedAt,
					RunID:       tc.runID,
					ID:          tc.jobID,
					Name:        tc.jobName,
				},
				Installation: &github.Installation{
					ID: &installationID,
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

			mockCloudBuildClient := &MockCloudBuildClient{}

			srv := &Server{
				webhookSecret:  []byte(tc.payloadWebhookSecret),
				appClient:      app,
				cbc:            mockCloudBuildClient,
				ghAPIBaseURL:   fakeGitHub.URL,
				runnerImageTag: "latest",
			}
			srv.handleWebhook().ServeHTTP(resp, req)

			if got, want := resp.Code, tc.expStatusCode; got != want {
				t.Errorf("expected %d to be %d", got, want)
			}

			if got, want := strings.TrimSpace(resp.Body.String()), tc.expRespBody; got != want {
				t.Errorf("expected %q to be %q", got, want)
			}

			if tc.expImageTag != "" {
				if mockCloudBuildClient.createBuildReq == nil {
					t.Fatalf("expected a build to be created, but it was not")
				}
				if got, want := mockCloudBuildClient.createBuildReq.GetBuild().GetSubstitutions()["_IMAGE_TAG"], tc.expImageTag; got != want {
					t.Errorf("expected image tag %q to be %q", got, want)
				}
			}
			if tc.expImageTag == "" && tc.action == "queued" && slices.Contains(tc.runnerLabels, defaultRunnerLabel) {
				if mockCloudBuildClient.createBuildReq == nil {
					t.Fatalf("expected a build to be created, but it was not")
				}
				if got, want := mockCloudBuildClient.createBuildReq.GetBuild().GetSubstitutions()["_IMAGE_TAG"], "latest"; got != want {
					t.Errorf("expected image tag %q to be %q", got, want)
				}
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
