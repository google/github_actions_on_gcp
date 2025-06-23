package webhook

import (
	"context"
	"fmt"
	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) GenerateRepoJITConfig(ctx context.Context, installationID int64, org string, repo string, runID int64) (*github.JITRunnerConfig, *apiResponse) {
	return s.generateJITConfig(ctx, installationID, org, &repo, runID)
}

func (s *Server) GenerateOrgJITConfig(ctx context.Context, installationID int64, org string, runID int64) (*github.JITRunnerConfig, *apiResponse) {
	return s.generateJITConfig(ctx, installationID, org, nil, runID)
}

func (s *Server) generateJITConfig(ctx context.Context, installationID int64, org string, repo *string, runID int64) (*github.JITRunnerConfig, *apiResponse) {
	installation, err := s.appClient.InstallationForID(ctx, strconv.FormatInt(installationID, 10))
	if err != nil {
		return nil, &apiResponse{http.StatusInternalServerError, "failed to setup installation client", err}
	}

	httpClient := oauth2.NewClient(ctx, (*installation).AllReposOAuth2TokenSource(ctx, map[string]string{
		"administration": "write",
	}))

	gh := github.NewClient(httpClient)
	baseURL, err := url.Parse(fmt.Sprintf("%s/", s.ghAPIBaseURL))
	if err != nil {
		return nil, &apiResponse{http.StatusInternalServerError, "failed to set github base URL", err}
	}
	gh.BaseURL = baseURL
	gh.UploadURL = baseURL

	var jitConfig *github.JITRunnerConfig

	if repo != nil {
		// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
		// guaranteed that particular job will run on this specific runner.
		// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
		// guaranteed that particular job will run on this specific runner.
		jitConfig, _, err = gh.Actions.GenerateRepoJITConfig(ctx, org, *repo, &github.GenerateJITConfigRequest{
			Name:          fmt.Sprintf("GCP-%d", runID),
			RunnerGroupID: 1,
			Labels:        []string{defaultRunnerLabel, "Linux", "X64"},
		})
	} else {
		jitConfig, _, err = gh.Actions.GenerateOrgJITConfig(ctx, org, &github.GenerateJITConfigRequest{
			Name:          fmt.Sprintf("GCP-%d", runID),
			RunnerGroupID: 1,
			Labels:        []string{defaultRunnerLabel, "Linux", "X64"},
		})
	}

	if err != nil {
		return nil, &apiResponse{http.StatusInternalServerError, "failed to generate jitconfig", err}
	}
	return jitConfig, nil
}
