package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/google/go-github/v69/github" // Update version if needed
	"golang.org/x/oauth2"
)

// Mock structs for dependencies (replace with your actual implementations if possible)

type mockGithubAuthApp struct {
	installation *mockInstallation
	err          error
}

type mockInstallation struct {
	ID                        int64
	Name                      string
	AllReposOAuth2TokenSource func(ctx context.Context, scopes map[string]string) oauth2.TokenSource
}

func (m *mockGithubAuthApp) InstallationForID(ctx context.Context, id string) (*mockInstallation, error) {
	return m.installation, m.err
}

type mockGithubClient struct {
	jitConfig *github.JITRunnerConfig
	err       error
	resp      *github.Response
}

func (m *mockGithubClient) GenerateRepoJITConfig(ctx context.Context, owner, repo string, req *github.GenerateJITConfigRequest) (*github.JITRunnerConfig, *github.Response, error) {
	return m.jitConfig, m.resp, m.err
}

type mockCloudBuildClient struct {
	build *cloudbuildpb.Build
	err   error
}

func (m *mockCloudBuildClient) RunBuildTrigger(ctx context.Context, req *cloudbuildpb.RunBuildTriggerRequest, opts ...any) (*cloudbuildpb.Build, error) {
	return m.build, m.err
}

type mockKMSClient struct {
	t *testing.T
}

func (m *mockKMSClient) Close() error { return nil }

type mockSigner struct {
	t *testing.T
}

func (m *mockSigner) Sign(ctx context.Context, data []byte) ([]byte, error) {
	return []byte("signed"), nil
}

// Test cases

func TestServerHandler(t *testing.T) {
	testCases := []struct {
		name           string
		event          string
		action         string
		installationID string
		expectedStatus int
		mockGithub     *mockGithubClient
		mockAuth       *mockGithubAuthApp
		mockCloudBuild *mockCloudBuildClient
		wantErr        bool
	}{
		{
			name:           "Successful Workflow Job Queued",
			event:          `{"action": "queued", "workflow_job": {"id": 1, "run_id": 12345}, "installation": {"id": 123}, "organization": {"login": "test-org"}, "repository": {"name": "test-repo"}}`,
			action:         "queued",
			installationID: "123",
			expectedStatus: http.StatusNoContent,
			mockGithub: &mockGithubClient{
				jitConfig: &github.JITRunnerConfig{EncodedJITConfig: github.String("encoded_jit_config")},
			},
			mockAuth: &mockGithubAuthApp{
				installation: &mockInstallation{
					ID:   123,
					Name: "test-installation",
					AllReposOAuth2TokenSource: func(ctx context.Context, scopes map[string]string) oauth2.TokenSource {
						return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
					},
				},
			},
			mockCloudBuild: &mockCloudBuildClient{build: &cloudbuildpb.Build{}},
		},
		{
			name:           "Workflow Job Not Queued",
			event:          `{"action": "completed", "workflow_job": {"id": 1, "run_id": 12345}, "installation": {"id": 123}, "organization": {"login": "test-org"}, "repository": {"name": "test-repo"}}`,
			action:         "completed",
			installationID: "123",
			expectedStatus: http.StatusNoContent,
			mockGithub:     &mockGithubClient{},
			mockAuth:       &mockGithubAuthApp{},
			mockCloudBuild: &mockCloudBuildClient{},
		},
		{
			name:           "Error Getting Installation",
			event:          `{"action": "queued", "workflow_job": {"id": 1, "run_id": 12345}, "installation": {"id": 123}, "organization": {"login": "test-org"}, "repository": {"name": "test-repo"}}`,
			action:         "queued",
			installationID: "123",
			expectedStatus: http.StatusInternalServerError,
			mockGithub:     &mockGithubClient{},
			mockAuth:       &mockGithubAuthApp{err: errors.New("installation not found")},
			mockCloudBuild: &mockCloudBuildClient{},
			wantErr:        true,
		},
		{
			name:           "Error Generating JIT Config",
			event:          `{"action": "queued", "workflow_job": {"id": 1, "run_id": 12345}, "installation": {"id": 123}, "organization": {"login": "test-org"}, "repository": {"name": "test-repo"}}`,
			action:         "queued",
			installationID: "123",
			expectedStatus: http.StatusInternalServerError,
			mockGithub:     &mockGithubClient{err: errors.New("failed to generate jitconfig")},
			mockAuth: &mockGithubAuthApp{
				installation: &mockInstallation{
					ID:   123,
					Name: "test-installation",
					AllReposOAuth2TokenSource: func(ctx context.Context, scopes map[string]string) oauth2.TokenSource {
						return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
					},
				},
			},
			mockCloudBuild: &mockCloudBuildClient{},
			wantErr:        true,
		},
		{
			name:           "Error Running Cloud Build",
			event:          `{"action": "queued", "workflow_job": {"id": 1, "run_id": 12345}, "installation": {"id": 123}, "organization": {"login": "test-org"}, "repository": {"name": "test-repo"}}`,
			action:         "queued",
			installationID: "123",
			expectedStatus: http.StatusInternalServerError,
			mockGithub: &mockGithubClient{
				jitConfig: &github.JITRunnerConfig{EncodedJITConfig: github.String("encoded_jit_config")},
			},
			mockAuth: &mockGithubAuthApp{
				installation: &mockInstallation{
					ID:   123,
					Name: "test-installation",
					AllReposOAuth2TokenSource: func(ctx context.Context, scopes map[string]string) oauth2.TokenSource {
						return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
					},
				},
			},
			mockCloudBuild: &mockCloudBuildClient{err: errors.New("failed to run build")},
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			logger := slog.Logger{}
			os.Setenv("PROJECT_ID", "test-project")
			os.Setenv("LOCATION", "us-west1")
			os.Setenv("TRIGGER_NAME", "test-trigger")
			os.Setenv("TRIGGER_ID", "test-trigger-id")
			defer os.Unsetenv("PROJECT_ID")
			defer os.Unsetenv("LOCATION")
			defer os.Unsetenv("TRIGGER_NAME")
			defer os.Unsetenv("TRIGGER_ID")

			server := &Server{
				logger:           &logger,
				ctx:              ctx,
				webhookSecret:    []byte("test-secret"),
				signer:           &mockSigner{t: t},
				appClient:        tc.mockAuth,
				cloudBuildClient: tc.mockCloudBuild,
			}

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.event))
			req.Header.Set("X-GitHub-Event", "workflow_job")
			req.Header.Set("X-Hub-Signature-256", "sha256=test-signature") // Replace with actual signature validation

			rr := httptest.NewRecorder()
			server.handler(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v, want %v", rr.Code, tc.expectedStatus)
			}

			if tc.wantErr && len(rr.Body.String()) == 0 {
				t.Error("expected error message in response body, but got none")
			}
		})
	}
}

//Helper functions (mostly unchanged from previous response)

func generateJITConfigRequest(event *github.WorkflowJobEvent) *github.GenerateJITConfigRequest {
	return &github.GenerateJITConfigRequest{
		Name:          fmt.Sprintf("GCP-%d", event.WorkflowJob.RunID),
		RunnerGroupID: 1,
		Labels:        []string{"self-hosted", "Linux", "X64"},
	}
}

func buildCloudBuildRequest(jitConfig string) *cloudbuildpb.RunBuildTriggerRequest {
	projectId := os.Getenv("PROJECT_ID")
	location := os.Getenv("LOCATION")
	triggerName := os.Getenv("TRIGGER_NAME")
	triggerId := os.Getenv("TRIGGER_ID")
	return &cloudbuildpb.RunBuildTriggerRequest{
		Name:      fmt.Sprintf("projects/%s/locations/%s/triggers/%s", projectId, location, triggerName),
		ProjectId: projectId,
		TriggerId: triggerId,
		Source: &cloudbuildpb.RepoSource{
			Substitutions: map[string]string{
				"_ENCODED_JIT_CONFIG": jitConfig,
			},
		},
	}
}

// Mock Logger (replace with your actual logging implementation)
type mockLogger struct{}

func (m *mockLogger) InfoContext(ctx context.Context, msg string, args ...any)  {}
func (m *mockLogger) ErrorContext(ctx context.Context, msg string, args ...any) {}

func Test_parseWebhook(t *testing.T) {
	type args struct {
		payload []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *github.WorkflowJobEvent
		wantErr bool
	}{
		{
			name: "Valid Payload",
			args: args{
				payload: []byte(`{"action": "queued", "workflow_job": {"id": 1, "run_id": 12345}, "installation": {"id": 123}, "organization": {"login": "test-org"}, "repository": {"name": "test-repo"}}`),
			},
			want: &github.WorkflowJobEvent{
				Action: github.String("queued"),
				WorkflowJob: &github.WorkflowJob{
					ID:    github.Int64(1),
					RunID: github.Int64(12345),
				},
				Installation: &github.Installation{
					ID: github.Int64(123),
				},
				Org: &github.Organization{
					Login: github.String("test-org"),
				},
				Repo: &github.Repository{
					Name: github.String("test-repo"),
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid Payload",
			args: args{
				payload: []byte(`invalid json`),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWebhook(tt.args.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseWebhook() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseWebhook() = %v, want %v", got, tt.want)
			}
		})
	}
}

func parseWebhook(payload []byte) (*github.WorkflowJobEvent, error) {
	var event github.WorkflowJobEvent
	err := json.Unmarshal(payload, &event)
	return &event, err
}
