package secret

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"gorm.io/gorm"
)

const (
	SecretTypeCodexOAuth        = "codex_oauth"
	SecretTypeOAuthRefreshToken = "oauth_refresh_token"
	SecretTypeRefreshToken      = "refresh_token"
	cipherVersion               = "v1"
)

var ErrSecretKeyRequired = errors.New("AUTOMATION_SECRET_KEY is required for secret operations")

type Service struct {
	db        *gorm.DB
	masterKey []byte
}

type CreateRequest struct {
	SecretType string
	ScopeRef   string
	Value      string
	ExpiresAt  int64
}

type Plaintext struct {
	Secret model.Secret
	Value  string
}

func New(database *gorm.DB, secretKey string) *Service {
	return &Service{db: database, masterKey: deriveKey(secretKey)}
}

func (s *Service) Enabled() bool {
	return len(s.masterKey) == 32
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*model.Secret, error) {
	req.SecretType = strings.ToLower(strings.TrimSpace(req.SecretType))
	req.ScopeRef = strings.TrimSpace(req.ScopeRef)
	if req.SecretType == "" {
		return nil, errors.New("secret_type is required")
	}
	if strings.TrimSpace(req.Value) == "" {
		return nil, errors.New("value is required")
	}
	ciphertext, err := s.encrypt(req.Value)
	if err != nil {
		return nil, err
	}
	record := model.Secret{
		SecretType:  req.SecretType,
		ScopeRef:    req.ScopeRef,
		Ciphertext:  ciphertext,
		Fingerprint: s.fingerprint(req.SecretType, req.ScopeRef, req.Value),
		ExpiresAt:   req.ExpiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) GetPlaintext(ctx context.Context, secretRef string) (*Plaintext, error) {
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return nil, errors.New("secret_ref is required")
	}
	var record model.Secret
	if err := s.db.WithContext(ctx).Where("secret_ref = ?", secretRef).First(&record).Error; err != nil {
		return nil, err
	}
	value, err := s.decrypt(record.Ciphertext)
	if err != nil {
		return nil, err
	}
	return &Plaintext{Secret: record, Value: value}, nil
}

func (s *Service) FindLatestPlaintext(ctx context.Context, scopeRef string, secretTypes ...string) (*Plaintext, error) {
	scopeRef = strings.TrimSpace(scopeRef)
	if scopeRef == "" {
		return nil, errors.New("scope_ref is required")
	}
	types := normalizeSecretTypes(secretTypes)
	if len(types) == 0 {
		return nil, errors.New("secret_types is required")
	}
	var record model.Secret
	if err := s.db.WithContext(ctx).
		Where("scope_ref = ? AND secret_type IN ?", scopeRef, types).
		Order("id DESC").
		First(&record).Error; err != nil {
		return nil, err
	}
	value, err := s.decrypt(record.Ciphertext)
	if err != nil {
		return nil, err
	}
	return &Plaintext{Secret: record, Value: value}, nil
}

func (s *Service) LinkJobSecret(ctx context.Context, jobID string, secretRef string, alias string) error {
	jobID = strings.TrimSpace(jobID)
	secretRef = strings.TrimSpace(secretRef)
	alias = strings.ToLower(strings.TrimSpace(alias))
	if jobID == "" || secretRef == "" {
		return errors.New("job_id and secret_ref are required")
	}
	var existing model.JobSecretRef
	err := s.db.WithContext(ctx).
		Where("job_id = ? AND secret_ref = ? AND alias = ?", jobID, secretRef, alias).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.WithContext(ctx).Create(&model.JobSecretRef{
		JobID:     jobID,
		SecretRef: secretRef,
		Alias:     alias,
	}).Error
}

func (s *Service) GetJobLinkedSecret(ctx context.Context, jobID string, alias string) (*model.Secret, error) {
	jobID = strings.TrimSpace(jobID)
	alias = strings.ToLower(strings.TrimSpace(alias))
	if jobID == "" {
		return nil, errors.New("job_id is required")
	}
	var ref model.JobSecretRef
	err := s.db.WithContext(ctx).
		Where("job_id = ? AND alias = ?", jobID, alias).
		Order("id DESC").
		First(&ref).Error
	if err != nil {
		return nil, err
	}
	var record model.Secret
	if err := s.db.WithContext(ctx).Where("secret_ref = ?", ref.SecretRef).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) encrypt(plaintext string) (string, error) {
	if !s.Enabled() {
		return "", ErrSecretKeyRequired
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(plaintext), []byte(cipherVersion))
	payload := append(nonce, sealed...)
	return cipherVersion + "." + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (s *Service) decrypt(ciphertext string) (string, error) {
	if !s.Enabled() {
		return "", ErrSecretKeyRequired
	}
	version, encoded, ok := strings.Cut(strings.TrimSpace(ciphertext), ".")
	if !ok || version != cipherVersion || encoded == "" {
		return "", errors.New("unsupported secret ciphertext format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) <= aead.NonceSize() {
		return "", errors.New("invalid secret ciphertext")
	}
	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, []byte(cipherVersion))
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

func (s *Service) fingerprint(secretType string, scopeRef string, value string) string {
	mac := hmac.New(sha256.New, s.masterKey)
	mac.Write([]byte(strings.ToLower(strings.TrimSpace(secretType))))
	mac.Write([]byte(":"))
	mac.Write([]byte(strings.TrimSpace(scopeRef)))
	mac.Write([]byte(":"))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func deriveKey(secretKey string) []byte {
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(secretKey))
	return sum[:]
}

func normalizeSecretTypes(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	return result
}
