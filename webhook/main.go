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
	"os"
	"os/signal"
	"strconv"
	"syscall"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	kms "cloud.google.com/go/kms/apiv1"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/logging"
	"github.com/google/go-github/v69/github"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"
	"golang.org/x/oauth2"
)

// gcloud run deploy webhook-go --region=us-west1 --source . --update-secrets=/etc/secrets/webhook/key=${KEY_NAME}:latest --allow-unauthenticated --set-env-vars=APP_ID=${APP_ID},TRIGGER_ID=${TRIGGER_ID},PROJECT_ID=${PROJECT_ID},KEY_ID=${KEY_ID},TRIGGER_NAME=${TRIGGER_NAME},LOCATION=${LOCATION}

type Server struct {
	logger           *slog.Logger
	ctx              context.Context
	webhookSecret    []byte
	kmsClient        *kms.KeyManagementClient
	signer           *gcpkms.Signer
	appClient        *githubauth.App
	cloudBuildClient *cloudbuild.Client
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
	webhookSecret, err := getWebhookSecret(ctx, logger)
	if err != nil {
		return err
	}

	kmsClient, err := getKeyManagementClient(ctx, logger)
	if err != nil {
		return err
	}

	signer, err := getSigner(ctx, logger, kmsClient)
	if err != nil {
		return err
	}

	appClient, err := getAppClient(ctx, logger, signer)
	if err != nil {
		return err
	}

	cloudBuildClient, err := getCloudBuildClient(ctx, logger)
	if err != nil {
		return err
	}

	server := &Server{
		logger:           logger,
		ctx:              ctx,
		webhookSecret:    webhookSecret,
		kmsClient:        kmsClient,
		signer:           signer,
		appClient:        appClient,
		cloudBuildClient: cloudBuildClient,
	}

	logger.InfoContext(ctx, "starting server")
	http.HandleFunc("/", server.handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		logger.InfoContext(ctx, "defaulting to port", "port", port)
	}
	logger.InfoContext(ctx, "starting server on port", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.ErrorContext(ctx, "http server error", "error", err)
		return err
	}
	return nil
}

func getWebhookSecret(ctx context.Context, logger *slog.Logger) ([]byte, error) {
	webhookSecret, err := os.ReadFile("/etc/secrets/webhook/key")
	if err != nil {
		logger.ErrorContext(ctx, "failed to read webhook secret", "error", err)
		return nil, err
	}
	return webhookSecret, nil
}

func getKeyManagementClient(ctx context.Context, logger *slog.Logger) (*kms.KeyManagementClient, error) {
	kmsClient, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create kms client", "error", err)
		return nil, err
	}
	return kmsClient, nil
}

func getSigner(ctx context.Context, logger *slog.Logger, kmsClient *kms.KeyManagementClient) (*gcpkms.Signer, error) {
	keyId := os.Getenv("KEY_ID")
	signer, err := gcpkms.NewSigner(ctx, kmsClient, keyId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create app signer", "error", err)
		return nil, err
	}
	return signer, nil
}

func getAppClient(ctx context.Context, logger *slog.Logger, signer *gcpkms.Signer) (*githubauth.App, error) {
	appId := os.Getenv("APP_ID")
	app, err := githubauth.NewApp(appId, signer)
	if err != nil {
		logger.ErrorContext(ctx, "failed to setup app client", "error", err)
		return nil, err
	}
	return app, nil
}

func getCloudBuildClient(ctx context.Context, logger *slog.Logger) (*cloudbuild.Client, error) {
	client, err := cloudbuild.NewClient(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create cloudbuild client", "error", err)
		return nil, err
	}
	return client, nil
}

func (s *Server) handler(resp http.ResponseWriter, req *http.Request) {
	payload, err := github.ValidatePayload(req, s.webhookSecret)
	if err != nil {
		s.logger.ErrorContext(s.ctx, "failed to validate payload", "error", err)
		fmt.Fprint(resp, html.EscapeString("failed to validate payload"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(req), payload)
	if err != nil {
		s.logger.ErrorContext(s.ctx, "failed to parse webhook", "error", err)
		fmt.Fprint(resp, html.EscapeString("failed to parse webhook"))
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch event := event.(type) {
	case *github.WorkflowJobEvent:
		if event.Action == nil || *event.Action != "queued" {
			s.logger.InfoContext(s.ctx, "no action taken for action", "action", *event.Action)
			resp.WriteHeader(http.StatusNoContent)
			return
		}

		installation, err := s.appClient.InstallationForID(s.ctx, strconv.FormatInt(*event.Installation.ID, 10))
		if err != nil {
			s.logger.ErrorContext(s.ctx, "failed to setup installation client", "error", err)
			fmt.Fprint(resp, html.EscapeString("failed to setup installation client"))
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		httpClient := oauth2.NewClient(s.ctx, installation.AllReposOAuth2TokenSource(s.ctx, map[string]string{
			"administration": "write",
		}))
		gh := github.NewClient(httpClient)

		// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
		// guaranteed that particular job will run on this specific runner.
		jitconfig, _, err := gh.Actions.GenerateRepoJITConfig(s.ctx, *event.Org.Login, *event.Repo.Name, &github.GenerateJITConfigRequest{Name: fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID), RunnerGroupID: 1, Labels: []string{"self-hosted", "Linux", "X64"}})
		if err != nil {
			s.logger.ErrorContext(s.ctx, "failed to generate jitconfig", "error", err)
			fmt.Fprint(resp, html.EscapeString("failed to generate jitconfig"))
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		projectId := os.Getenv("PROJECT_ID")
		location := os.Getenv("LOCATION")
		triggerName := os.Getenv("TRIGGER_NAME")
		triggerId := os.Getenv("TRIGGER_ID")
		buildReq := &cloudbuildpb.RunBuildTriggerRequest{
			Name:      fmt.Sprintf("projects/%s/locations/%s/triggers/%s", projectId, location, triggerName),
			ProjectId: projectId,
			TriggerId: triggerId,
			Source: &cloudbuildpb.RepoSource{
				Substitutions: map[string]string{
					"_ENCODED_JIT_CONFIG": *jitconfig.EncodedJITConfig,
				},
			},
		}

		_, err = s.cloudBuildClient.RunBuildTrigger(s.ctx, buildReq)
		if err != nil {
			s.logger.ErrorContext(s.ctx, "failed to run build", "error", err)
			fmt.Fprint(resp, html.EscapeString("failed to run build"))
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp.WriteHeader(http.StatusNoContent)
	}
}
