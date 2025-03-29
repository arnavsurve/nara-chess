package main

import (
	"arnavsurve/nara-chess/server/pkg/handlers"
	"log"
	"net/http"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/submitMove", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleSubmitHistory(w, r)
	})

	muxCORS := CORSMiddleware(mux)

	log.Println("Serving at 127.0.0.1:42069")
	if err = http.ListenAndServe(":42069", muxCORS); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
