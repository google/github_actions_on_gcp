// Copyright 2025 Google LLC
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

package webhook

import (
	"fmt"
	"os"
)

// OSFileReader implements FileReader using the os package.
type OSFileReader struct{}

// Provdes an instance of a FileReader.
func NewOSFileReader() FileReader {
	return OSFileReader{}
}

// ReadFile reads a file and returns its content.
func (o OSFileReader) ReadFile(filename string) ([]byte, error) {
	res, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return res, nil
}
