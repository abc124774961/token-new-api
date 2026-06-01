package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
)

func TestGetStatusSupportContactsEnabledFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cs := console_setting.GetConsoleSetting()
	oldContacts := cs.SupportContacts
	oldEnabled := cs.SupportContactsEnabled
	defer func() {
		cs.SupportContacts = oldContacts
		cs.SupportContactsEnabled = oldEnabled
	}()

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	defer func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	}()

	cs.SupportContacts = `[{"id":1,"type":"email","title":"邮件支持","value":"support@example.com","priority":1,"enabled":true}]`
	cs.SupportContactsEnabled = true

	body := callGetStatusForSupportContactsTest(t)
	if body["support_contacts_enabled"] != true {
		t.Fatalf("support_contacts_enabled = %#v, want true", body["support_contacts_enabled"])
	}
	contacts, ok := body["support_contacts"].([]interface{})
	if !ok || len(contacts) != 1 {
		t.Fatalf("support_contacts = %#v, want one contact", body["support_contacts"])
	}

	cs.SupportContactsEnabled = false
	body = callGetStatusForSupportContactsTest(t)
	if body["support_contacts_enabled"] != false {
		t.Fatalf("support_contacts_enabled = %#v, want false", body["support_contacts_enabled"])
	}
	if _, ok := body["support_contacts"]; ok {
		t.Fatalf("support_contacts should be omitted when disabled: %#v", body["support_contacts"])
	}
}

func callGetStatusForSupportContactsTest(t *testing.T) map[string]interface{} {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := common.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success {
		t.Fatalf("success = false, body = %s", recorder.Body.String())
	}
	return payload.Data
}
