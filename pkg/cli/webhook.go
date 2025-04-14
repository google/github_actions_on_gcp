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

package cli

import (
	"context"
	"fmt"
	"net/http"

	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
	"github.com/abcxyz/pkg/serving"
	"google.golang.org/api/option"

	"github.com/google/github_actions_on_gcp/pkg/version"
	"github.com/google/github_actions_on_gcp/pkg/webhook"
)

var _ cli.Command = (*WebhookServerCommand)(nil)

type WebhookServerCommand struct {
	cli.BaseCommand

	cfg *webhook.Config

	// only used for testing
	testFlagSetOpts []cli.Option

	// only used for testing
	testCloudBuildClientOverride webhook.CloudBuildClient

	// only used for testing
	testKMSClientOverride webhook.KeyManagementClient

	// only used for testing
	testOSFileReaderOverride webhook.FileReader
}

func (c *WebhookServerCommand) Desc() string {
	return `Start a webhook server for github-actions-on-gcp`
}

func (c *WebhookServerCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options]
  Start a webhook server for github-actions-on-gcp.
`
}

func (c *WebhookServerCommand) Flags() *cli.FlagSet {
	c.cfg = &webhook.Config{}
	set := cli.NewFlagSet(c.testFlagSetOpts...)
	return c.cfg.ToFlags(set)
}

func (c *WebhookServerCommand) Run(ctx context.Context, args []string) error {
	server, mux, err := c.RunUnstarted(ctx, args)
	if err != nil {
		return err
	}

	return server.StartHTTPHandler(ctx, mux)
}

func (c *WebhookServerCommand) RunUnstarted(ctx context.Context, args []string) (*serving.Server, http.Handler, error) {
	f := c.Flags()
	if err := f.Parse(args); err != nil {
		return nil, nil, fmt.Errorf("failed to parse flags: %w", err)
	}
	args = f.Args()
	if len(args) > 0 {
		return nil, nil, fmt.Errorf("unexpected arguments: %q", args)
	}

	logger := logging.FromContext(ctx)
	logger.DebugContext(ctx, "server starting",
		"name", version.Name,
		"commit", version.Commit,
		"version", version.Version)

	h, err := renderer.New(ctx, nil,
		renderer.WithOnError(func(err error) {
			logger.ErrorContext(ctx, "failed to render", "error", err)
		}))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create renderer: %w", err)
	}

	if err := c.cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}
	logger.DebugContext(ctx, "loaded configuration", "config", c.cfg)

	agent := fmt.Sprintf("google:github-actions-on-gcp/%s", version.Version)
	opts := []option.ClientOption{option.WithUserAgent(agent)}
	webhookClientOptions := &webhook.WebhookClientOptions{
		KeyManagementClientOpts: opts,
	}

	// expect tests to pass overide
	if c.testKMSClientOverride != nil {
		webhookClientOptions.KeyManagementClientOverride = c.testKMSClientOverride
	}

	// expect tests to pass overide
	if c.testCloudBuildClientOverride != nil {
		webhookClientOptions.CloudBuildClientOverride = c.testCloudBuildClientOverride
	}

	// expect tests to pass overide
	if c.testOSFileReaderOverride != nil {
		webhookClientOptions.OSFileReaderOverride = c.testOSFileReaderOverride
	}

	webhookServer, err := webhook.NewServer(ctx, h, c.cfg, webhookClientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server: %w", err)
	}

	mux := webhookServer.Routes(ctx)

	server, err := serving.New(c.cfg.Port)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create serving infrastructure: %w", err)
	}

	return server, mux, nil
}
