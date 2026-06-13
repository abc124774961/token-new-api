package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserContactModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldUsingSQLite := common.UsingSQLite
	oldRedisEnabled := common.RedisEnabled
	common.UsingSQLite = true
	common.RedisEnabled = false
	initCol()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.RedisEnabled = oldRedisEnabled
		initCol()
	})
	return db
}

func TestSearchUsersMatchesContactFields(t *testing.T) {
	db := setupUserContactModelTestDB(t)
	require.NoError(t, db.Create(&User{
		Username:     "account-001",
		DisplayName:  "Legacy Name",
		Password:     "password",
		Role:         common.RoleCommonUser,
		Status:       common.UserStatusEnabled,
		Group:        "default",
		ContactName:  "运营昵称",
		ContactEmail: "contact@example.com",
		ContactQQ:    "123456789",
		ContactOther: "telegram-user",
	}).Error)

	users, total, err := SearchUsers("123456789", "", 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.Equal(t, "account-001", users[0].Username)

	users, total, err = SearchUsers("telegram-user", "default", 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.Equal(t, "运营昵称", users[0].ContactName)
}

func TestSearchUsersAvoidsShortContactFuzzyMatches(t *testing.T) {
	db := setupUserContactModelTestDB(t)
	require.NoError(t, db.Create(&User{
		Username:     "account-short-contact",
		DisplayName:  "Short Contact",
		Password:     "password",
		Role:         common.RoleCommonUser,
		Status:       common.UserStatusEnabled,
		Group:        "default",
		ContactOther: "wx",
	}).Error)

	users, total, err := SearchUsers("w", "", 0, 20)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, users)

	users, total, err = SearchUsers("wx", "", 0, 20)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, users)
}

func TestUserPreferredDisplayNameUsesContactPriority(t *testing.T) {
	user := &User{
		Username:     "account-001",
		DisplayName:  "Display",
		ContactEmail: "contact@example.com",
		ContactQQ:    "123456789",
	}
	require.Equal(t, "123456789", user.PreferredDisplayName())

	user.ContactName = "Preferred"
	require.Equal(t, "Preferred", user.PreferredDisplayName())
}
