package settings

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"Aegis/internal/portfolio"
)

var (
	ErrAccountImportInvalid   = errors.New("invalid or stale portfolio input")
	ErrAccountVersionConflict = errors.New("portfolio version already contains different data")
	ErrAccountVersionMissing  = errors.New("portfolio version not found")
	ErrAccountStoreCapacity   = errors.New("portfolio store capacity reached")
)

const (
	maxAccountFile     = 8 << 20
	maxAccountVersions = 256
	accountAAD         = "aegis-research-accounts-v1"
)

type AccountImport struct {
	Snapshot portfolio.ResearchAccountSnapshot `json:"snapshot"`
	Policy   *portfolio.ResearchRiskPolicy     `json:"policy,omitempty"`
}

type accountPolicyVersion struct {
	AccountID string                       `json:"account_id"`
	Policy    portfolio.ResearchRiskPolicy `json:"policy"`
}

type accountArchive struct {
	Snapshots []portfolio.ResearchAccountSnapshot `json:"snapshots"`
	Policies  []accountPolicyVersion              `json:"policies"`
}

type PortfolioAccountStore struct {
	mu     sync.Mutex
	path   string
	cipher cipher.AEAD
	maxAge time.Duration
}

// NewPortfolioAccountStore uses a dedicated deployment key. Callers enable
// the import surface only when authenticated and a key has been provisioned.
func NewPortfolioAccountStore(path, keyHex string, maxAge time.Duration) (*PortfolioAccountStore, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 || strings.TrimSpace(path) == "" || maxAge <= 0 {
		return nil, errors.New("portfolio storage requires a 64-hex key, path and positive maximum age")
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &PortfolioAccountStore{path: path, cipher: aead, maxAge: maxAge}, nil
}

func (s *PortfolioAccountStore) Import(input AccountImport, now time.Time) (portfolio.ResearchAccountView, bool, error) {
	snapshot := input.Snapshot
	if now.IsZero() || snapshot.Book.State == portfolio.StateNotProvided || snapshot.Source == nil ||
		snapshot.Source.Kind != portfolio.ResearchSourceOperator || !snapshot.Source.AvailableAt.IsZero() {
		return portfolio.ResearchAccountView{}, false, ErrAccountImportInvalid
	}
	source := *snapshot.Source
	source.AvailableAt = now.UTC()
	snapshot.Source = &source
	var policy *portfolio.ResearchRiskPolicy
	if input.Policy != nil {
		if input.Policy.Source.Kind != portfolio.ResearchSourceOperator || !input.Policy.Source.AvailableAt.IsZero() {
			return portfolio.ResearchAccountView{}, false, ErrAccountImportInvalid
		}
		copy := *input.Policy
		copy.Source.AvailableAt = now.UTC()
		policy = &copy
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	archive, err := s.load()
	if err != nil {
		return portfolio.ResearchAccountView{}, false, err
	}
	previous := archive.snapshot(snapshot.AccountID, snapshot.Version)
	if previous != nil {
		source.AvailableAt = previous.Source.AvailableAt
		if !sameAccountData(previous, &snapshot) {
			return portfolio.ResearchAccountView{}, false, ErrAccountVersionConflict
		}
	}
	created := previous == nil
	if created {
		archive.Snapshots = append(archive.Snapshots, snapshot)
	}
	if policy != nil {
		existing := archive.policy(snapshot.AccountID, policy.Version)
		if existing != nil {
			policy.Source.AvailableAt = existing.Source.AvailableAt
			if !sameAccountData(existing, policy) {
				return portfolio.ResearchAccountView{}, false, ErrAccountVersionConflict
			}
		} else {
			archive.Policies = append(archive.Policies, accountPolicyVersion{AccountID: snapshot.AccountID, Policy: *policy})
			created = true
		}
	}
	view, err := portfolio.AssessResearchAccount(snapshot, policy, now, s.maxAge)
	if err != nil {
		return portfolio.ResearchAccountView{}, false, fmt.Errorf("%w: %v", ErrAccountImportInvalid, err)
	}
	if created {
		if err := s.save(archive); err != nil {
			return portfolio.ResearchAccountView{}, false, err
		}
	}
	return view, created, nil
}

func (s *PortfolioAccountStore) Get(accountID, accountVersion, policyVersion string, cutoff time.Time) (portfolio.ResearchAccountView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	archive, err := s.load()
	if err != nil {
		return portfolio.ResearchAccountView{}, err
	}
	snapshot := archive.snapshot(accountID, accountVersion)
	if snapshot == nil {
		return portfolio.ResearchAccountView{}, ErrAccountVersionMissing
	}
	var policy *portfolio.ResearchRiskPolicy
	if policyVersion != "" {
		policy = archive.policy(accountID, policyVersion)
		if policy == nil {
			return portfolio.ResearchAccountView{}, ErrAccountVersionMissing
		}
	}
	view, err := portfolio.AssessResearchAccount(*snapshot, policy, cutoff, s.maxAge)
	if err != nil {
		return portfolio.ResearchAccountView{}, ErrAccountImportInvalid
	}
	return view, nil
}

func (archive accountArchive) snapshot(id, version string) *portfolio.ResearchAccountSnapshot {
	for i := range archive.Snapshots {
		row := &archive.Snapshots[i]
		if row.AccountID == id && row.Version == version {
			return row
		}
	}
	return nil
}

func (archive accountArchive) policy(id, version string) *portfolio.ResearchRiskPolicy {
	for i := range archive.Policies {
		row := &archive.Policies[i]
		if row.AccountID == id && row.Policy.Version == version {
			return &row.Policy
		}
	}
	return nil
}

func sameAccountData(left, right any) bool {
	a, err := json.Marshal(left)
	if err != nil {
		return false
	}
	b, err := json.Marshal(right)
	return err == nil && bytes.Equal(a, b)
}

func (s *PortfolioAccountStore) load() (accountArchive, error) {
	var archive accountArchive
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return archive, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxAccountFile {
		return archive, errors.New("portfolio archive has unsafe permissions or format")
	}
	if err := privateAccountDir(filepath.Dir(s.path)); err != nil {
		return archive, err
	}
	data, err := os.ReadFile(s.path)
	nonceSize := s.cipher.NonceSize()
	if err != nil || len(data) < nonceSize+s.cipher.Overhead() || len(data) > maxAccountFile {
		return archive, errors.New("portfolio archive unavailable")
	}
	plaintext, err := s.cipher.Open(nil, data[:nonceSize], data[nonceSize:], []byte(accountAAD))
	if err != nil {
		return archive, errors.New("portfolio archive decryption failed")
	}
	defer clear(plaintext)
	if err := json.Unmarshal(plaintext, &archive); err != nil ||
		len(archive.Snapshots)+len(archive.Policies) > maxAccountVersions {
		return accountArchive{}, errors.New("invalid portfolio archive")
	}
	for _, snapshot := range archive.Snapshots {
		if snapshot.Source == nil {
			return accountArchive{}, errors.New("invalid portfolio archive")
		}
	}
	return archive, nil
}

func privateAccountDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("portfolio directory has unsafe permissions")
	}
	return nil
}

func (s *PortfolioAccountStore) save(archive accountArchive) error {
	if len(archive.Snapshots)+len(archive.Policies) > maxAccountVersions {
		return ErrAccountStoreCapacity
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		return err
	}
	defer clear(raw)
	if len(raw)+s.cipher.NonceSize()+s.cipher.Overhead() > maxAccountFile {
		return ErrAccountStoreCapacity
	}
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := append(nonce, s.cipher.Seal(nil, nonce, raw, []byte(accountAAD))...)
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := privateAccountDir(dir); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".account-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(ciphertext)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), s.path)
}
