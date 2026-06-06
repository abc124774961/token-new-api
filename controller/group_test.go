package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type getUserGroupsAPIResponse struct {
	Success bool                             `json:"success"`
	Message string                           `json:"message"`
	Data    map[string]userGroupResponseItem `json:"data"`
}

func TestGetUserGroupsIncludesDynamicBillingWhenRequested(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetUserGroupDynamicBillingDisplayCache()
	defer resetUserGroupDynamicBillingDisplayCache()

	originalUserGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","codex-plus":"Plus 分组","auto":"自动分组"}`))
	defer func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserGroups))
	}()

	originalAutoGroups := setting.AutoGroups2JsonString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["codex-plus"]`))
	defer func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	}()

	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"codex-plus":0.1}`))
	defer func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
	}()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingEnabledAt:      common.GetTimestamp() - 3600,
		DynamicBillingWindowSamples:  300,
		DynamicBillingMinSamples:     1,
		DynamicBillingRefreshSeconds: 30,
		DynamicBillingMaxAgeSeconds:  300,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
				CandidateGroups:  []string{"codex-plus"},
			},
		},
	})
	defer restoreSetting()

	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-test:auto": {
			RequestedModel: "gpt-test",
			Group:          "auto",
			Ratio:          0.0671,
			PricePerM:      0.168,
			SampleCount:    12,
			CalculatedAt:   time.Now().Unix() - 10,
		},
	})
	defer restoreBaselines()

	router := gin.New()
	router.GET("/api/user/self/groups", func(c *gin.Context) {
		c.Set("id", 0)
		GetUserGroups(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/self/groups?include_dynamic_billing=true", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response getUserGroupsAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	autoGroup, ok := response.Data["auto"]
	require.True(t, ok)
	require.Equal(t, "自动", autoGroup.Ratio)
	require.NotNil(t, autoGroup.DynamicBilling)
	require.Equal(t, "auto", autoGroup.DynamicBilling.PolicyGroup)
	require.InEpsilon(t, 0.0671, autoGroup.DynamicBilling.CurrentRatio, 0.000001)

	plusGroup, ok := response.Data["codex-plus"]
	require.True(t, ok)
	require.Equal(t, 0.1, plusGroup.Ratio)
	require.Nil(t, plusGroup.DynamicBilling)
}

func TestGetUserGroupsOmitsDynamicBillingByDefault(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetUserGroupDynamicBillingDisplayCache()
	defer resetUserGroupDynamicBillingDisplayCache()

	originalUserGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","auto":"自动分组"}`))
	defer func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserGroups))
	}()

	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	defer func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
	}()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled: true,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
			},
		},
	})
	defer restoreSetting()

	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-test:auto": {
			RequestedModel: "gpt-test",
			Group:          "auto",
			Ratio:          0.08,
			PricePerM:      0.2,
			SampleCount:    6,
			CalculatedAt:   time.Now().Unix() - 10,
		},
	})
	defer restoreBaselines()

	router := gin.New()
	router.GET("/api/user/self/groups", func(c *gin.Context) {
		c.Set("id", 0)
		GetUserGroups(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/self/groups", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response getUserGroupsAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	autoGroup := response.Data["auto"]
	require.Nil(t, autoGroup.DynamicBilling)
}

func TestBuildUserGroupDynamicBillingDisplayMapIncludesRatioRange(t *testing.T) {
	currentOverview := ModelGatewayDynamicBillingOverview{
		Enabled: true,
		Groups: []ModelGatewayDynamicBillingGroupOverview{
			{
				PolicyGroup:  "auto",
				Status:       "active",
				CurrentRatio: 0.0692,
			},
		},
	}
	overview7d := ModelGatewayDynamicBillingOverview{
		Enabled: true,
		Groups: []ModelGatewayDynamicBillingGroupOverview{
			{
				PolicyGroup:      "auto",
				Status:           "active",
				AverageRatio:     0.0673,
				MinRatio:         0.0614,
				MaxRatio:         0.0739,
				AveragePricePerM: 0.22,
			},
		},
	}

	result := buildUserGroupDynamicBillingDisplayMap(currentOverview, overview7d)
	autoItem, ok := result["auto"]
	require.True(t, ok)
	require.InEpsilon(t, 0.0692, autoItem.CurrentRatio, 0.000001)
	require.InEpsilon(t, 0.0673, autoItem.AverageRatio7d, 0.000001)
	require.InEpsilon(t, 0.0614, autoItem.MinRatio7d, 0.000001)
	require.InEpsilon(t, 0.0739, autoItem.MaxRatio7d, 0.000001)
}
