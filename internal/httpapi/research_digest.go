package httpapi

import (
	"errors"
	"net/http"
	"os"
	"time"

	"Aegis/internal/application"
)

type ResearchDigestController interface {
	EventsBetween(from, through string) ([]application.MonitorEvent, error)
}

func (h *Handler) WithResearchDigest(source ResearchDigestController, location *time.Location) *Handler {
	h.digest, h.digestZone = source, location
	return h
}

type digestAlert struct {
	application.MonitorEvent
	TriggerID     string                       `json:"trigger_id,omitempty"`
	Route         application.ResearchRoute    `json:"route,omitempty"`
	State         application.ScheduleRunState `json:"trigger_state,omitempty"`
	RunID         string                       `json:"run_id,omitempty"`
	PreviousRunID string                       `json:"previous_run_id,omitempty"`
	FailureCode   string                       `json:"failure_code,omitempty"`
}

func (h *Handler) researchDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	if h.digest == nil || h.digestZone == nil || h.triggers == nil {
		http.Error(w, "research digest unavailable", http.StatusServiceUnavailable)
		return
	}
	period := r.URL.Query().Get("period")
	offset := 0
	switch period {
	case "", "daily":
		period = "daily"
	case "weekly":
		offset = -6
	default:
		http.Error(w, "invalid digest period", http.StatusBadRequest)
		return
	}
	const dateLayout = "20060102"
	today := h.now().In(h.digestZone).Format(dateLayout)
	through := r.URL.Query().Get("through")
	if through == "" {
		through = today
	}
	end, err := time.ParseInLocation(dateLayout, through, h.digestZone)
	if err != nil || through > today {
		http.Error(w, "invalid digest date", http.StatusBadRequest)
		return
	}
	from := end.AddDate(0, 0, offset).Format(dateLayout)
	events, err := h.digest.EventsBetween(from, through)
	if err != nil {
		http.Error(w, "monitor history unavailable", http.StatusInternalServerError)
		return
	}
	alerts := make([]digestAlert, 0, len(events))
	sourceOutages := 0
	var history []application.RunRecord
	for _, event := range events {
		switch event.Outcome {
		case application.MonitorSourceDown:
			sourceOutages++
		case application.MonitorFired:
		default:
			continue
		}
		item := digestAlert{MonitorEvent: event}
		if event.Outcome == application.MonitorFired {
			if history == nil {
				history, err = h.runs.List(r.Context())
				if err != nil {
					http.Error(w, "research history unavailable", http.StatusInternalServerError)
					return
				}
				if history == nil {
					history = []application.RunRecord{}
				}
			}
			var previousAt time.Time
			for _, run := range history {
				if run.Symbol == event.Symbol && run.State == application.RunCompleted && run.FinishedAt != nil &&
					!run.FinishedAt.After(event.CreatedAt) && run.FinishedAt.After(previousAt) {
					item.PreviousRunID, previousAt = run.ID, *run.FinishedAt
				}
			}
			trigger, err := h.triggers.Get(event.TriggerID())
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				http.Error(w, "trigger history unavailable", http.StatusInternalServerError)
				return
			}
			if err == nil {
				item.TriggerID, item.Route, item.State, item.RunID = trigger.ID, trigger.Route, trigger.State, trigger.RunID
				if trigger.State == application.ScheduleFailed {
					item.FailureCode = digestFailureCode(trigger.FailureNote)
				}
			}
		}
		alerts = append(alerts, item)
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"period": period, "from": from, "through": through, "alerts": alerts, "source_outages": sourceOutages,
	})
}

// Older trigger files can contain arbitrary error text. Only fixed codes
// are eligible for the workbench feed.
func digestFailureCode(note string) string {
	switch note {
	case application.FailureResearch, application.FailureInterrupted, application.FailureInvalidResult,
		application.FailurePersistence, application.FailureTimeout,
		"RESEARCH_CANCELLED", "RESEARCH_SUBMISSION_FAILED", "TRIGGER_INPUT_INVALID":
		return note
	default:
		return "TRIGGER_FAILED"
	}
}
