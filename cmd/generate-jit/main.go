// Copyright 2025 The Authors (see AUTHORS file)
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

package main

import (
	"context"
	"crypto/rsa"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/abcxyz/pkg/githubauth"
	"github.com/abcxyz/pkg/logging"
	"github.com/google/go-github/v69/github"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"golang.org/x/oauth2"
)

var (
	appID          = flag.String("app-id", "", "GitHub App's ID")
	privateKeyPath = flag.String("private-key", "", "Path to your private key PEM file")
	orgName        = flag.String("org", "", "GitHub organization name")
	runnerName     = flag.String("runner-name", "my-gcp-runner", "Name for the new runner")
	runnerLabels   = flag.String("runner-labels", "self-hosted,Linux,X64", "Comma-separated labels for the runner")
	runnerGroupID  = flag.Int64("runner-group-id", 1, "The ID of the runner group to assign the new runner to")
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer done()

	logger := logging.NewFromEnv("")
	ctx = logging.WithLogger(ctx, logger)

	if err := realMain(ctx); err != nil {
		done()
		logger.ErrorContext(ctx, "process exited with error", "error", err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context) error {
	flag.Parse()

	privateKeyBytes, err := os.ReadFile(*privateKeyPath)
	if err != nil {
		return fmt.Errorf("error reading private key file: %w", err)
	}

	key, err := jwk.ParseKey(privateKeyBytes, jwk.WithPEM(true))
	if err != nil {
		return fmt.Errorf("error parsing private key with jwx: %w", err)
	}

	var privateKey rsa.PrivateKey
	if err := key.Raw(&privateKey); err != nil {
		return fmt.Errorf("failed to get raw rsa private key from jwk: %w", err)
	}

	appAuth, err := githubauth.NewApp(*appID, &privateKey)
	if err != nil {
		return fmt.Errorf("failed to create github app auth: %w", err)
	}

	installation, err := appAuth.InstallationForOrg(ctx, *orgName)
	if err != nil {
		return fmt.Errorf("failed to find installation for org %q: %w", *orgName, err)
	}

	tokenSource := installation.AllReposOAuth2TokenSource(ctx,
		map[string]string{
			"organization_self_hosted_runners": "write",
		},
	)

	httpClient := oauth2.NewClient(ctx, tokenSource)
	gh := github.NewClient(httpClient)

	jitconfig, _, err := gh.Actions.GenerateOrgJITConfig(ctx, *orgName, &github.GenerateJITConfigRequest{
		Name:          *runnerName,
		RunnerGroupID: *runnerGroupID,
		Labels:        strings.Split(*runnerLabels, ","),
	})
	if err != nil {
		return fmt.Errorf("failed to generate jitconfig: %w", err)
	}

	fmt.Print(*jitconfig.EncodedJITConfig)

	return nil
}
