package secret

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/token-account-automation/internal/db"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return database
}

func TestCreateAndDecryptSecret(t *testing.T) {
	database := testDB(t)
	svc := New(database, "unit-test-secret-key")
	ctx := context.Background()

	record, err := svc.Create(ctx, CreateRequest{
		SecretType: SecretTypeOAuthRefreshToken,
		ScopeRef:   "auto_target_1",
		Value:      "refresh-token-a",
	})
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}
	if record.SecretRef == "" || record.Fingerprint == "" {
		t.Fatalf("missing secret metadata: %+v", record)
	}
	var stored model.Secret
	if err := database.Where("secret_ref = ?", record.SecretRef).First(&stored).Error; err != nil {
		t.Fatalf("load stored secret: %v", err)
	}
	if strings.Contains(stored.Ciphertext, "refresh-token-a") {
		t.Fatalf("ciphertext contains plaintext: %s", stored.Ciphertext)
	}

	plain, err := svc.GetPlaintext(ctx, record.SecretRef)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if plain.Value != "refresh-token-a" {
		t.Fatalf("unexpected plaintext: %q", plain.Value)
	}
}

func TestSecretRequiresKey(t *testing.T) {
	svc := New(testDB(t), "")
	_, err := svc.Create(context.Background(), CreateRequest{
		SecretType: SecretTypeOAuthRefreshToken,
		ScopeRef:   "auto_target_1",
		Value:      "refresh-token-a",
	})
	if err == nil || !strings.Contains(err.Error(), "AUTOMATION_SECRET_KEY") {
		t.Fatalf("expected key error, got %v", err)
	}
}
