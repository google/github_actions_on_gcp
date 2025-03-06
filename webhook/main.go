package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	kms "cloud.google.com/go/kms/apiv1"
	"github.com/abcxyz/pkg/githubauth"
	"github.com/google/go-github/v69/github"
	"github.com/sethvargo/go-gcpkms/pkg/gcpkms"
	"golang.org/x/oauth2"
)

// gcloud run deploy webhook-go --region=us-west1 --source . --update-secrets=/etc/secrets/webhook/key=${KEY_NAME}:latest --allow-unauthenticated --set-env-vars=APP_ID=${APP_ID},TRIGGER_ID=${TRIGGER_ID},PROJECT_ID=${PROJECT_ID},KEY_ID=${KEY_ID}

func main() {
	log.Print("starting server")
	http.HandleFunc("/", handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("defaulting to port %s", port)
	}
	log.Printf("starting server on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handler(resp http.ResponseWriter, req *http.Request) {
	ctx := context.Background()

	webhookSecret, err := os.ReadFile("/etc/secrets/webhook/key")
	if err != nil {
		log.Printf("failed to read webhook secret: %v", err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	payload, err := github.ValidatePayload(req, webhookSecret)
	if err != nil {
		log.Printf("failed to validate payload: %v", err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(req), payload)
	if err != nil {
		log.Printf("failed to parse webhook: %v", err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch event := event.(type) {
	case *github.WorkflowJobEvent:
		if event.Action == nil || *event.Action != "queued" {
			log.Printf("no action taken for %v", *event.Action)
			resp.WriteHeader(http.StatusNoContent)
			return
		}

		kmsClient, err := kms.NewKeyManagementClient(ctx)
		if err != nil {
			log.Printf("failed to create kms client: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}
		keyId := os.Getenv("KEY_ID")
		signer, err := gcpkms.NewSigner(ctx, kmsClient, keyId)
		if err != nil {
			log.Printf("failed to create app signer: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}
		appId := os.Getenv("APP_ID")
		app, err := githubauth.NewApp(appId, signer)
		if err != nil {
			log.Printf("failed to setup client: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}
		installation, err := app.InstallationForID(ctx, strconv.FormatInt(*event.Installation.ID, 10))
		if err != nil {
			log.Printf("failed to setup client: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		httpClient := oauth2.NewClient(ctx, installation.AllReposOAuth2TokenSource(ctx, map[string]string{
			"administration": "write",
		}))
		gh := github.NewClient(httpClient)

		// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
		// guaranteed that particular job will run on this specific runner.
		jitconfig, _, err := gh.Actions.GenerateRepoJITConfig(ctx, *event.Org.Login, *event.Repo.Name, &github.GenerateJITConfigRequest{Name: fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID), RunnerGroupID: 1, Labels: []string{"self-hosted", "Linux", "X64"}})
		if err != nil {
			log.Printf("failed to generate jitconfig: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		client, err := cloudbuild.NewClient(ctx)
		if err != nil {
			log.Printf("failed to create cloudbuild client: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		projectId := os.Getenv("PROJECT_ID")
		triggerId := os.Getenv("TRIGGER_ID")
		buildReq := &cloudbuildpb.RunBuildTriggerRequest{
			Name:      fmt.Sprintf("projects/%s/locations/us-west1/triggers/github-actions-runner", projectId),
			ProjectId: projectId,
			TriggerId: triggerId,
			Source: &cloudbuildpb.RepoSource{
				Substitutions: map[string]string{
					"_ENCODED_JIT_CONFIG": *jitconfig.EncodedJITConfig,
				},
			},
		}

		_, err = client.RunBuildTrigger(ctx, buildReq)
		if err != nil {
			log.Printf("failed to run build: %v", err)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp.WriteHeader(http.StatusNoContent)
	}
}
