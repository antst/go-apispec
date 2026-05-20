// Package dto holds request/response shapes with deliberate name overlap so
// the fixture can distinguish the HTTP-layer types (CheckRoomHTTPRequest /
// HTTPResponse) from same-prefix internal RPC types (CheckRoomRequest /
// Response). Issue #22's bug surfaces when apispec picks the wrong one.
package dto

// CheckRoomHTTPRequest is decoded from the JSON body. It MUST be the type
// referenced by the operation's requestBody.
type CheckRoomHTTPRequest struct {
	Room string `json:"room"`
}

// CheckRoomHTTPResponse is what the handler writes back as JSON.
type CheckRoomHTTPResponse struct {
	OK bool `json:"ok"`
}

// CheckRoomRequest is an unrelated internal RPC shape. Any leakage of this
// type into the OpenAPI spec is a bug.
type CheckRoomRequest struct {
	RoomID string `json:"roomId"`
}

// CheckRoomResponse — same package, same prefix, completely unrelated.
type CheckRoomResponse struct {
	Allowed bool `json:"allowed"`
}

// LivenessResponse is returned by the GET /health/live endpoint.
type LivenessResponse struct {
	Status string `json:"status"`
}
