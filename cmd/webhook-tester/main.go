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
	signatureHeader    string
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
			signatureHeader:    "sha256=invalid",
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
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response Status: %s\n", resp.Status)
		fmt.Printf("Response Body: %s\n", string(body))

		if resp.StatusCode != tc.expectedStatusCode {
			fmt.Fprintf(os.Stderr, "Test case %q failed: expected status code %d, got %d\n", tc.name, tc.expectedStatusCode, resp.StatusCode)
			os.Exit(1)
		}
		fmt.Printf("Test case %q passed.\n\n", tc.name)
	}

	fmt.Println("All test cases passed.")
}
