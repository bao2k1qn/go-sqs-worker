package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// LoggerMiddleware logs structured request info with JSON request bodies
func LoggerMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method
		var bodyBytes []byte

		// --- Chỉ đọc body nếu là POST, PUT, PATCH ---
		if method == "POST" || method == "PUT" || method == "PATCH" {
			if c.Request.Body != nil {
				bodyBytes, _ = io.ReadAll(c.Request.Body)
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // khôi phục body
			}
		}

		// --- Query params ---
		queryParams := map[string]string{}
		for key, vals := range c.Request.URL.Query() {
			queryParams[key] = strings.Join(vals, ",")
		}

		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()

		logData := logrus.Fields{
			"timestamp":  time.Now().Format(time.RFC3339),
			"status":     status,
			"method":     method,
			"path":       c.Request.URL.Path,
			"latency_ms": latency.Milliseconds(),
		}

		if len(queryParams) > 0 {
			logData["query_params"] = queryParams
		}

		// --- Log body nếu có ---
		if len(bodyBytes) > 0 {
			if json.Valid(bodyBytes) {
				// Parse thành object bất kỳ
				var body interface{}
				if err := json.Unmarshal(bodyBytes, &body); err == nil {
					body = sanitizeJSON(body) // ẩn thông tin nhạy cảm
					logData["request_body"] = body
				} else {
					logData["request_body"] = string(bodyBytes)
				}
			} else {
				logData["request_body"] = string(bodyBytes)
			}
		}

		// --- Log lỗi (nếu có) ---
		if len(c.Errors) > 0 {
			logData["errors"] = c.Errors.ByType(gin.ErrorTypePrivate).String()
		}

		entry := logger.WithFields(logData)
		switch {
		case status >= 500:
			entry.Error("server_error")
		case status >= 400:
			entry.Warn("client_error")
		default:
			entry.Info("request_completed")
		}
	}
}

// sanitizeJSON ẩn các trường nhạy cảm trong JSON object (map hoặc slice)
func sanitizeJSON(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "password") ||
				strings.Contains(lower, "token") ||
				strings.Contains(lower, "secret") ||
				strings.Contains(lower, "api_key") ||
				strings.Contains(lower, "access_token") {
				v[k] = "***REDACTED***"
			} else {
				v[k] = sanitizeJSON(val)
			}
		}
	case []interface{}:
		for i, item := range v {
			v[i] = sanitizeJSON(item)
		}
	}
	return data
}
