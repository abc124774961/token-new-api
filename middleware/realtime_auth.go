package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func RealtimeAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		username, _ := session.Get("username").(string)
		id, idOK := session.Get("id").(int)
		role, roleOK := session.Get("role").(int)
		status, statusOK := session.Get("status").(int)
		if !idOK || !roleOK || !statusOK || strings.TrimSpace(username) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthNotLoggedIn),
			})
			c.Abort()
			return
		}
		if status == common.UserStatusDisabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthUserBanned),
			})
			c.Abort()
			return
		}
		if role < common.RoleAdminUser {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
			})
			c.Abort()
			return
		}
		if rawUserID := strings.TrimSpace(c.Query("user_id")); rawUserID != "" {
			userID, err := strconv.Atoi(rawUserID)
			if err != nil || userID != id {
				c.JSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgAuthUserIdMismatch),
				})
				c.Abort()
				return
			}
		}
		c.Set("username", username)
		c.Set("role", role)
		c.Set("id", id)
		c.Set("group", session.Get("group"))
		c.Set("user_group", session.Get("group"))
		c.Next()
	}
}
