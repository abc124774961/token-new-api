package middleware

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

var defNext = func(c *gin.Context) {
	c.Next()
}

var globalWebRateLimitStaticPaths = map[string]struct{}{
	"/apple-touch-icon.png": {},
	"/favicon.ico":          {},
	"/favicon.png":          {},
	"/favicon.svg":          {},
	"/manifest.json":        {},
	"/robots.txt":           {},
	"/site.webmanifest":     {},
	"/sitemap.xml":          {},
}

var globalWebRateLimitStaticExtensions = []string{
	".avif", ".css", ".gif", ".ico", ".jpeg", ".jpg", ".js", ".map",
	".otf", ".png", ".svg", ".ttf", ".txt", ".webmanifest", ".webp",
	".woff", ".woff2", ".xml",
}

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		limiter := rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
		return func(c *gin.Context) {
			if shouldSkipGlobalWebRateLimit(c) {
				c.Next()
				return
			}
			limiter(c)
			if c.IsAborted() && c.Writer.Status() == http.StatusTooManyRequests && !c.Writer.Written() {
				abortWithGlobalWebRateLimitPage(c, common.GlobalWebRateLimitDuration)
			}
		}
	}
	return defNext
}

func abortWithGlobalWebRateLimitPage(c *gin.Context, retryAfterSeconds int64) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 60
	}
	c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusTooManyRequests, "text/html; charset=utf-8", []byte(globalWebRateLimitPage(retryAfterSeconds, c.GetString(common.RequestIdKey))))
	c.Abort()
}

func globalWebRateLimitPage(retryAfterSeconds int64, requestID string) string {
	retryAfter := strconv.FormatInt(retryAfterSeconds, 10)
	requestMeta := ""
	if requestID != "" {
		requestMeta = `<p class="request-id">Request ID: ` + html.EscapeString(requestID) + `</p>`
	}
	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>访问太频繁</title>
  <style>
    :root { color-scheme: light dark; }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 24px;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: #172033;
      background:
        radial-gradient(circle at 18% 12%, rgba(14, 165, 233, .14), transparent 32%),
        linear-gradient(135deg, #f7fbff 0%, #eef7f4 52%, #f8fafc 100%);
    }
    main {
      width: min(520px, 100%);
      border: 1px solid rgba(15, 23, 42, .12);
      border-radius: 8px;
      padding: 32px;
      background: rgba(255, 255, 255, .86);
      box-shadow: 0 20px 56px rgba(15, 23, 42, .12);
    }
    .eyebrow {
      margin: 0 0 12px;
      color: #0891b2;
      font-size: 13px;
      font-weight: 700;
      letter-spacing: 0;
    }
    h1 {
      margin: 0 0 14px;
      font-size: 28px;
      line-height: 1.2;
      letter-spacing: 0;
    }
    p {
      margin: 0 0 12px;
      color: #475569;
      font-size: 15px;
      line-height: 1.7;
    }
    .notice {
      margin: 20px 0;
      padding: 14px 16px;
      border-left: 3px solid #06b6d4;
      border-radius: 6px;
      background: rgba(6, 182, 212, .08);
      color: #0f4c5c;
    }
    .countdown-panel {
      display: flex;
      align-items: baseline;
      gap: 10px;
      margin: 18px 0 4px;
      padding: 18px;
      border-radius: 8px;
      background: rgba(15, 23, 42, .04);
    }
    .countdown-value {
      min-width: 72px;
      font-size: 44px;
      line-height: 1;
      font-weight: 800;
      color: #0891b2;
      font-variant-numeric: tabular-nums;
    }
    .countdown-label {
      color: #475569;
      font-size: 15px;
      font-weight: 650;
    }
    .reload-note {
      margin: 10px 0 0;
      color: #64748b;
      font-size: 13px;
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 24px;
    }
    button {
      min-height: 40px;
      border: 1px solid rgba(15, 23, 42, .14);
      border-radius: 6px;
      padding: 0 16px;
      font-size: 14px;
      font-weight: 650;
      color: #0f172a;
      background: #fff;
      cursor: pointer;
    }
    button.primary {
      border-color: #0891b2;
      color: #fff;
      background: #0891b2;
    }
    button:disabled {
      cursor: not-allowed;
      opacity: .58;
    }
    .request-id {
      margin-top: 18px;
      color: #94a3b8;
      font-size: 12px;
      word-break: break-all;
    }
    @media (prefers-color-scheme: dark) {
      body {
        color: #e5eef8;
        background:
          radial-gradient(circle at 18% 12%, rgba(34, 211, 238, .14), transparent 32%),
          linear-gradient(135deg, #07111f 0%, #0f1f2a 54%, #101827 100%);
      }
      main {
        border-color: rgba(148, 163, 184, .18);
        background: rgba(15, 23, 42, .86);
        box-shadow: 0 24px 70px rgba(0, 0, 0, .36);
      }
      p { color: #b6c3d3; }
      .notice {
        background: rgba(34, 211, 238, .10);
        color: #a5f3fc;
      }
      .countdown-panel {
        background: rgba(148, 163, 184, .08);
      }
      .countdown-value {
        color: #22d3ee;
      }
      .countdown-label {
        color: #b6c3d3;
      }
      .reload-note {
        color: #94a3b8;
      }
      button {
        border-color: rgba(148, 163, 184, .24);
        color: #e5eef8;
        background: rgba(15, 23, 42, .72);
      }
      button.primary {
        border-color: #06b6d4;
        background: #0891b2;
      }
    }
  </style>
</head>
<body>
  <main>
    <p class="eyebrow">HTTP 429</p>
    <h1>访问太频繁</h1>
    <p>检测到你在短时间内重复刷新或快速切换页面。为了保护服务稳定，系统暂时限制了本次访问。</p>
    <p class="notice">请不要过快重复刷新，等待 <strong><span id="countdown">` + retryAfter + `</span> 秒</strong> 后再尝试。</p>
    <div class="countdown-panel" aria-live="polite">
      <span id="countdown-large" class="countdown-value">` + retryAfter + `</span>
      <span class="countdown-label">秒后可以重新尝试</span>
    </div>
    <p id="reload-note" class="reload-note">倒计时结束后页面会自动刷新。</p>
    <div class="actions">
      <button type="button" onclick="history.length > 1 ? history.back() : location.href='/'">返回上一页</button>
      <button id="retry-button" class="primary" type="button" disabled>稍后重试（` + retryAfter + ` 秒）</button>
    </div>
    ` + requestMeta + `
  </main>
  <script>
    (function () {
      var left = ` + retryAfter + `;
      var countdown = document.getElementById('countdown');
      var countdownLarge = document.getElementById('countdown-large');
      var retryButton = document.getElementById('retry-button');
      var reloadNote = document.getElementById('reload-note');
      function tick() {
        var value = String(Math.max(left, 0));
        countdown.textContent = value;
        countdownLarge.textContent = value;
        if (left <= 0) {
          retryButton.textContent = '正在重新尝试...';
          reloadNote.textContent = '正在自动刷新页面，请稍候。';
          window.setTimeout(function () { window.location.reload(); }, 400);
          return;
        }
        retryButton.textContent = '稍后重试（' + value + ' 秒）';
        left -= 1;
        window.setTimeout(tick, 1000);
      }
      tick();
    })();
  </script>
</body>
</html>`
}

func shouldSkipGlobalWebRateLimit(c *gin.Context) bool {
	if c.Request == nil || c.Request.URL == nil {
		return false
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}

	path := c.Request.URL.Path
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "/assets/") {
		return true
	}

	if _, ok := globalWebRateLimitStaticPaths[path]; ok {
		return true
	}

	for _, ext := range globalWebRateLimitStaticExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	if common.GlobalApiRateLimitEnable {
		return rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
	}
	return defNext
}

func CriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
	}
	return defNext
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}

// userRateLimitFactory creates a rate limiter keyed by authenticated user ID
// instead of client IP, making it resistant to proxy rotation attacks.
// Must be used AFTER authentication middleware (UserAuth).
func userRateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			userId := c.GetInt("id")
			if userId == 0 {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			key := fmt.Sprintf("rateLimit:%s:user:%d", mark, userId)
			userRedisRateLimiter(c, maxRequestNum, duration, key)
		}
	}
	// It's safe to call multi times.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		if userId == 0 {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		key := fmt.Sprintf("%s:user:%d", mark, userId)
		if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}
	}
}

// userRedisRateLimiter is like redisRateLimiter but accepts a pre-built key
// (to support user-ID-based keys).
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, key string) {
	ctx := context.Background()
	rdb := common.RDB
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

// SearchRateLimit returns a per-user rate limiter for search endpoints.
// Configurable via SEARCH_RATE_LIMIT_ENABLE / SEARCH_RATE_LIMIT / SEARCH_RATE_LIMIT_DURATION.
func SearchRateLimit() func(c *gin.Context) {
	if !common.SearchRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(common.SearchRateLimitNum, common.SearchRateLimitDuration, "SR")
}
