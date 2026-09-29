package event

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	capevent "Aegis/internal/capability/event"
)

// EvidenceQuote is model-produced grounding intent. Its document identity and
// quote are untrusted until ResolveEvidenceGroundings verifies them.
type EvidenceQuote struct {
	DocumentID string `json:"document_id"`
	Quote      string `json:"quote"`
}

// EvidenceGrounding is a verified exact span in CanonicalDocument.Content.
// StartByte is inclusive and EndByte is exclusive.
type EvidenceGrounding struct {
	DocumentID string `json:"document_id"`
	Quote      string `json:"quote"`
	StartByte  int    `json:"start_byte"`
	EndByte    int    `json:"end_byte"`
}

func ResolveEvidenceGroundings(
	quotes []EvidenceQuote,
	documents map[string]capevent.CanonicalDocument,
) ([]EvidenceGrounding, error) {
	if len(quotes) == 0 {
		return nil, errors.New(
			"at least one evidence quote is required",
		)
	}

	seen := make(map[string]struct{}, len(quotes))
	result := make(
		[]EvidenceGrounding,
		0,
		len(quotes),
	)
	for index, raw := range quotes {
		documentID := strings.TrimSpace(
			raw.DocumentID,
		)
		quote := strings.TrimSpace(raw.Quote)
		if documentID == "" {
			return nil, fmt.Errorf(
				"evidence quote %d document_id is required",
				index,
			)
		}
		if quote == "" {
			return nil, fmt.Errorf(
				"evidence quote %d quote is required",
				index,
			)
		}

		document, ok := documents[documentID]
		if !ok {
			return nil, fmt.Errorf(
				"evidence quote %d references unknown canonical document %q",
				index,
				documentID,
			)
		}

		key := documentID + "\x00" + quote
		if _, duplicate := seen[key]; duplicate {
			continue
		}

		start := strings.Index(
			document.Content,
			quote,
		)
		if start < 0 {
			return nil, fmt.Errorf(
				"evidence quote %d is not an exact substring of canonical document %q",
				index,
				documentID,
			)
		}
		if strings.Index(
			document.Content[start+1:],
			quote,
		) >= 0 {
			return nil, fmt.Errorf(
				"evidence quote %d is ambiguous in canonical document %q",
				index,
				documentID,
			)
		}

		seen[key] = struct{}{}
		result = append(
			result,
			EvidenceGrounding{
				DocumentID: documentID,
				Quote:      quote,
				StartByte:  start,
				EndByte:    start + len(quote),
			},
		)
	}

	sort.Slice(
		result,
		func(i int, j int) bool {
			if result[i].DocumentID !=
				result[j].DocumentID {
				return result[i].DocumentID <
					result[j].DocumentID
			}
			if result[i].StartByte !=
				result[j].StartByte {
				return result[i].StartByte <
					result[j].StartByte
			}
			if result[i].EndByte !=
				result[j].EndByte {
				return result[i].EndByte <
					result[j].EndByte
			}
			return result[i].Quote <
				result[j].Quote
		},
	)
	return result, nil
}

func groundingDocumentIDs(
	groundings []EvidenceGrounding,
) []string {
	values := make([]string, 0, len(groundings))
	for _, grounding := range groundings {
		values = append(
			values,
			grounding.DocumentID,
		)
	}
	return uniqueSortedStrings(values)
}
