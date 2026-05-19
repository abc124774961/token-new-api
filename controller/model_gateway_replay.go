package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/replay"
	"github.com/gin-gonic/gin"
)

func ExportModelGatewayReplay(c *gin.Context) {
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" {
		common.ApiErrorMsg(c, "request_id is required")
		return
	}

	exporter := replay.NewArtifactExporter(
		replay.NewGormRecordRepository(model.DB),
		replay.ExportOptions{StableIDs: c.Query("stable_ids") == "true"},
	)
	artifact, err := exporter.ExportByRequestID(requestID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if c.Query("download") == "true" {
		data, err := replay.MarshalArtifact(artifact)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filename := fmt.Sprintf("modelgateway-replay-%s.json", safeReplayFilenamePart(requestID))
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/json; charset=utf-8", append(data, '\n'))
		return
	}

	common.ApiSuccess(c, gin.H{
		"request_id": requestID,
		"count":      len(artifact.Records),
		"artifact":   artifact,
	})
}

func safeReplayFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if strings.Trim(out, "_") == "" {
		return "unknown"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}
