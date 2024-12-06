// main.go
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func main() {
	backendURLStr := getEnv("BACKEND_API_URL", "http://zim-iot-data-api-service.iot-edge")
	backendURL, err := url.Parse(backendURLStr)
	if err != nil {
		log.Fatalf("Failed to parse BACKEND_API_URL: %v", err)
	}

	// 리버스 프록시 설정
	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, e error) {
		log.Printf("Proxy error: %v", e)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		proxy.ServeHTTP(w, r)
	})

	port := getEnv("PORT", "8080")
	log.Printf("Starting proxy server on port %s, forwarding to %s", port, backendURLStr)

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
