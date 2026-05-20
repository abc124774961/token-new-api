package integration_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/stretchr/testify/require"
)

func TestDefaultRuntimeObservabilityDepsDoNotStartRuntimeEventSubscriber(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		integration.ResetDefaultRuntimeObservabilityDeps()
	})

	setting := scheduler_setting.DefaultSetting()
	setting.RuntimeSyncEnabled = true
	setting.RuntimeSyncRedisEnabled = false
	setting.RuntimeSyncEventPushEnabled = true
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restoreSetting)

	integration.ResetDefaultRuntimeObservabilityDeps()
	runtimeDeps := integration.DefaultRuntimeObservabilityDeps()

	require.NotNil(t, runtimeDeps)
	require.NotNil(t, runtimeDeps.RuntimeSyncEventStore)
	requireNoRuntimeEventSubscriber(t, runtimeDeps)
}

func TestExplicitRuntimeEventSubscriberLifecycleDisabledEntrySnapshotEmptyAndCloseIdempotent(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
		require.NoError(t, integration.CloseRuntimeEventSubscriber())
	})

	setting := scheduler_setting.DefaultSetting()
	setting.RuntimeSyncEnabled = true
	setting.RuntimeSyncRedisEnabled = true
	setting.RuntimeSyncEventSubscribeEnabled = true
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restoreSetting)

	integration.StartRuntimeEventSubscriber()
	require.Zero(t, integration.RuntimeEventSubscriberSnapshot().Total)
	require.NoError(t, integration.CloseRuntimeEventSubscriber())
	require.NoError(t, integration.CloseRuntimeEventSubscriber())

	common.RedisEnabled = true
	common.RDB = nil
	integration.SyncRuntimeEventSubscriberLifecycle()
	require.Zero(t, integration.RuntimeEventSubscriberSnapshot().Total)
}

func requireNoRuntimeEventSubscriber(t *testing.T, runtimeDeps *integration.DefaultRuntimeObservability) {
	t.Helper()

	value := reflect.ValueOf(runtimeDeps)
	require.Equal(t, reflect.Pointer, value.Kind())
	value = value.Elem()
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldType := valueType.Field(i)
		if !strings.Contains(strings.ToLower(fieldType.Name), "subscriber") {
			continue
		}
		field := value.Field(i)
		switch field.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			require.Truef(t, field.IsNil(), "default runtime observability deps should not initialize %s", fieldType.Name)
		default:
			require.Zerof(t, field.Interface(), "default runtime observability deps should not initialize %s", fieldType.Name)
		}
	}
}
