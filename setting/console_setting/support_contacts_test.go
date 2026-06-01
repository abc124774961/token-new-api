package console_setting

import (
	"strings"
	"testing"
)

func TestValidateSupportContacts(t *testing.T) {
	valid := `[
		{"id":2,"type":"email","title":"邮件支持","value":"support@example.com","url":"mailto:support@example.com?subject=Help","priority":20,"enabled":true},
		{"id":1,"type":"telegram","title":"Telegram","description":"工作日在线","url":"https://t.me/example","qrcode":"https://example.com/qr.png","priority":10}
	]`
	if err := ValidateConsoleSettings(valid, "SupportContacts"); err != nil {
		t.Fatalf("valid support contacts should pass: %v", err)
	}

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "too many",
			payload: `[` + strings.TrimRight(strings.Repeat(`{"type":"email","title":"Email","value":"a@example.com"},`, 21), ",") + `]`,
			want:    "不能超过20个",
		},
		{
			name:    "missing title",
			payload: `[{"type":"email","value":"a@example.com"}]`,
			want:    "缺少标题字段",
		},
		{
			name:    "missing target",
			payload: `[{"type":"email","title":"Email"}]`,
			want:    "至少需要填写值、链接或二维码之一",
		},
		{
			name:    "bad type",
			payload: `[{"type":"sms","title":"SMS","value":"10086"}]`,
			want:    "类型值不合法",
		},
		{
			name:    "unsafe url",
			payload: `[{"type":"custom","title":"Bad","url":"javascript:alert(1)"}]`,
			want:    "包含不允许的内容",
		},
		{
			name:    "bad qrcode scheme",
			payload: `[{"type":"wechat","title":"微信","qrcode":"data:image/png;base64,abc"}]`,
			want:    "二维码仅支持 http 或 https 地址",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConsoleSettings(tt.payload, "SupportContacts")
			if err == nil {
				t.Fatalf("expected error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestGetSupportContactsFiltersAndSorts(t *testing.T) {
	cs := GetConsoleSetting()
	oldContacts := cs.SupportContacts
	defer func() {
		cs.SupportContacts = oldContacts
	}()

	cs.SupportContacts = `[
		{"id":3,"type":"docs","title":"文档","url":"https://docs.example.com","priority":30,"enabled":true},
		{"id":1,"type":"telegram","title":"Telegram","url":"https://t.me/example","priority":10},
		{"id":2,"type":"email","title":"邮件","value":"support@example.com","priority":20,"enabled":false}
	]`

	contacts := GetSupportContacts()
	if len(contacts) != 2 {
		t.Fatalf("len(contacts) = %d, want 2", len(contacts))
	}
	if contacts[0].ID != 1 || contacts[1].ID != 3 {
		t.Fatalf("contacts order = [%d, %d], want [1, 3]", contacts[0].ID, contacts[1].ID)
	}
	if !contacts[0].Enabled {
		t.Fatalf("missing enabled field should default to true")
	}
}
