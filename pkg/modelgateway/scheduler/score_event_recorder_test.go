package scheduler

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestScoreEventRecorderRespectsObservabilitySwitch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayScoreEvent{}))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	defer restoreSetting()

	recorder := NewScoreEventRecorder(8)
	result := core.AttemptResult{
		RequestID:     "req-score-event-switch",
		AttemptIndex:  0,
		ChannelID:     88,
		ModelName:     "gpt-5.5",
		SelectedGroup: "vip",
		EndpointType:  constant.EndpointTypeOpenAI,
		ObservedAt:    time.Now(),
	}
	snapshot := core.RuntimeSnapshot{
		Key: core.RuntimeKey{
			ChannelID:      88,
			RequestedModel: "gpt-5.5",
			Group:          "vip",
			EndpointType:   constant.EndpointTypeOpenAI,
		},
	}
	decision := core.ScoreSampleDecision{ScoreSample: true}
	before := core.ScoreResult{Total: 0.4, Items: []core.ScoreItem{{Key: "completion_rate", Score: 0.4, Weight: 1, WeightedScore: 0.4}}}
	after := core.ScoreResult{Total: 0.8, Items: []core.ScoreItem{{Key: "completion_rate", Score: 0.8, Weight: 1, WeightedScore: 0.8}}}

	recorder.ReportAdjustment(result, snapshot, decision, before, after)
	time.Sleep(30 * time.Millisecond)
	var disabledCount int64
	require.NoError(t, db.Model(&model.ModelGatewayScoreEvent{}).Count(&disabledCount).Error)
	require.Equal(t, int64(0), disabledCount)

	enabledSetting := scheduler_setting.DefaultSetting()
	enabledSetting.ObservabilityScoreEventEnabled = true
	scheduler_setting.SetSetting(enabledSetting)
	recorder.ReportAdjustment(result, snapshot, decision, before, after)

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelGatewayScoreEvent{}).Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)
}
