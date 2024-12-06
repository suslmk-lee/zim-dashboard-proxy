// main.go
package main

import (
	"bytes"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// getEnv retrieves environment variables with a default fallback.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// LoggingMiddleware logs incoming requests and outgoing responses.
func LoggingMiddleware(logger *logrus.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log the incoming request
		reqDump, err := httputil.DumpRequest(r, true)
		if err != nil {
			logger.WithError(err).Error("Failed to dump request")
		} else {
			logger.Infof("Incoming Request:\n%s", string(reqDump))
		}

		// Capture the response using a custom ResponseWriter
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK, body: &bytes.Buffer{}}

		// Process the request
		next.ServeHTTP(lrw, r)

		// Log the outgoing response
		respDump := lrw.body.String()
		logger.Infof("Outgoing Response: Status %d, Body: %s", lrw.statusCode, respDump)
	})
}

// loggingResponseWriter captures the status code and response body.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// WriteHeader captures the status code.
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Write captures the response body.
func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	lrw.body.Write(b)
	return lrw.ResponseWriter.Write(b)
}

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logger.SetOutput(os.Stdout)

	logLevelStr := getEnv("LOG_LEVEL", "info")
	logLevel, err := logrus.ParseLevel(strings.ToLower(logLevelStr))
	if err != nil {
		logger.Warnf("Invalid LOG_LEVEL '%s', defaulting to 'info'", logLevelStr)
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	backendURLStr := getEnv("BACKEND_API_URL", "http://zim-iot-data-api-service.iot-edge")
	backendURL, err := url.Parse(backendURLStr)
	if err != nil {
		logger.Fatalf("Failed to parse BACKEND_API_URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, e error) {
		logger.WithError(e).Errorf("Proxy error: %s %s", req.Method, req.URL.Path)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigins := []string{
			"http://133.186.135.247",
		}

		isAllowed := false
		for _, o := range allowedOrigins {
			if o == origin {
				isAllowed = true
				break
			}
		}

		if isAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			http.Error(w, "CORS origin denied", http.StatusForbidden)
			return
		}

		// Set other CORS headers
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		proxy.ServeHTTP(w, r)
	})

	loggedHandler := LoggingMiddleware(logger, handler)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Add readiness logic if needed
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	http.Handle("/", loggedHandler)

	port := getEnv("PORT", "8080")
	logger.Infof("Starting proxy server on port %s, forwarding to %s", port, backendURLStr)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}
}
