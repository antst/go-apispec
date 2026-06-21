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

package spec

import (
	"strconv"
	"strings"
)

// SchemaMapperImpl implements SchemaMapper
type SchemaMapperImpl struct {
	cfg *APISpecConfig
}

// NewSchemaMapper creates a new schema mapper
func NewSchemaMapper(cfg *APISpecConfig) *SchemaMapperImpl {
	return &SchemaMapperImpl{
		cfg: cfg,
	}
}

// HTTPStatusByName maps HTTP status code names to their numeric values.
var HTTPStatusByName = map[string]int{
	// 1xx
	"StatusContinue":           100,
	"StatusSwitchingProtocols": 101,
	"StatusProcessing":         102,
	"StatusEarlyHints":         103,

	// 2xx
	"StatusOK":                   200,
	"StatusCreated":              201,
	"StatusAccepted":             202,
	"StatusNonAuthoritativeInfo": 203,
	"StatusNoContent":            204,
	"StatusResetContent":         205,
	"StatusPartialContent":       206,
	"StatusMultiStatus":          207,
	"StatusAlreadyReported":      208,
	"StatusIMUsed":               226,

	// 3xx
	"StatusMultipleChoices":   300,
	"StatusMovedPermanently":  301,
	"StatusFound":             302,
	"StatusSeeOther":          303,
	"StatusNotModified":       304,
	"StatusUseProxy":          305,
	"StatusTemporaryRedirect": 307,
	"StatusPermanentRedirect": 308,

	// 4xx
	"StatusBadRequest":                   400,
	"StatusUnauthorized":                 401,
	"StatusPaymentRequired":              402,
	"StatusForbidden":                    403,
	"StatusNotFound":                     404,
	"StatusMethodNotAllowed":             405,
	"StatusNotAcceptable":                406,
	"StatusProxyAuthRequired":            407,
	"StatusRequestTimeout":               408,
	"StatusConflict":                     409,
	"StatusGone":                         410,
	"StatusLengthRequired":               411,
	"StatusPreconditionFailed":           412,
	"StatusRequestEntityTooLarge":        413,
	"StatusRequestURITooLong":            414,
	"StatusUnsupportedMediaType":         415,
	"StatusRequestedRangeNotSatisfiable": 416,
	"StatusExpectationFailed":            417,
	"StatusTeapot":                       418,
	"StatusMisdirectedRequest":           421,
	"StatusUnprocessableEntity":          422,
	"StatusLocked":                       423,
	"StatusFailedDependency":             424,
	"StatusTooEarly":                     425,
	"StatusUpgradeRequired":              426,
	"StatusPreconditionRequired":         428,
	"StatusTooManyRequests":              429,
	"StatusRequestHeaderFieldsTooLarge":  431,
	"StatusUnavailableForLegalReasons":   451,

	// 5xx
	"StatusInternalServerError":           500,
	"StatusNotImplemented":                501,
	"StatusBadGateway":                    502,
	"StatusServiceUnavailable":            503,
	"StatusGatewayTimeout":                504,
	"StatusHTTPVersionNotSupported":       505,
	"StatusVariantAlsoNegotiates":         506,
	"StatusInsufficientStorage":           507,
	"StatusLoopDetected":                  508,
	"StatusNotExtended":                   510,
	"StatusNetworkAuthenticationRequired": 511,
}

// MapStatusCode maps a status code string to HTTP status code
func (s *SchemaMapperImpl) MapStatusCode(statusStr string) (int, bool) {
	// Remove quotes if present
	statusStr = strings.Trim(statusStr, "\"")

	if i := strings.LastIndex(statusStr, "."); i != -1 {
		statusStr = statusStr[i+1:]
	}

	// Check for net/http status constants
	statusInt, ok := HTTPStatusByName[statusStr]
	if ok {
		return statusInt, true
	}

	// Try to parse as integer
	status, err := strconv.Atoi(statusStr)
	if err != nil {
		return 0, false
	}

	return status, true
}

// MapMethodFromFunctionName extracts HTTP method from function name
func (s *SchemaMapperImpl) MapMethodFromFunctionName(funcName string) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, method := range methods {
		if strings.Contains(strings.ToUpper(funcName), method) {
			return method
		}
	}
	return ""
}
