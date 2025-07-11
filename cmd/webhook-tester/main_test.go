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
	"testing"
)

func TestGenerateSignature(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"foo":"bar"}`)
	secret := "test-secret"
	// This is the known correct HMAC-SHA256 signature for the payload and secret.
	// Generated with: echo -n '{"foo":"bar"}' | openssl sha256 -hmac "test-secret"
	expectedSignature := "sha256=9b1abf7d901bda91325d00f6b397fb0dc257937939b27d4dc67848ab9e08f6c0"

	got, err := generateSignature(payload, secret)
	if err != nil {
		t.Fatalf("generateSignature() returned an unexpected error: %v", err)
	}

	if got != expectedSignature {
		t.Errorf("generateSignature() got = %v, want %v", got, expectedSignature)
	}
}
