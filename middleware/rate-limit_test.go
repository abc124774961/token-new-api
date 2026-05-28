package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestShouldSkipGlobalWebRateLimitForStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "asset js", method: http.MethodGet, path: "/assets/index-DaXTzQrh.js", want: true},
		{name: "asset css", method: http.MethodHead, path: "/assets/index-CC-Z4ggV.css", want: true},
		{name: "favicon", method: http.MethodGet, path: "/favicon.ico", want: true},
		{name: "root document", method: http.MethodGet, path: "/", want: false},
		{name: "login document", method: http.MethodGet, path: "/login", want: false},
		{name: "api route", method: http.MethodGet, path: "/api/status", want: false},
		{name: "post asset path", method: http.MethodPost, path: "/assets/index.js", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tt.method, tt.path, nil)

			if got := shouldSkipGlobalWebRateLimit(c); got != tt.want {
				t.Fatalf("shouldSkipGlobalWebRateLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlobalWebRateLimitReturnsFriendlyHtmlPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.RedisEnabled
	originalEnabled := common.GlobalWebRateLimitEnable
	originalNum := common.GlobalWebRateLimitNum
	originalDuration := common.GlobalWebRateLimitDuration
	originalLimiter := inMemoryRateLimiter
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		common.GlobalWebRateLimitEnable = originalEnabled
		common.GlobalWebRateLimitNum = originalNum
		common.GlobalWebRateLimitDuration = originalDuration
		inMemoryRateLimiter = originalLimiter
	})

	common.RedisEnabled = false
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = 1
	common.GlobalWebRateLimitDuration = 30
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	router := gin.New()
	router.Use(GlobalWebRateLimit())
	router.GET("/login", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/login", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusOK)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/login", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if got := second.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want %q", got, "30")
	}
	if got := second.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	body := second.Body.String()
	for _, want := range []string{"访问太频繁", "请不要过快重复刷新", "稍后重试", "倒计时结束后页面会自动刷新", "window.location.reload()"} {
		if !strings.Contains(body, want) {
			t.Fatalf("friendly page missing %q in body: %s", want, body)
		}
	}
}
