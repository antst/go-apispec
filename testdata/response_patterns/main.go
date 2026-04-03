// Package main demonstrates diverse response writing patterns for testing
// apispec's ability to detect response types, status codes, and content types.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

// User is a standard JSON model.
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ErrorResponse is an error model.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PaginatedResponse wraps a list with pagination info.
type PaginatedResponse struct {
	Items      []User `json:"items"`
	TotalCount int    `json:"total_count"`
	Page       int    `json:"page"`
}

// FileMetadata holds info about a file.
type FileMetadata struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

func main() {
	r := chi.NewRouter()

	// --- JSON Encoder patterns ---
	r.Get("/users", ListUsersEncoder)
	r.Post("/users", CreateUserEncoder)
	r.Get("/users/{id}", GetUserEncoder)

	// --- w.Write patterns ---
	r.Get("/health", HealthCheck)
	r.Get("/ping", Ping)

	// --- Binary / raw bytes ---
	r.Get("/files/{id}/download", DownloadFile)
	r.Get("/files/{id}/thumbnail", ServeThumbnail)

	// --- Multiple status codes ---
	r.Get("/cache/{key}", GetWithCache)

	// --- No content / 204 ---
	r.Delete("/users/{id}", DeleteUser)

	// --- String responses ---
	r.Get("/version", GetVersion)

	// --- Paginated response ---
	r.Get("/users/search", SearchUsers)

	// --- Error responses ---
	r.Get("/protected", ProtectedEndpoint)

	// --- fmt.Fprintf ---
	r.Get("/debug", DebugEndpoint)

	// --- io.Copy streaming ---
	r.Get("/stream", StreamData)

	// --- Content-Type inference ---
	r.Get("/image", ServeImage)
	r.Get("/text", ServeText)

	// --- Status code from variable ---
	r.Post("/items", CreateItem)

	// --- Struct with dive tag ---
	r.Post("/newsletter", SubscribeNewsletter)

	// --- Variable path ---
	usersPath := "/v2/users"
	r.Get(usersPath, ListUsersV2)

	// --- Decode from non-r.Body (should NOT produce request body) ---
	r.Get("/config", LoadConfig)

	// --- Generic struct response ---
	r.Get("/user-wrapped", GetUserWrapped)

	// --- Interface-based handler ---
	var server ContentServer = &FileServer{}
	r.Get("/files/serve", server.Serve)

	// --- Method-switching handler (conditional HTTP methods) ---
	r.HandleFunc("/dispatch", DispatchHandler)

	// --- Nested generic ---
	r.Get("/paged-users", GetPagedUsers)

	// --- Cross-function status code ---
	r.Post("/items-func", CreateItemViaFunc)

	http.ListenAndServe(":3000", r)
}

// ListUsersEncoder uses json.NewEncoder to write a slice of Users.
func ListUsersEncoder(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// CreateUserEncoder uses json.NewEncoder with 201 Created.
func CreateUserEncoder(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid body"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// GetUserEncoder uses json.NewEncoder with 404 fallback.
func GetUserEncoder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "missing id", Code: 400})
		return
	}
	user := User{ID: 1, Name: "Alice"}
	json.NewEncoder(w).Encode(user)
}

// HealthCheck writes raw bytes with w.Write.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Ping writes a simple string.
func Ping(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong"))
}

// DownloadFile serves raw binary data via w.Write with []byte.
func DownloadFile(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("/tmp/file.bin")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "file not found"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// ServeThumbnail serves an image thumbnail.
func ServeThumbnail(w http.ResponseWriter, r *http.Request) {
	data := make([]byte, 1024)
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

// GetWithCache demonstrates 200 vs 304 responses.
func GetWithCache(w http.ResponseWriter, r *http.Request) {
	etag := r.Header.Get("If-None-Match")
	if etag == "\"v1\"" {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", "\"v1\"")
	json.NewEncoder(w).Encode(User{ID: 1, Name: "cached"})
}

// DeleteUser returns 204 No Content.
func DeleteUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// GetVersion returns a plain text version string.
func GetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("v1.2.3"))
}

// SearchUsers returns a paginated response.
func SearchUsers(w http.ResponseWriter, r *http.Request) {
	response := PaginatedResponse{
		Items:      []User{{ID: 1, Name: "Alice"}},
		TotalCount: 1,
		Page:       1,
	}
	json.NewEncoder(w).Encode(response)
}

// ProtectedEndpoint returns 401 or 403.
func ProtectedEndpoint(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "unauthorized", Code: 401})
		return
	}
	if token != "Bearer valid" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "forbidden", Code: 403})
		return
	}
	json.NewEncoder(w).Encode(User{ID: 1, Name: "admin"})
}

// DebugEndpoint uses fmt.Fprintf to write the response.
func DebugEndpoint(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "debug info: %s", r.URL.Path)
}

// StreamData uses io.Copy to stream response data.
func StreamData(w http.ResponseWriter, r *http.Request) {
	reader := strings.NewReader("streaming data here")
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, reader)
}

// --- Content-Type inference handlers ---

// ServeImage sets Content-Type to image/png and writes binary data.
func ServeImage(w http.ResponseWriter, r *http.Request) {
	data := make([]byte, 256)
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

// ServeText sets Content-Type to text/plain and writes text.
func ServeText(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "plain text response")
}

// --- Status code from variable handler ---

// CreateItem uses a status code variable before WriteHeader.
func CreateItem(w http.ResponseWriter, r *http.Request) {
	var item User
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid body"})
		return
	}
	status := http.StatusCreated
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(item)
}

// --- Struct with dive validation tags ---

// NewsletterSubscription has dive-validated email list.
type NewsletterSubscription struct {
	Name   string   `json:"name" validate:"required"`
	Emails []string `json:"emails" validate:"required,dive,email"`
	Tags   []string `json:"tags" validate:"dive,min=1,max=50"`
}

// SubscribeNewsletter accepts a subscription with validated email list.
func SubscribeNewsletter(w http.ResponseWriter, r *http.Request) {
	var sub NewsletterSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid body"})
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sub)
}

// --- Variable path handler ---

// ListUsersV2 serves users at a variable-defined path.
func ListUsersV2(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
	}
	json.NewEncoder(w).Encode(users)
}

// --- Decode from non-r.Body handler ---

// AppConfig is a config model loaded from file, not from request body.
type AppConfig struct {
	Debug   bool   `json:"debug"`
	LogPath string `json:"log_path"`
}

// LoadConfig reads config from a file (not from r.Body).
// The json.NewDecoder(file).Decode should NOT produce a request body schema.
func LoadConfig(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open("config.json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "config not found"})
		return
	}
	defer file.Close()

	var cfg AppConfig
	json.NewDecoder(file).Decode(&cfg)
	json.NewEncoder(w).Encode(cfg)
}

// --- Generic struct response ---

// APIResponse is a generic response wrapper.
type APIResponse[T any] struct {
	Data  T      `json:"data"`
	Error string `json:"error,omitempty"`
}

// GetUserWrapped returns a user wrapped in APIResponse.
func GetUserWrapped(w http.ResponseWriter, r *http.Request) {
	resp := APIResponse[User]{
		Data: User{ID: 1, Name: "Alice", Email: "alice@example.com"},
	}
	json.NewEncoder(w).Encode(resp)
}

// --- Method-switching handler ---

// DispatchHandler handles both GET and POST with different responses
// based on r.Method. This tests conditional HTTP method detection.
func DispatchHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		json.NewEncoder(w).Encode([]User{
			{ID: 1, Name: "Alice", Email: "alice@example.com"},
		})
	case "POST":
		var user User
		json.NewDecoder(r.Body).Decode(&user)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(user)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- Interface-based handler ---

// ContentServer is an interface for serving content.
type ContentServer interface {
	Serve(w http.ResponseWriter, r *http.Request)
}

// FileServer is a concrete implementation of ContentServer.
type FileServer struct{}

// Serve serves a file metadata response.
func (fs *FileServer) Serve(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(FileMetadata{
		Name: "document.pdf",
		Size: 1024,
		Type: "application/pdf",
	})
}

// --- Nested generic test ---

// PagedList wraps items with pagination.
type PagedList[T any] struct {
	Items []T `json:"items"`
	Page  int `json:"page"`
	Total int `json:"total"`
}

// GetPagedUsers returns a nested generic: PagedList[User].
func GetPagedUsers(w http.ResponseWriter, r *http.Request) {
	resp := PagedList[User]{
		Items: []User{{ID: 1, Name: "Alice", Email: "a@b.com"}},
		Page:  1,
		Total: 1,
	}
	json.NewEncoder(w).Encode(resp)
}

// --- Cross-function status code ---

// statusCreated returns http.StatusCreated as a constant.
func statusCreated() int {
	return http.StatusCreated
}

// CreateItemViaFunc uses a function call to get the status code.
func CreateItemViaFunc(w http.ResponseWriter, r *http.Request) {
	var item User
	json.NewDecoder(r.Body).Decode(&item)
	w.WriteHeader(statusCreated())
	json.NewEncoder(w).Encode(item)
}
