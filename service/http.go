package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	defaultDownstreamKeepAliveIntervalSeconds = 15
	minDownstreamKeepAliveIntervalSeconds     = 5
	maxDownstreamKeepAliveIntervalSeconds     = 25
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			// avoid setting Content-Length
			if k == "Content-Length" {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	common.SetContextKey(c, constant.ContextKeyRelayResponseStarted, true)
	common.SetContextKey(c, constant.ContextKeyRelayClientReceivedStarted, true)
	c.Writer.Flush()
}

func ReadAllWithJSONKeepAlive(c *gin.Context, src *http.Response) ([]byte, error) {
	if src == nil || src.Body == nil {
		return nil, nil
	}
	keepAlive := StartJSONDownstreamKeepAlive(c, src)
	if keepAlive != nil {
		defer keepAlive.Stop()
	}
	return io.ReadAll(src.Body)
}

type DownstreamKeepAlive struct {
	c      *gin.Context
	src    *http.Response
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
	header sync.Once
	count  atomic.Int64
	mutex  *sync.Mutex
}

func StartJSONDownstreamKeepAlive(c *gin.Context, src *http.Response) *DownstreamKeepAlive {
	if !DownstreamKeepAliveEnabled() || c == nil || c.Writer == nil || responseLooksBinary(src) {
		return nil
	}
	keepAlive := &DownstreamKeepAlive{
		c:     c,
		src:   src,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		mutex: &sync.Mutex{},
	}
	interval := DownstreamKeepAliveInterval()
	go keepAlive.run(interval, []byte("\n"))
	return keepAlive
}

func IOCopyBytesWithJSONKeepAliveGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c == nil || c.Writer == nil {
		return
	}
	if !DownstreamKeepAliveEnabled() || responseLooksBinary(src) {
		IOCopyBytesGracefully(c, src, data)
		return
	}
	keepAlive := &DownstreamKeepAlive{
		c:     c,
		src:   src,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		mutex: &sync.Mutex{},
	}
	keepAlive.writeFinal(data, src)
}

func (keepAlive *DownstreamKeepAlive) run(interval time.Duration, payload []byte) {
	defer close(keepAlive.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := keepAlive.write(payload); err != nil {
				markDownstreamWriteError(keepAlive.c, err)
				return
			}
		case <-keepAlive.stop:
			return
		}
	}
}

func (keepAlive *DownstreamKeepAlive) write(payload []byte) error {
	if keepAlive == nil || keepAlive.c == nil || keepAlive.c.Writer == nil {
		return nil
	}
	if keepAlive.c.Request != nil && keepAlive.c.Request.Context().Err() != nil {
		return keepAlive.c.Request.Context().Err()
	}
	keepAlive.mutex.Lock()
	defer keepAlive.mutex.Unlock()
	keepAlive.ensureHeaders(keepAlive.src)
	if _, err := keepAlive.c.Writer.Write(payload); err != nil {
		return err
	}
	keepAlive.c.Writer.Flush()
	count := keepAlive.count.Add(1)
	common.SetContextKey(keepAlive.c, constant.ContextKeyRelayDownstreamKeepAliveCount, int(count))
	common.SetContextKey(keepAlive.c, constant.ContextKeyRelayJSONKeepAliveStarted, true)
	common.SetContextKey(keepAlive.c, constant.ContextKeyRelayClientReceivedStarted, true)
	return nil
}

func (keepAlive *DownstreamKeepAlive) writeFinal(data []byte, src *http.Response) {
	if keepAlive == nil || keepAlive.c == nil || keepAlive.c.Writer == nil {
		return
	}
	keepAlive.mutex.Lock()
	defer keepAlive.mutex.Unlock()
	keepAlive.ensureHeaders(src)
	if _, err := keepAlive.c.Writer.Write(data); err != nil {
		markDownstreamWriteError(keepAlive.c, err)
		return
	}
	common.SetContextKey(keepAlive.c, constant.ContextKeyRelayDownstreamWriteStatus, "ok")
	common.SetContextKey(keepAlive.c, constant.ContextKeyRelayResponseStarted, true)
	common.SetContextKey(keepAlive.c, constant.ContextKeyRelayClientReceivedStarted, true)
	keepAlive.c.Writer.Flush()
}

func (keepAlive *DownstreamKeepAlive) ensureHeaders(src *http.Response) {
	if keepAlive == nil || keepAlive.c == nil || keepAlive.c.Writer == nil {
		return
	}
	keepAlive.header.Do(func() {
		prepareChunkedJSONResponseHeaders(keepAlive.c, src)
		statusCode := http.StatusOK
		if src != nil {
			statusCode = src.StatusCode
		}
		if !keepAlive.c.Writer.Written() {
			keepAlive.c.Writer.WriteHeader(statusCode)
		}
	})
}

func (keepAlive *DownstreamKeepAlive) Stop() {
	if keepAlive == nil {
		return
	}
	keepAlive.once.Do(func() {
		close(keepAlive.stop)
		<-keepAlive.done
	})
}

func prepareChunkedJSONResponseHeaders(c *gin.Context, src *http.Response) {
	if src != nil {
		common.SetContextKey(c, constant.ContextKeyRelayUpstreamStatus, src.StatusCode)
		for k, v := range src.Header {
			if strings.EqualFold(k, "Content-Length") {
				continue
			}
			if len(v) > 0 {
				c.Writer.Header().Set(k, v[0])
			}
		}
	}
	c.Writer.Header().Del("Content-Length")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
}

func DownstreamKeepAliveEnabled() bool {
	settings := operation_setting.GetGeneralSetting()
	return settings == nil || settings.DownstreamKeepaliveEnabled
}

func DownstreamKeepAliveInterval() time.Duration {
	settings := operation_setting.GetGeneralSetting()
	seconds := defaultDownstreamKeepAliveIntervalSeconds
	if settings != nil && settings.DownstreamKeepaliveIntervalSeconds > 0 {
		seconds = settings.DownstreamKeepaliveIntervalSeconds
	}
	if seconds < minDownstreamKeepAliveIntervalSeconds {
		seconds = minDownstreamKeepAliveIntervalSeconds
	}
	if seconds > maxDownstreamKeepAliveIntervalSeconds {
		seconds = maxDownstreamKeepAliveIntervalSeconds
	}
	return time.Duration(seconds) * time.Second
}

func responseLooksBinary(src *http.Response) bool {
	if src == nil {
		return false
	}
	contentType := strings.ToLower(src.Header.Get("Content-Type"))
	return strings.HasPrefix(contentType, "audio/") ||
		strings.HasPrefix(contentType, "video/") ||
		strings.HasPrefix(contentType, "image/") ||
		strings.Contains(contentType, "octet-stream") ||
		strings.Contains(contentType, "zip")
}

func markDownstreamWriteError(c *gin.Context, err error) {
	if c == nil || err == nil {
		return
	}
	status := "downstream_write_error"
	if c.Request != nil && c.Request.Context().Err() != nil {
		status = "client_aborted"
	}
	common.SetContextKey(c, constant.ContextKeyRelayDownstreamWriteStatus, status)
	common.SetContextKey(c, constant.ContextKeyRelayFinalClassification, status)
	logger.LogError(c, fmt.Sprintf("downstream write failed: %s", err.Error()))
}
