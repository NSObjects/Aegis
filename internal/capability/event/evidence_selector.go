package event

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const defaultSemanticSelectionThreshold = 0.80

type EvidenceTarget struct {
	Symbol  string
	Name    string
	Aliases []string
}

type EvidenceTargetResolver interface {
	Resolve(
		ctx context.Context,
		task ResearchTask,
	) (EvidenceTarget, error)
}


type MapEvidenceTargetResolver map[string]EvidenceTarget

var _ EvidenceTargetResolver = MapEvidenceTargetResolver{}

func (r MapEvidenceTargetResolver) Resolve(
	_ context.Context,
	task ResearchTask,
) (EvidenceTarget, error) {
	symbol := strings.ToUpper(
		strings.TrimSpace(task.Symbol),
	)
	if symbol == "" {
		return EvidenceTarget{}, errors.New(
			"evidence target task symbol is required",
		)
	}

	target, ok := r[symbol]
	if !ok {
		for configuredSymbol, candidate := range r {
			if strings.EqualFold(
				strings.TrimSpace(configuredSymbol),
				symbol,
			) {
				target = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		return EvidenceTarget{}, fmt.Errorf(
			"evidence target %q is not configured",
			symbol,
		)
	}
	if strings.TrimSpace(target.Symbol) == "" {
		target.Symbol = symbol
	}
	return target, nil
}

type SemanticEvidenceMatch struct {
	RawDocumentID string  `json:"raw_document_id"`
	Relevant      bool    `json:"relevant"`
	Confidence    float64 `json:"confidence"`
	Reason        string  `json:"reason"`
}

type SemanticEvidenceMatcher interface {
	Match(
		ctx context.Context,
		target EvidenceTarget,
		documents []RawDocument,
	) ([]SemanticEvidenceMatch, error)
}

type HybridEvidenceSelector struct {
	TargetResolver EvidenceTargetResolver
	SemanticMatcher SemanticEvidenceMatcher
	MinConfidence  float64
}

var _ EvidenceSelector = (*HybridEvidenceSelector)(nil)

func (s *HybridEvidenceSelector) Select(
	ctx context.Context,
	task ResearchTask,
	documents []RawDocument,
) ([]EvidenceSelectionDecision, error) {
	if s == nil || s.TargetResolver == nil {
		return nil, errors.New(
			"evidence target resolver is required",
		)
	}

	target, err := s.TargetResolver.Resolve(
		ctx,
		task,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve evidence target: %w",
			err,
		)
	}
	target, err = normalizeEvidenceTarget(
		task,
		target,
	)
	if err != nil {
		return nil, err
	}

	threshold := s.MinConfidence
	if threshold == 0 {
		threshold = defaultSemanticSelectionThreshold
	}
	if threshold < 0 || threshold > 1 {
		return nil, errors.New(
			"semantic evidence selection threshold must be between 0 and 1",
		)
	}

	decisions := make(
		[]EvidenceSelectionDecision,
		0,
		len(documents),
	)
	unresolved := make(
		[]RawDocument,
		0,
		len(documents),
	)

	seenIDs := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		documentID := strings.TrimSpace(document.ID)
		if documentID == "" {
			return nil, errors.New(
				"candidate raw document id is required",
			)
		}
		if _, duplicate := seenIDs[documentID]; duplicate {
			return nil, fmt.Errorf(
				"candidate raw document id %q is duplicated",
				documentID,
			)
		}
		seenIDs[documentID] = struct{}{}

		if reason, matched := deterministicEvidenceMatch(
			target,
			document,
		); matched {
			decisions = append(
				decisions,
				EvidenceSelectionDecision{
					RawDocumentID: documentID,
					Selected:      true,
					Method:        SelectionDeterministic,
					Confidence:    1,
					Reason:        reason,
				},
			)
			continue
		}

		unresolved = append(
			unresolved,
			document,
		)
	}

	if len(unresolved) > 0 {
		if s.SemanticMatcher == nil {
			return nil, errors.New(
				"semantic evidence matcher is required for unresolved candidates",
			)
		}

		matches, matchErr := s.SemanticMatcher.Match(
			ctx,
			target,
			unresolved,
		)
		if matchErr != nil {
			return nil, fmt.Errorf(
				"semantic evidence match: %w",
				matchErr,
			)
		}

		semanticDecisions, validationErr :=
			validateSemanticMatches(
				unresolved,
				matches,
				threshold,
			)
		if validationErr != nil {
			return nil, validationErr
		}
		decisions = append(
			decisions,
			semanticDecisions...,
		)
	}

	sort.SliceStable(
		decisions,
		func(i int, j int) bool {
			return decisions[i].RawDocumentID <
				decisions[j].RawDocumentID
		},
	)

	return decisions, nil
}

func normalizeEvidenceTarget(
	task ResearchTask,
	target EvidenceTarget,
) (EvidenceTarget, error) {
	taskSymbol := strings.ToUpper(
		strings.TrimSpace(task.Symbol),
	)
	target.Symbol = strings.ToUpper(
		strings.TrimSpace(target.Symbol),
	)
	target.Name = strings.TrimSpace(
		target.Name,
	)

	if target.Symbol == "" {
		target.Symbol = taskSymbol
	}
	if taskSymbol != "" &&
		target.Symbol != "" &&
		target.Symbol != taskSymbol {
		return EvidenceTarget{}, fmt.Errorf(
			"resolved evidence target symbol %q does not match task symbol %q",
			target.Symbol,
			taskSymbol,
		)
	}
	if target.Symbol == "" &&
		target.Name == "" &&
		len(target.Aliases) == 0 {
		return EvidenceTarget{}, errors.New(
			"resolved evidence target identity is empty",
		)
	}

	seen := map[string]struct{}{}
	aliases := make([]string, 0, len(target.Aliases))
	for _, alias := range target.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		key := strings.ToLower(alias)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		aliases = append(aliases, alias)
	}
	target.Aliases = aliases

	return target, nil
}

func deterministicEvidenceMatch(
	target EvidenceTarget,
	document RawDocument,
) (string, bool) {
	targetSymbol := strings.ToUpper(
		strings.TrimSpace(target.Symbol),
	)
	baseSymbol := snapshotBaseSymbol(
		targetSymbol,
	)

	for _, key := range []string{
		"symbol",
		"instrument_id",
	} {
		candidate := strings.ToUpper(
			strings.TrimSpace(
				snapshotMetadataString(
					document.RawMetadata,
					key,
				),
			),
		)
		if candidate == "" {
			continue
		}
		if candidate == targetSymbol ||
			(targetSymbol != "" &&
				snapshotBaseSymbol(candidate) ==
					baseSymbol) {
			return "candidate metadata " + key +
				" matches target symbol", true
		}
	}

	text := strings.TrimSpace(
		document.Title + "\n" +
			document.Content,
	)
	if text == "" {
		return "", false
	}

	if targetSymbol != "" &&
		containsStrongToken(
			text,
			targetSymbol,
		) {
		return "candidate text explicitly mentions target symbol", true
	}
	if baseSymbol != "" &&
		baseSymbol != targetSymbol &&
		containsStrongToken(
			text,
			baseSymbol,
		) {
		return "candidate text explicitly mentions target base symbol", true
	}

	entities := make(
		[]string,
		0,
		1+len(target.Aliases),
	)
	if target.Name != "" {
		entities = append(
			entities,
			target.Name,
		)
	}
	entities = append(
		entities,
		target.Aliases...,
	)
	for _, entity := range entities {
		if containsEntity(
			text,
			entity,
		) {
			return fmt.Sprintf(
				"candidate text explicitly mentions target entity %q",
				entity,
			), true
		}
	}

	return "", false
}

func containsStrongToken(
	text string,
	token string,
) bool {
	text = strings.ToUpper(text)
	token = strings.ToUpper(
		strings.TrimSpace(token),
	)
	if token == "" {
		return false
	}

	pattern := `(^|[^A-Z0-9])` +
		regexp.QuoteMeta(token) +
		`([^A-Z0-9]|$)`
	matched, err := regexp.MatchString(
		pattern,
		text,
	)
	return err == nil && matched
}

func containsEntity(
	text string,
	entity string,
) bool {
	text = strings.ToLower(text)
	entity = strings.ToLower(
		strings.TrimSpace(entity),
	)
	if entity == "" {
		return false
	}
	return strings.Contains(
		text,
		entity,
	)
}

func validateSemanticMatches(
	documents []RawDocument,
	matches []SemanticEvidenceMatch,
	threshold float64,
) ([]EvidenceSelectionDecision, error) {
	decisions, err := validatePartialSemanticMatches(documents, matches, threshold)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(decisions))
	for _, decision := range decisions {
		seen[decision.RawDocumentID] = struct{}{}
	}
	for _, document := range documents {
		documentID := strings.TrimSpace(document.ID)
		if _, ok := seen[documentID]; !ok {
			return nil, fmt.Errorf(
				"semantic evidence research incomplete: missing match for raw document %q",
				documentID,
			)
		}
	}
	return decisions, nil
}

func validatePartialSemanticMatches(
	documents []RawDocument,
	matches []SemanticEvidenceMatch,
	threshold float64,
) ([]EvidenceSelectionDecision, error) {
	expected := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		expected[strings.TrimSpace(document.ID)] = struct{}{}
	}

	seen := make(map[string]struct{}, len(matches))
	decisions := make(
		[]EvidenceSelectionDecision,
		0,
		len(matches),
	)

	for _, match := range matches {
		documentID := strings.TrimSpace(
			match.RawDocumentID,
		)
		if documentID == "" {
			return nil, errors.New(
				"semantic evidence match raw document id is required",
			)
		}
		if _, ok := expected[documentID]; !ok {
			return nil, fmt.Errorf(
				"semantic evidence match references unknown raw document %q",
				documentID,
			)
		}
		if _, duplicate := seen[documentID]; duplicate {
			return nil, fmt.Errorf(
				"semantic evidence match for raw document %q is duplicated",
				documentID,
			)
		}
		if match.Confidence < 0 ||
			match.Confidence > 1 {
			return nil, fmt.Errorf(
				"semantic evidence match confidence for raw document %q must be between 0 and 1",
				documentID,
			)
		}
		seen[documentID] = struct{}{}

		selected := match.Relevant &&
			match.Confidence >= threshold
		reason := strings.TrimSpace(match.Reason)
		if match.Relevant &&
			match.Confidence < threshold {
			reason = fmt.Sprintf(
				"semantic relevance confidence %.3f below selection threshold %.3f: %s",
				match.Confidence,
				threshold,
				reason,
			)
		}

		decisions = append(
			decisions,
			EvidenceSelectionDecision{
				RawDocumentID: documentID,
				Selected:      selected,
				Method:        SelectionSemantic,
				Confidence:    match.Confidence,
				Reason:        reason,
			},
		)
	}


	return decisions, nil
}
