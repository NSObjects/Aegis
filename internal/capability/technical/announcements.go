package technical

import (
	"context"
	"fmt"
	"time"
)

// Announcement is one observed issuer disclosure from the provider. It is a
// pointer to what the source returned at query time; it is not a research
// conclusion and carries no model judgement.
type Announcement struct {
	AnnDate string `json:"ann_date"`
	Title   string `json:"title"`
}

// ListAnnouncements returns the issuer announcements for one symbol between
// start and end (inclusive, YYYYMMDD). Empty results are a valid answer only
// when the provider call itself succeeded: callers must distinguish "no
// announcements" from "source unavailable".
func ListAnnouncements(ctx context.Context, client *TushareClient, symbol, start, end string) ([]Announcement, error) {
	if client == nil {
		return nil, fmt.Errorf("tushare client required")
	}
	if len(symbol) != 9 || (!endsWithSuffix(symbol)) {
		return nil, fmt.Errorf("invalid announcement symbol %q", symbol)
	}
	for _, d := range []string{start, end} {
		if _, err := time.Parse("20060102", d); err != nil || len(d) != 8 {
			return nil, fmt.Errorf("invalid announcement date %q", d)
		}
	}
	if start > end {
		return nil, fmt.Errorf("announcement window inverted")
	}
	rows, err := client.query(ctx, "anns_d", map[string]any{"ts_code": symbol, "start_date": start, "end_date": end}, "ann_date", "title")
	if err != nil {
		return nil, fmt.Errorf("announcements query: %w", err)
	}
	out := make([]Announcement, 0, len(rows))
	for _, row := range rows {
		date, _ := row["ann_date"].(string)
		title, _ := row["title"].(string)
		if date == "" || title == "" {
			return nil, fmt.Errorf("announcement row missing fields")
		}
		out = append(out, Announcement{AnnDate: date, Title: title})
	}
	return out, nil
}

func endsWithSuffix(symbol string) bool {
	return hasSuffix(symbol, ".SH") || hasSuffix(symbol, ".SZ")
}
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
