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
	"fmt"
	"html"
	"net/http"
	"slices"

	"github.com/abcxyz/pkg/logging"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"

	"github.com/google/go-github/v69/github"
)

var (
	defaultRunnerLabel = "self-hosted"
	runnerStartedMsg   = "runner started"
)

// apiResponse is a structure that contains a http status code,
// a string response message and any error that might have occurred
// in the processing of a request.
type apiResponse struct {
	Code    int
	Message string
	Error   error
}

func (s *Server) handleWebhook() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := logging.FromContext(ctx)

		resp := s.processRequest(r)
		if resp.Error != nil {
			logger.ErrorContext(ctx, "error processing request",
				"error", resp.Error,
				"code", resp.Code,
				"body", resp.Message)
		}

		w.WriteHeader(resp.Code)
		fmt.Fprint(w, html.EscapeString(resp.Message))
	})
}

func (s *Server) processRequest(r *http.Request) *apiResponse {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	payload, err := github.ValidatePayload(r, s.webhookSecret)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, "failed to validate payload", err}
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		return &apiResponse{http.StatusInternalServerError, "failed to parse webhook", err}
	}

	switch event := event.(type) {
	case *github.WorkflowJobEvent:
		if event.Action == nil || *event.Action != "queued" {
			return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for action type: %q", *event.Action), nil}
		}

		if !slices.Contains(event.WorkflowJob.Labels, defaultRunnerLabel) {
			logger.InfoContext(ctx, "no action taken for labels", "labels", event.WorkflowJob.Labels)
			return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for labels: %s", event.WorkflowJob.Labels), nil}
		}

		jitConfig, errResponse := s.GenerateRepoJITConfig(ctx, *event.Installation.ID, *event.Org.Login, *event.Repo.Name, fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID))
		if errResponse != nil {
			return errResponse
		}

		build := &cloudbuildpb.Build{
			ServiceAccount: s.runnerServiceAccount,
			Steps: []*cloudbuildpb.BuildStep{
				{
					Id:         "run",
					Name:       "gcr.io/cloud-builders/docker",
					Entrypoint: "bash",
					Args: []string{
						"-c",
						// privileged and security-opts are needed to run Docker-in-Docker
						// https://rootlesscontaine.rs/getting-started/common/apparmor/
						"docker run --privileged --security-opt seccomp=unconfined --security-opt apparmor=unconfined -e ENCODED_JIT_CONFIG=$_ENCODED_JIT_CONFIG $_REPOSITORY_ID/$_IMAGE_NAME:$_IMAGE_TAG",
					},
				},
			},
			Options: &cloudbuildpb.BuildOptions{
				Logging: cloudbuildpb.BuildOptions_CLOUD_LOGGING_ONLY,
			},
			Substitutions: map[string]string{
				"_ENCODED_JIT_CONFIG": *jitConfig.EncodedJITConfig,
				"_REPOSITORY_ID":      s.runnerRepositoryID,
				"_IMAGE_NAME":         s.runnerImageName,
				"_IMAGE_TAG":          s.runnerImageTag,
			},
		}

		if s.runnerWorkerPoolID != "" {
			build.Options.Pool = &cloudbuildpb.BuildOptions_PoolOption{
				Name: s.runnerWorkerPoolID,
			}
		}

		buildReq := &cloudbuildpb.CreateBuildRequest{
			Parent:    fmt.Sprintf("projects/%s/locations/%s", s.runnerProjectID, s.runnerLocation),
			ProjectId: s.runnerProjectID,
			Build:     build,
		}

		if err := s.cbc.CreateBuild(ctx, buildReq); err != nil {
			return &apiResponse{http.StatusInternalServerError, "failed to run build", err}
		}

		logger.InfoContext(ctx, runnerStartedMsg, "runner_id", fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID))
		return &apiResponse{http.StatusOK, runnerStartedMsg, nil}
	}

	return &apiResponse{http.StatusInternalServerError, "unexpected event type dispatched from webhook", fmt.Errorf("event type: %s", event)}
}
