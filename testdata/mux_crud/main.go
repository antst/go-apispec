// Package main exercises a full gorilla/mux REST CRUD lifecycle with mux.Vars
// path variables and per-route .Methods() matchers.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// Order is the resource.
type Order struct {
	ID     string  `json:"id"`
	Total  float64 `json:"total"`
	Status string  `json:"status"`
}

func listOrders(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode([]Order{}) }
func getOrder(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["id"]
	_ = json.NewEncoder(w).Encode(Order{})
}
func createOrder(w http.ResponseWriter, r *http.Request) {
	var o Order
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(o)
}
func deleteOrder(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["id"]
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/orders", listOrders).Methods("GET")
	r.HandleFunc("/orders", createOrder).Methods("POST")
	r.HandleFunc("/orders/{id}", getOrder).Methods("GET")
	r.HandleFunc("/orders/{id}", deleteOrder).Methods("DELETE")
	_ = http.ListenAndServe(":8080", r)
}
