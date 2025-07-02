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
	"time"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/abcxyz/pkg/logging"

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
		// Check for nil action first to avoid nil pointer dereference
		if event.Action == nil {
			logger.InfoContext(ctx, "no action taken for nil action type")
			return &apiResponse{http.StatusOK, "no action taken for nil action type", nil}
		}

		// Common attributes to always include for WorkflowJobEvent
		var jobID string
		if event.WorkflowJob != nil && event.WorkflowJob.ID != nil {
			jobID = fmt.Sprintf("%d", *event.WorkflowJob.ID)
		}

		runnerID := fmt.Sprintf("GCP-%s", jobID)

		// Base log fields that will be common to most WorkflowJob logs
		baseLogFields := []any{
			"action_event_name", *event.Action,
			"gh_run_id", *event.WorkflowJob.RunID,
			"gh_job_id", *event.WorkflowJob.ID,
			"gh_job_name", event.WorkflowJob.Name,
			"job_id", jobID,
			"runner_id", runnerID,
		}

		// Add all available timestamps to base log fields (they might be nil depending on event action)
		if event.WorkflowJob.CreatedAt != nil {
			baseLogFields = append(baseLogFields, "created_at", getTimeString(event.WorkflowJob.CreatedAt))
		}
		if event.WorkflowJob.StartedAt != nil {
			baseLogFields = append(baseLogFields, "started_at", getTimeString(event.WorkflowJob.StartedAt))
		}
		if event.WorkflowJob.CompletedAt != nil {
			baseLogFields = append(baseLogFields, "completed_at", getTimeString(event.WorkflowJob.CompletedAt))
		}

		switch *event.Action {
		case "queued":
			logger.InfoContext(ctx, "Workflow job queued", baseLogFields...)

			if !slices.Contains(event.WorkflowJob.Labels, defaultRunnerLabel) {
				logger.InfoContext(ctx, "no action taken for labels", append(baseLogFields, "labels", event.WorkflowJob.Labels)...)
				return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for labels: %s", event.WorkflowJob.Labels), nil}
			}

			if event.Installation == nil || event.Installation.ID == nil || event.Org == nil || event.Org.Login == nil || event.Repo == nil || event.Repo.Name == nil {
				err := fmt.Errorf("event is missing required fields (installation, org, or repo)")
				logger.ErrorContext(ctx, "cannot generate JIT config due to missing event data", append(baseLogFields, "error", err)...)
				return &apiResponse{http.StatusBadRequest, "unexpected event payload struture", err}
			}

			jitConfig, errResponse := s.GenerateRepoJITConfig(ctx, *event.Installation.ID, *event.Org.Login, *event.Repo.Name, runnerID)
			if errResponse != nil {
				logger.ErrorContext(ctx, "failed to generate JIT config", append(baseLogFields, "error", errResponse.Error, "response_message", errResponse.Message)...)
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
				logger.ErrorContext(ctx, "failed to run Cloud Build for runner", append(baseLogFields, "error", err)...)
				return &apiResponse{http.StatusInternalServerError, "failed to run build", err}
			}

			logger.InfoContext(ctx, runnerStartedMsg, baseLogFields...)
			return &apiResponse{http.StatusOK, runnerStartedMsg, nil}

		case "in_progress":
			// Calculate and log "queued duration"
			logFields := append([]any{}, baseLogFields...) // Create a mutable copy

			if event.WorkflowJob.CreatedAt != nil && event.WorkflowJob.StartedAt != nil {
				queuedDuration := event.WorkflowJob.StartedAt.Time.Sub(event.WorkflowJob.CreatedAt.Time)

				logFields = append(logFields, "duration_queued_seconds", queuedDuration.Seconds())
			}

			logger.InfoContext(ctx, "Workflow job in progress", logFields...)
			return &apiResponse{http.StatusOK, "workflow job in progress event logged", nil}

		case "completed":
			// Calculate and log "in progress duration"
			logFields := append([]any{}, baseLogFields...) // Create a mutable copy

			if event.WorkflowJob.Conclusion != nil {
				logFields = append(logFields, "conclusion", *event.WorkflowJob.Conclusion)
			}

			if event.WorkflowJob.StartedAt != nil && event.WorkflowJob.CompletedAt != nil {
				inProgressDuration := event.WorkflowJob.CompletedAt.Time.Sub(event.WorkflowJob.StartedAt.Time)
				logFields = append(logFields, "duration_in_progress_seconds", inProgressDuration.Seconds())
			}

			// Optional: Also log total duration from creation to completion here
			if event.WorkflowJob.CreatedAt != nil && event.WorkflowJob.CompletedAt != nil {
				totalDuration := event.WorkflowJob.CompletedAt.Time.Sub(event.WorkflowJob.CreatedAt.Time)
				logFields = append(logFields, "duration_total_seconds", totalDuration.Seconds())
			}

			logger.InfoContext(ctx, "Workflow job completed", logFields...)
			return &apiResponse{http.StatusOK, "workflow job completed event logged", nil}

		default:
			// Log other unhandled workflow job actions
			logger.InfoContext(ctx, "no action taken for unhandled workflow job action type", append(baseLogFields, "action", *event.Action)...)
			return &apiResponse{http.StatusOK, fmt.Sprintf("no action taken for action type: %q", *event.Action), nil}
		}

	default:
		// Log other unhandled webhook event types
		logger.ErrorContext(ctx, "Received unhandled event type",
			"event_type", fmt.Sprintf("%T", event),
			"payload", string(payload))
		return &apiResponse{http.StatusInternalServerError, "unexpected event type dispatched from webhook", fmt.Errorf("event type: %T", event)}
	}
}

// getTimeString is a helper function to format a *github.Timestamp pointer into an ISO 8601 string.
// It safely handles nil *github.Timestamp pointers.
// It returns "N/A" if the time pointer is nil.
func getTimeString(ghTime *github.Timestamp) string {
	if ghTime == nil { // ONLY check if the *pointer* itself is nil
		return "N/A"
	}
	// ghTime.Time is a time.Time struct, which is never nil.
	return ghTime.Time.Format(time.RFC3339Nano)
}
