package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/capability/technical"
)

type digestMonitorSource struct {
	announcements []technical.Announcement
	err           error
}

func (s *digestMonitorSource) Observation(context.Context, string, string) (technical.HistoricalObservation, bool, error) {
	return technical.HistoricalObservation{}, false, errors.New("unexpected observation")
}

func (s *digestMonitorSource) Announcements(context.Context, string, string, string) ([]technical.Announcement, error) {
	return s.announcements, s.err
}

func TestResearchDigestMonitorToRunAndSourceOutage(t *testing.T) {
	dir := t.TempDir()
	monitors, err := application.NewFileMonitorStore(filepath.Join(dir, "monitors"))
	if err != nil {
		t.Fatal(err)
	}
	triggers, err := application.NewFileTriggerStore(filepath.Join(dir, "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	day := time.Now().In(zone)
	prior := day.AddDate(0, 0, -1)
	previousCompletion := day.Add(-2 * time.Hour)
	const newRunID, priorRunID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runs := &fakeRuns{
		rec: application.RunRecord{ID: newRunID, State: application.RunRunning, Symbol: "600519.SH"},
		listRecords: []application.RunRecord{{
			ID: priorRunID, State: application.RunCompleted, Symbol: "600519.SH", FinishedAt: &previousCompletion,
		}},
	}
	router, err := application.NewTriggerRouter(triggers, runs, application.ModeDecision, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	source := &digestMonitorSource{err: errors.New("provider unavailable")}
	monitor, err := application.NewMonitor(monitors, source)
	if err != nil {
		t.Fatal(err)
	}
	monitor.WithTriggerRouter(router)
	rule := application.MonitorRule{
		ID: "0123456789abcdef", Symbol: "600519.SH", Kind: application.MonitorAnnouncement,
		Enabled: true, CreatedAt: prior, UpdatedAt: prior,
	}
	if err := monitors.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	const layout = "20060102"
	if err := monitor.Tick(ctx, prior.Format(layout)); err != nil {
		t.Fatal(err)
	}
	source.err = nil
	source.announcements = []technical.Announcement{{AnnDate: day.Format(layout), Title: "回购公告"}}
	// A retry must honor the prior day's recorded outage rather than start a run.
	for _, date := range []time.Time{prior, day, day} {
		if err := monitor.Tick(ctx, date.Format(layout)); err != nil {
			t.Fatal(err)
		}
	}
	if runs.submitCalls != 1 {
		t.Fatalf("got %d submissions, want exactly 1", runs.submitCalls)
	}
	runs.rec.State = application.RunCompleted
	if err := router.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	if err := monitors.DeleteRule(rule.ID); err != nil {
		t.Fatal(err)
	}
	h := mkHandler(t, runs, application.ModeDecision)
	h.now = func() time.Time { return day }
	h.WithTriggerController(triggers).WithResearchDigest(monitors, zone)
	path := "/api/v1/research-digest"
	if w := apiRequest(t, h, http.MethodGet, path, "", false, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated response: %d", w.Code)
	}
	for _, tc := range []struct {
		period string
		count  int
		outage int
	}{{"daily", 1, 0}, {"weekly", 2, 1}} {
		w := apiRequest(t, h, http.MethodGet, path+"?period="+tc.period, "", true, nil)
		var reply struct {
			Alerts  []digestAlert `json:"alerts"`
			Outages int           `json:"source_outages"`
		}
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.period, w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		if len(reply.Alerts) != tc.count || reply.Outages != tc.outage {
			t.Fatalf("%s: %+v", tc.period, reply)
		}
		alert := reply.Alerts[0]
		if alert.RunID != newRunID || alert.PreviousRunID != priorRunID || alert.State != application.ScheduleCompleted || alert.TriggerID == "" {
			t.Fatalf("missing audit or report references: %+v", alert)
		}
		if tc.outage == 1 && (reply.Alerts[1].Outcome != application.MonitorSourceDown || reply.Alerts[1].RunID != "") {
			t.Fatalf("incorrect outage alert: %+v", reply.Alerts[1])
		}
	}
}
