// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
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
	"net/http"

	"test_cgo_mixed/ai" // This will have CGO issues
	"test_cgo_mixed/api"
)

func main() {
	// Register API routes
	api.RegisterRoutes()

	// Initialize AI (but this might fail due to CGO)
	ai.Init()

	http.ListenAndServe(":8080", nil)
}
