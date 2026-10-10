package settings

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptedStatusUpdateAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secure", "secrets.enc")
	key := hex.EncodeToString([]byte("12345678901234567890123456789012"))
	store, err := NewStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := store.Status(ctx)
	if err != nil || !before.Editable {
		t.Fatalf("status %+v %v", before, err)
	}
	secret := "test-tushare-token-private"
	status, err := store.Update(ctx, map[string]string{"TUSHARE_TOKEN": secret}, nil)
	if err != nil || !status.Configured["TUSHARE_TOKEN"] {
		t.Fatalf("save %+v %v", status, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), secret) {
		t.Fatal("plaintext leaked to credential file")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("unsafe permissions")
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0700 {
		t.Fatal("unsafe directory permissions")
	}
	recovered, _ := NewStore(path, key)
	val, err := recovered.Load(ctx)
	if err != nil || val["TUSHARE_TOKEN"] != secret {
		t.Fatalf("reload %v %v", val, err)
	}
	_, err = store.Update(ctx, map[string]string{"BAD_KEY": "secret-secret"}, nil)
	if err == nil {
		t.Fatal("accepted unknown key")
	}
	_, err = store.Update(ctx, nil, []string{"TUSHARE_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := store.Load(ctx)
	if after["TUSHARE_TOKEN"] != "" {
		t.Fatal("clear failed")
	}
	wrong, _ := NewStore(path, hex.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyzzzzzzz")))
	if _, err = wrong.Load(ctx); err == nil {
		t.Fatal("wrong master key decrypted")
	}
}
func TestFailClosedAndReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.enc")
	s, err := NewStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(context.Background(), map[string]string{"DEEPSEEK_API_KEY": "valid-secret-key"}, nil); err != ErrReadOnly {
		t.Fatal("writable without master key")
	}
	if _, err = NewStore(path, "bad"); err == nil {
		t.Fatal("accepted weak master key")
	}
	ctx := context.Background()
	if err = os.Symlink("/etc/hosts", path); err != nil {
		t.Fatal(err)
	}
	k, _ := NewStore(path, hex.EncodeToString([]byte("12345678901234567890123456789012")))
	if _, err = k.Load(ctx); err == nil {
		t.Fatal("followed credential symlink")
	}
}
