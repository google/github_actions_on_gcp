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

package main

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	kms "cloud.google.com/go/kms/apiv1"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/logging"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"
	"golang.org/x/oauth2"

	"github.com/google/go-github/v69/github"
	"github.com/googleapis/gax-go/v2"
)

type server struct {
	logger           *slog.Logger
	webhookSecret    []byte
	appClient        *githubauth.App
	cloudBuildClient cloudBuildClient
	baseURL          *url.URL
	projectID        string
	location         string
}

type cloudBuildClient interface {
	CreateBuild(ctx context.Context, req *cloudbuildpb.CreateBuildRequest, opts ...gax.CallOption) (*cloudbuild.CreateBuildOperation, error)
}

func main() {
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer done()

	logger := logging.NewFromEnv("WEBHOOK_RECEIVER_")
	ctx = logging.WithLogger(ctx, logger)

	if err := realMain(ctx, logger); err != nil {
		done()
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func realMain(ctx context.Context, logger *slog.Logger) error {
	server, err := newServer(ctx, logger)
	if err != nil {
		return err
	}
	logger.InfoContext(ctx, "starting server")
	http.HandleFunc("/", server.handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		logger.InfoContext(ctx, "defaulting to port", "port", port)
	}
	logger.InfoContext(ctx, "starting server on port", "port", port)
	httpServer := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: 3 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil {
		logger.ErrorContext(ctx, "http server error", "error", err)
		return fmt.Errorf("http server failed: %w", err)
	}
	return nil
}

func newServer(ctx context.Context, logger *slog.Logger) (*server, error) {
	webhookKeyPath := os.Getenv("WEBHOOK_KEY_MOUNT_PATH")
	webhookSecret, err := os.ReadFile(webhookKeyPath)
	if err != nil {
		logger.ErrorContext(ctx, "failed to read webhook secret", "error", err)
		return nil, fmt.Errorf("failed to read webhook secret: %w", err)
	}

	kmsClient, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create kms client", "error", err)
		return nil, fmt.Errorf("failed to create kms client: %w", err)
	}

	keyID := os.Getenv("KMS_APP_PRIVATE_KEY_ID")
	signer, err := gcpkms.NewSigner(ctx, kmsClient, keyID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create app signer", "error", err)
		return nil, fmt.Errorf("failed to create app signer: %w", err)
	}

	appID := os.Getenv("GITHUB_APP_ID")
	appClient, err := githubauth.NewApp(appID, signer)
	if err != nil {
		logger.ErrorContext(ctx, "failed to setup app client", "error", err)
		return nil, fmt.Errorf("failed to setup app client: %w", err)
	}

	cloudBuildClient, err := cloudbuild.NewClient(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create cloudbuild client", "error", err)
		return nil, fmt.Errorf("failed to create cloudbuild client: %w", err)
	}

	return &server{
		logger:           logger,
		webhookSecret:    webhookSecret,
		appClient:        appClient,
		cloudBuildClient: cloudBuildClient,
		projectID:        os.Getenv("PROJECT_ID"),
		location:         os.Getenv("BUILD_LOCATION"),
	}, nil
}

func (s *server) handler(resp http.ResponseWriter, req *http.Request) {
	ctx := logging.WithLogger(req.Context(), s.logger)
	payload, err := github.ValidatePayload(req, s.webhookSecret)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to validate payload", "error", err)
		fmt.Fprint(resp, html.EscapeString("failed to validate payload"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(req), payload)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to parse webhook", "error", err)
		fmt.Fprint(resp, html.EscapeString("failed to parse webhook"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch event := event.(type) {
	case *github.WorkflowJobEvent:
		if event.Action == nil || *event.Action != "queued" {
			s.logger.InfoContext(ctx, "no action taken for action type", "action", *event.Action)
			resp.WriteHeader(http.StatusNoContent)
			return
		}

		if !slices.Contains(event.WorkflowJob.Labels, "self-hosted") {
			s.logger.InfoContext(ctx, "no action taken for labels", "labels", event.WorkflowJob.Labels)
			resp.WriteHeader(http.StatusNoContent)
			return
		}

		installation, err := s.appClient.InstallationForID(ctx, strconv.FormatInt(*event.Installation.ID, 10))
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to setup installation client", "error", err)
			fmt.Fprint(resp, html.EscapeString("failed to setup installation client"))
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		httpClient := oauth2.NewClient(ctx, (*installation).AllReposOAuth2TokenSource(ctx, map[string]string{
			"administration": "write",
		}))
		gh := github.NewClient(httpClient)
		if s.baseURL != nil {
			gh.BaseURL = s.baseURL
		}

		// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
		// guaranteed that particular job will run on this specific runner.
		jitconfig, _, err := gh.Actions.GenerateRepoJITConfig(ctx, *event.Org.Login, *event.Repo.Name, &github.GenerateJITConfigRequest{Name: fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID), RunnerGroupID: 1, Labels: []string{"self-hosted", "Linux", "X64"}})
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to generate jitconfig", "error", err)
			fmt.Fprint(resp, html.EscapeString("failed to generate jitconfig"))
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		build := &cloudbuildpb.Build{
			Steps: []*cloudbuildpb.BuildStep{
				{
					Id:         "run",
					Name:       "gcr.io/cloud-builders/docker",
					Entrypoint: "bash",
					// privileged and security-opts are needed to run Docker-in-Docker
					// https://rootlesscontaine.rs/getting-started/common/apparmor/
					Args: []string{
						"-c",
						fmt.Sprintf("docker run --privileged --security-opt seccomp=unconfined --security-opt apparmor=unconfined -eENCODED_JIT_CONFIG=$_ENCODED_JIT_CONFIG $LOCATION-docker.pkg.dev/%s/$_CONTAINER_REPOSITORY/github-actions-runner:latest", s.projectID),
					},
				},
			},
			Options: &cloudbuildpb.BuildOptions{
				Logging: cloudbuildpb.BuildOptions_CLOUD_LOGGING_ONLY,
			},
			Substitutions: map[string]string{
				"_ENCODED_JIT_CONFIG": *jitconfig.EncodedJITConfig,
			},
		}

		buildReq := &cloudbuildpb.CreateBuildRequest{
			Parent:    fmt.Sprintf("projects/%s/locations/%s", s.projectID, s.location),
			ProjectId: s.projectID,
			Build:     build,
		}

		_, err = s.cloudBuildClient.CreateBuild(ctx, buildReq)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to run build", "error", err)
			fmt.Fprint(resp, html.EscapeString("failed to run build"))
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		s.logger.InfoContext(ctx, "started runner", "runner_id", fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID))
		resp.WriteHeader(http.StatusNoContent)
	}
}
