package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyCodexApplicationEnvironmentHeaders(t *testing.T) {
	db, env := setupCodexApplicationEnvironmentTestDB(t)
	_ = db

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, appconstant.ContextKeyChannelAccountCodexEnvironmentID, env.Id)

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	req.Header.Set("User-Agent", "client-ua")

	applyCodexApplicationEnvironmentHeaders(ctx, &req.Header)

	require.Equal(t, "client-ua", req.Header.Get("User-Agent"))
	require.Equal(t, env.Originator, req.Header.Get("originator"))
	require.Equal(t, env.BetaFeatures, req.Header.Get("X-Codex-Beta-Features"))
	require.Equal(t, "responses=v1", req.Header.Get("OpenAI-Beta"))
	require.Empty(t, req.Header.Get("session_id"))
	require.Empty(t, req.Header.Get("X-Codex-Window-Id"))
	require.Empty(t, req.Header.Get("X-Codex-Trace"))
	require.Empty(t, req.Header.Get("X-Codex-Turn-Metadata"))
}

func TestDoApiRequestAppliesCodexEnvironmentHeaders(t *testing.T) {
	db, env := setupCodexApplicationEnvironmentTestDB(t)
	_ = db

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(ctx, appconstant.ContextKeyChannelAccountCodexEnvironmentID, env.Id)

	serverHeaders := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHeaders <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	service.InitHttpClient()
	adaptor := &testAdaptor{requestURL: server.URL}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: appconstant.ChannelTypeCodex,
			HeadersOverride: map[string]any{
				"User-Agent": "override-ua",
			},
		},
	}

	resp, err := DoApiRequest(adaptor, ctx, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	select {
	case headers := <-serverHeaders:
		require.Equal(t, "override-ua", headers.Get("User-Agent"))
		require.Equal(t, env.Originator, headers.Get("originator"))
		require.Equal(t, "responses=v1", headers.Get("OpenAI-Beta"))
		require.Empty(t, headers.Get("session_id"))
		require.Empty(t, headers.Get("X-Codex-Trace"))
		require.Equal(t, "setup-value", headers.Get("X-Setup"))
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for upstream headers")
	}
}

func setupCodexApplicationEnvironmentTestDB(t *testing.T) (*gorm.DB, *model.CodexApplicationEnvironment) {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CodexApplicationEnvironment{}))

	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	env := &model.CodexApplicationEnvironment{
		Id:           9000001,
		Name:         "test-env",
		Platform:     "macOS",
		AppVersion:   "0.135.0",
		UserAgent:    "Codex Desktop/0.135.0",
		Originator:   "codex_cli_rs",
		SessionID:    "sess-123",
		WindowID:     "win-123",
		BetaFeatures: "terminal_resize_reflow",
		HeadersJSON:  `{"OpenAI-Beta":"responses=v1","X-Codex-Trace":"trace-42"}`,
		Enabled:      true,
		Source:       "custom",
		Remark:       "test environment",
	}
	require.NoError(t, db.Create(env).Error)
	return db, env
}

type testAdaptor struct {
	requestURL string
}

func (a *testAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a *testAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return a.requestURL, nil
}

func (a *testAdaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	req.Set("X-Setup", "setup-value")
	req.Set("User-Agent", "setup-ua")
	return nil
}

func (a *testAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}

func (a *testAdaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *testAdaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, nil
}

func (a *testAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}

func (a *testAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, nil
}

func (a *testAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}

func (a *testAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return nil, nil
}

func (a *testAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return nil, nil
}

func (a *testAdaptor) GetModelList() []string {
	return nil
}

func (a *testAdaptor) GetChannelName() string {
	return "test"
}

func (a *testAdaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, nil
}

func (a *testAdaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}
