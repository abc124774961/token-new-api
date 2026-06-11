package relay

import (
	"bytes"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func sanitizeOpenAIUpstreamJSONData(info *relaycommon.RelayInfo, jsonData []byte) ([]byte, error) {
	if info == nil {
		return jsonData, nil
	}
	return relaycommon.SanitizeOpenAIPrivacyFields(jsonData, info.ChannelOtherSettings)
}

func sanitizedOpenAIUpstreamBodyReader(c *gin.Context, info *relaycommon.RelayInfo, storage common.BodyStorage) (io.Reader, error) {
	if storage == nil {
		return nil, nil
	}
	if !isJSONRequestContentType(c) {
		return common.ReaderOnly(storage), nil
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	sanitized, err := sanitizeOpenAIUpstreamJSONData(info, body)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(sanitized), nil
}

func isJSONRequestContentType(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("Content-Type")))
	if contentType == "" {
		return false
	}
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}
