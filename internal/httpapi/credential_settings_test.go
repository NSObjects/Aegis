package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"Aegis/internal/application"
	"Aegis/internal/settings"
)

func TestCredentialEndpointRequiresBearerAndNeverEchoes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.enc")
	key := hex.EncodeToString([]byte("12345678901234567890123456789012"))
	store, err := settings.NewStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	h := mkHandler(t, &fakeRuns{rec: testRecord()}, application.ModePortfolio).WithCredentialStore(store)
	endpoint := "/api/v1/settings/credentials"
	anonymous := apiRequest(t, h, "GET", endpoint, "", false, nil)
	if anonymous.Code != 401 {
		t.Fatalf("anonymous credentials status %d", anonymous.Code)
	}
	supplied := "provider-secret-exactly-for-test"
	input := `{"changes":{"DEEPSEEK_API_KEY":"` + supplied + `"}}`
	saved := apiRequest(t, h, http.MethodPut, endpoint, input, true, nil)
	if saved.Code != 200 {
		t.Fatalf("credential save %d: %s", saved.Code, saved.Body.String())
	}
	if strings.Contains(saved.Body.String(), supplied) {
		t.Fatal("credential echoed from update")
	}
	status := apiRequest(t, h, "GET", endpoint, "", true, nil)
	if status.Code != 200 || strings.Contains(status.Body.String(), supplied) {
		t.Fatal("credential echoed from GET")
	}
	var result settings.Status
	if err := json.Unmarshal(status.Body.Bytes(), &result); err != nil || !result.Configured["DEEPSEEK_API_KEY"] {
		t.Fatalf("bad status %+v, %v", result, err)
	}
	invalid := apiRequest(t, h, "PUT", endpoint, `{"changes":{"UNKNOWN":"bad-secret-string"}}`, true, nil)
	if invalid.Code != 400 {
		t.Fatalf("unknown key was accepted %d", invalid.Code)
	}
	clear := apiRequest(t, h, "PUT", endpoint, `{"clear":["DEEPSEEK_API_KEY"]}`, true, nil)
	if clear.Code != 200 {
		t.Fatalf("clear rejected %d", clear.Code)
	}
}
