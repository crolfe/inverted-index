package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func badRequest(w http.ResponseWriter, msg string) {
	body := make(map[string]string)
	body["error"] = msg

	encoded, _ := json.Marshal(body)
	http.Error(w, string(encoded), http.StatusBadRequest)
	return
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.FormValue("q")
	if q == "" {
		badRequest(w, "missing 'q' parameter")
		return
	}

	terms := strings.Split(strings.ToLower(q), " ")
	results := search(terms)

	encoded, err := json.Marshal(results)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, string(encoded))
}

func listenAndServe() {
	http.HandleFunc("/search", handleSearch)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
