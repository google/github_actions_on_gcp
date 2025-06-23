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
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"golang.org/x/oauth2"

	"github.com/google/go-github/v69/github"
)

func (s *Server) GenerateRepoJITConfig(ctx context.Context, installationID int64, org, repo, runnerName string) (*github.JITRunnerConfig, *apiResponse) {
	return s.generateJITConfig(ctx, installationID, org, &repo, runnerName)
}

func (s *Server) GenerateOrgJITConfig(ctx context.Context, installationID int64, org, runnerName string) (*github.JITRunnerConfig, *apiResponse) {
	return s.generateJITConfig(ctx, installationID, org, nil, runnerName)
}

func (s *Server) generateJITConfig(ctx context.Context, installationID int64, org string, repo *string, runnerName string) (*github.JITRunnerConfig, *apiResponse) {
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

	// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
	// guaranteed that particular job will run on this specific runner.
	// Note that even though event.WorkflowJob.RunID is used for a dynamic string, it's not
	// guaranteed that particular job will run on this specific runner.
	jitRequest := &github.GenerateJITConfigRequest{
		Name:          runnerName,
		RunnerGroupID: 1,
		Labels:        []string{defaultRunnerLabel, "Linux", "X64"},
	}

	var jitConfig *github.JITRunnerConfig

	if repo != nil {
		jitConfig, _, err = gh.Actions.GenerateRepoJITConfig(ctx, org, *repo, jitRequest)
	} else {
		jitConfig, _, err = gh.Actions.GenerateOrgJITConfig(ctx, org, jitRequest)
	}

	if err != nil {
		return nil, &apiResponse{http.StatusInternalServerError, "failed to generate jitconfig", err}
	}
	return jitConfig, nil
}
