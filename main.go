// main.go
package main

import (
	"bytes"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/sirupsen/logrus"
)

// getEnv는 환경 변수를 가져오고, 없으면 기본값을 반환합니다.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// LoggingMiddleware는 요청과 응답을 로깅하는 미들웨어입니다.
func LoggingMiddleware(logger *logrus.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 요청 로깅
		reqDump, err := httputil.DumpRequest(r, true)
		if err != nil {
			logger.WithError(err).Error("Failed to dump request")
		} else {
			logger.Infof("Incoming Request:\n%s", string(reqDump))
		}

		// 응답을 캡처하기 위해 ResponseWriter를 래핑
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK, body: &bytes.Buffer{}}

		// 다음 핸들러 호출
		next.ServeHTTP(lrw, r)

		// 응답 로깅
		respDump := lrw.body.String()
		logger.Infof("Outgoing Response: Status %d, Body: %s", lrw.statusCode, respDump)
	})
}

// loggingResponseWriter는 응답을 캡처하여 로깅하기 위한 구조체입니다.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	lrw.body.Write(b) // 응답 본문 캡처
	return lrw.ResponseWriter.Write(b)
}

func main() {
	// 로거 설정
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)

	backendURLStr := getEnv("BACKEND_API_URL", "http://zim-iot-data-api-service.iot-edge")
	backendURL, err := url.Parse(backendURLStr)
	if err != nil {
		logger.Fatalf("Failed to parse BACKEND_API_URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, e error) {
		logger.WithError(e).Error("Proxy error")
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://133.186.135.247")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

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
		// 여기에서 준비 상태를 확인할 수 있는 로직 추가 가능
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
