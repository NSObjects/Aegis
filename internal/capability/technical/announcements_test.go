package technical

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func announcementsClient(t *testing.T, handler http.HandlerFunc) *TushareClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewTushareClient("fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	client.endpoint = server.URL
	return client
}

func TestListAnnouncementsParsesAndValidates(t *testing.T) {
	client := announcementsClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readAllBody(t, r)
		if strings.Contains(body, `"ts_code":"600519.SH"`) {
			_, _ = w.Write([]byte(`{"code":0,"msg":"","data":{"fields":["ann_date","title"],"items":[["20261023","关于回购股份的公告"],["20261020","董事会决议公告"]]}}`))
			return
		}
		if strings.Contains(body, `"ts_code":"000001.SZ"`) {
			_, _ = w.Write([]byte(`{"code":0,"msg":"","data":{"fields":["ann_date","title"],"items":[]}}`))
			return
		}
		http.Error(w, "provider down", http.StatusBadGateway)
	})
	ctx := context.Background()
	got, err := ListAnnouncements(ctx, client, "600519.SH", "20261020", "20261023")
	if err != nil || len(got) != 2 || got[0].Title != "关于回购股份的公告" || got[0].AnnDate != "20261023" {
		t.Fatalf("announcements: %+v %v", got, err)
	}
	empty, err := ListAnnouncements(ctx, client, "000001.SZ", "20261020", "20261023")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty is valid: %+v %v", empty, err)
	}
	if _, err := ListAnnouncements(ctx, client, "600519", "20261020", "20261023"); err == nil {
		t.Fatal("bad symbol accepted")
	}
	if _, err := ListAnnouncements(ctx, client, "600519.SH", "20261023", "20261020"); err == nil {
		t.Fatal("inverted window accepted")
	}
	if _, err := ListAnnouncements(ctx, client, "600519.SH", "2026102", "20261023"); err == nil {
		t.Fatal("bad date accepted")
	}
}
