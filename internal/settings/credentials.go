// Package settings stores single-operator provider credentials server-side.
// The encryption key is deployment configuration, never accepted over HTTP.
package settings

import (
	"context"
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
)

var Names = []string{"DEEPSEEK_API_KEY", "TUSHARE_TOKEN", "TAVILY_API_KEY"}

var ErrReadOnly = errors.New("credential storage is disabled")

type Status struct {
	Configured map[string]bool `json:"configured"`
	Editable   bool            `json:"editable"`
}

type Store struct {
	mu   sync.Mutex
	path string
	key  []byte
}

// NewStore accepts exactly 32 random bytes encoded as 64 hex characters.
// An empty key permits status-only mode backed by environment variables.
func NewStore(path, masterKeyHex string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("credential path is required")
	}
	if masterKeyHex == "" {
		return &Store{path: path}, nil
	}
	key, err := hex.DecodeString(masterKeyHex)
	if err != nil || len(key) != 32 {
		return nil, errors.New("credential master key must be 64 hex characters")
	}
	return &Store{path: path, key: key}, nil
}

func validName(k string) bool {
	for _, name := range Names {
		if k == name {
			return true
		}
	}
	return false
}

// Load returns a detached plaintext map, solely for constructing a new workflow.
// No route may serialize this map or emit it to a log.
func (s *Store) Load(ctx context.Context) (map[string]string, error) {
	if s == nil {
		return nil, errors.New("credential store is nil")
	}
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}
func (s *Store) loadLocked() (map[string]string, error) {
	result := make(map[string]string)
	if len(s.key) == 0 {
		return result, nil
	}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, errors.New("credential file unavailable")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, errors.New("credential file has unsafe permissions or format")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, errors.New("credential file unavailable")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, errors.New("invalid credential key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("invalid credential cipher")
	}
	if len(data) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("credential file is corrupt")
	}
	plaintext, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte("aegis-provider-credentials-v1"))
	if err != nil {
		return nil, errors.New("credential file decryption failed")
	}
	if err := json.Unmarshal(plaintext, &result); err != nil {
		return nil, errors.New("credential file is corrupt")
	}
	for k, v := range result {
		if !validName(k) || strings.TrimSpace(v) == "" {
			return nil, errors.New("credential file contains invalid entries")
		}
	}
	return result, nil
}

// Status returns only availability booleans, never values or partial masks.
func (s *Store) Status(ctx context.Context) (Status, error) {
	values, err := s.Load(ctx)
	if err != nil {
		return Status{}, err
	}
	result := Status{Configured: make(map[string]bool), Editable: len(s.key) == 32}
	for _, name := range Names {
		result.Configured[name] = values[name] != "" || strings.TrimSpace(os.Getenv(name)) != ""
	}
	return result, nil
}

// Update accepts an allowlisted patch. Empty values are not supported; use
// clear to explicitly remove a stored override (environment fallback remains).
func (s *Store) Update(ctx context.Context, changes map[string]string, clear []string) (Status, error) {
	if s == nil || len(s.key) != 32 {
		return Status{}, ErrReadOnly
	}
	if ctx == nil {
		return Status{}, errors.New("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	if len(changes) == 0 && len(clear) == 0 {
		return Status{}, errors.New("no credential changes")
	}
	for k, v := range changes {
		if !validName(k) || len(v) < 8 || len(v) > 4096 || strings.TrimSpace(v) != v || strings.ContainsAny(v, "\r\n\x00") {
			return Status{}, errors.New("invalid credential update")
		}
	}
	for _, k := range clear {
		if !validName(k) {
			return Status{}, errors.New("unknown credential name")
		}
		if _, ok := changes[k]; ok {
			return Status{}, errors.New("conflicting credential change")
		}
	}
	s.mu.Lock()
	values, err := s.loadLocked()
	if err != nil {
		s.mu.Unlock()
		return Status{}, err
	}
	for k, v := range changes {
		values[k] = v
	}
	for _, k := range clear {
		delete(values, k)
	}
	payload, err := json.Marshal(values)
	if err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("cannot encode credentials")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("invalid credential key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("invalid credential cipher")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("cannot encrypt credentials")
	}
	ciphertext := append(nonce, aead.Seal(nil, nonce, payload, []byte("aegis-provider-credentials-v1"))...)
	dir := filepath.Dir(s.path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("credential directory unavailable")
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm()&0077 != 0 {
		s.mu.Unlock()
		return Status{}, errors.New("credential directory has unsafe permissions")
	}
	file, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("credential file unavailable")
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
		s.mu.Unlock()
		return Status{}, errors.New("cannot persist credentials")
	}
	if err = os.Rename(file.Name(), s.path); err != nil {
		s.mu.Unlock()
		return Status{}, errors.New("cannot commit credentials")
	}
	s.mu.Unlock()
	return s.Status(ctx)
}

func (s *Store) String() string {
	return fmt.Sprintf("CredentialStore{encrypted:%t}", s != nil && len(s.key) == 32)
}
