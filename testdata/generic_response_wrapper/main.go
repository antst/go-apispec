// Package main exercises a generic envelope wrapper APIResponse[T] returned from
// handlers with different concrete T. Each call site must bind T to its concrete
// type (data: {$ref: User} / {$ref: Product}), not leave it as the unbound
// declaration APIResponse[T any] with data: object.
package main

import (
	"encoding/json"
	"net/http"
)

// APIResponse is the generic envelope.
type APIResponse[T any] struct {
	Data    T      `json:"data"`
	Message string `json:"message"`
}

// User is one payload type.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Product is another payload type.
type Product struct {
	SKU   string `json:"sku"`
	Price int    `json:"price"`
}

func getUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(APIResponse[User]{Data: User{}, Message: "ok"})
}

func getProduct(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(APIResponse[Product]{Data: Product{}, Message: "ok"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", getUser)
	mux.HandleFunc("GET /product", getProduct)
	_ = http.ListenAndServe(":8080", mux)
}
