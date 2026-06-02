package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDynamicBillingBaselineMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		initCol()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestEnsureDynamicBillingBaselineColumnsAddsCostMultiplierFields(t *testing.T) {
	db := setupDynamicBillingBaselineMigrationTestDB(t)
	require.NoError(t, db.Exec(`CREATE TABLE model_gateway_dynamic_billing_baselines (
		id integer primary key autoincrement,
		billing_group text not null,
		reference_model text default '',
		ratio decimal(18,8) default 0,
		reference_price_per_m decimal(18,8) default 0,
		sample_count integer default 0,
		model_count integer default 0,
		window_start bigint default 0,
		window_end bigint default 0,
		profit_rate decimal(18,8) default 0,
		calculated_at bigint,
		created_at bigint,
		updated_at bigint
	)`).Error)

	require.NoError(t, ensureDynamicBillingBaselineColumns())

	for _, column := range []string{
		"base_quota_at_ratio_1",
		"cost_multiplier",
		"apply_reason",
		"required_revenue_usd",
		"effective_ratio",
		"upstream_cost_usd",
	} {
		require.True(t, db.Migrator().HasColumn(&ModelGatewayDynamicBillingBaseline{}, column), column)
	}
}

func TestEnsureModelGatewayProfitRecommendationColumnsAddsScopeFields(t *testing.T) {
	db := setupDynamicBillingBaselineMigrationTestDB(t)
	require.NoError(t, db.Exec(`CREATE TABLE model_gateway_profit_ratio_recommendations (
		id integer primary key autoincrement,
		window varchar(32) default '24h',
		dimension varchar(32) default 'group',
		start_timestamp bigint,
		end_timestamp bigint,
		target_profit_rate decimal(18,8) default 0,
		revenue_usd decimal(18,8) default 0,
		operating_cost_usd decimal(18,8) default 0,
		created_at bigint,
		updated_at bigint
	)`).Error)

	require.NoError(t, ensureModelGatewayProfitRecommendationColumns())

	for _, column := range []string{
		"scope_type",
		"scope_id",
		"scope_key",
		"scope_name",
	} {
		require.True(t, db.Migrator().HasColumn(&ModelGatewayProfitRatioRecommendation{}, column), column)
	}
}
