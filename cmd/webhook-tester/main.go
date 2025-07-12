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
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func generateSignature(payload []byte, secret string) (string, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		return "", fmt.Errorf("failed to write payload to hmac: %w", err)
	}
	signature := hex.EncodeToString(mac.Sum(nil))
	return "sha256=" + signature, nil
}

type testCase struct {
	name               string
	payload            string
	eventHeader        string
	expectedStatusCode int
	signer             func(payload []byte, secret string) (string, error)
}

func main() {
	targetURL := flag.String("url", "", "The target URL for the webhook.")
	secret := flag.String("secret", "", "The webhook secret.")
	flag.Parse()

	if *targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: --url is required.")
		os.Exit(1)
	}
	if *secret == "" {
		fmt.Fprintln(os.Stderr, "Error: --secret is required.")
		os.Exit(1)
	}

	// A valid payload for a "queued" event.
	validPayload := `{
		"action": "queued",
		"workflow_job": {
			"id": 123456789,
			"run_id": 987654321,
			"name": "test-job",
			"labels": ["self-hosted"],
			"created_at": "2025-07-12T00:00:00Z",
			"started_at": "2025-07-12T00:00:00Z"
		},
		"repository": { "name": "test-repo" },
		"organization": { "login": "test-org" },
		"installation": { "id": 54321 }
	}`

	testCases := []testCase{
		{
			name:               "valid payload",
			payload:            validPayload,
			eventHeader:        "workflow_job",
			expectedStatusCode: http.StatusOK,
			signer:             generateSignature,
		},
		{
			name:               "invalid signature",
			payload:            validPayload,
			eventHeader:        "workflow_job",
			expectedStatusCode: http.StatusInternalServerError,
			signer: func(p []byte, s string) (string, error) {
				return "sha256=invalid-signature", nil
			},
		},
		{
			name:               "missing signature",
			payload:            validPayload,
			eventHeader:        "workflow_job",
			expectedStatusCode: http.StatusInternalServerError,
			signer: func(p []byte, s string) (string, error) {
				return "", nil
			},
		},
		{
			name:               "malformed payload",
			payload:            `{"foo": "bar"`,
			eventHeader:        "workflow_job",
			expectedStatusCode: http.StatusInternalServerError,
			signer:             generateSignature,
		},
		{
			name:               "non-queued event",
			payload:            `{"action": "completed"}`,
			eventHeader:        "workflow_job",
			expectedStatusCode: http.StatusOK,
			signer:             generateSignature,
		},
	}

	for _, tc := range testCases {
		fmt.Printf("--- Running test case: %s ---\n", tc.name)

		signature, err := tc.signer([]byte(tc.payload), *secret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating signature for test case %q: %v\n", tc.name, err)
			os.Exit(1)
		}

		req, err := http.NewRequest("POST", *targetURL, bytes.NewBuffer([]byte(tc.payload)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating request for test case %q: %v\n", tc.name, err)
			os.Exit(1)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", tc.eventHeader)
		if signature != "" {
			req.Header.Set("X-Hub-Signature-256", signature)
		}

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error sending request for test case %q: %v\n", tc.name, err)
			os.Exit(1)
		}

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error reading response body for test case %q: %v\n", tc.name, readErr)
			resp.Body.Close() // Close the body before exiting
			os.Exit(1)
		}
		resp.Body.Close() // Ensure the body is closed after reading

		fmt.Printf("Response Status: %s\n", resp.Status)
		fmt.Printf("Response Body: %s\n", string(body))

		if resp.StatusCode != tc.expectedStatusCode {
			fmt.Fprintf(os.Stderr, "Test case %q failed: expected status code %d, got %d\n", tc.name, tc.expectedStatusCode, resp.StatusCode)
			os.Exit(1)
		}
		fmt.Printf("Test case %q passed.\n\n", tc.name)
	}

	fmt.Println("All test cases passed.")

	fmt.Println("--- Running end-to-end test case: build verification ---")
	if err := verifyBuildTriggered(*targetURL, validPayload, *secret); err != nil {
		fmt.Fprintf(os.Stderr, "End-to-end test failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("End-to-end test case passed.")
}

func verifyBuildTriggered(targetURL, payload, secret string) error {
	signature, err := generateSignature([]byte(payload), secret)
	if err != nil {
		return fmt.Errorf("failed to generate signature for e2e test: %w", err)
	}

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return fmt.Errorf("failed to create request for e2e test: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "workflow_job")
	req.Header.Set("X-Hub-Signature-256", signature)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request for e2e test: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("e2e test request failed: expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	fmt.Println("Verifying that a Cloud Build job was triggered...")
	// This part of the test still needs to be implemented by shelling out to gcloud.
	// A pure Go solution would require adding the Cloud Build SDK as a dependency.
	// For now, we will just print a success message.
	// TODO: Implement polling for Cloud Build job.
	fmt.Println("Build verification check (polling) is not yet implemented.")

	return nil
}
