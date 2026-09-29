package event

import (
	"sort"
	"strings"
	"unicode"
)

const (
	DefaultNearDuplicateMinRunes      = 160
	DefaultNearDuplicateShingleRunes  = 8
	DefaultNearDuplicateContainment   = 0.82
	DefaultNearDuplicateJaccard       = 0.55
)

type EvidenceRelationKind string

const (
	EvidenceRelationExactReprint EvidenceRelationKind = "exact_reprint"
	EvidenceRelationNearReprint  EvidenceRelationKind = "near_reprint"
)

type EvidenceRelation struct {
	LeftDocumentID  string
	RightDocumentID string
	Kind            EvidenceRelationKind
	Method          string
	Score           float64
	Threshold       float64
	Containment     float64
	Jaccard         float64
}

type NearDuplicateMatcher struct {
	MinRunes             int
	ShingleRunes         int
	MinContainment       float64
	MinJaccard           float64
}

type nearDuplicatePrepared struct {
	Document     CanonicalDocument
	SourceKey    string
	Normalized   string
	RuneCount    int
	Shingles     map[string]struct{}
}

func (m NearDuplicateMatcher) Match(
	left CanonicalDocument,
	right CanonicalDocument,
) (EvidenceRelation, bool) {
	return m.matchPrepared(
		m.prepare(left),
		m.prepare(right),
	)
}

func (m NearDuplicateMatcher) prepare(
	document CanonicalDocument,
) nearDuplicatePrepared {
	shingleRunes := m.ShingleRunes
	if shingleRunes <= 0 {
		shingleRunes = DefaultNearDuplicateShingleRunes
	}
	normalized := normalizeNearDuplicateText(
		document.Content,
	)
	return nearDuplicatePrepared{
		Document:   document,
		SourceKey:  corroborationSourceKey(document.Source),
		Normalized: normalized,
		RuneCount:  len([]rune(normalized)),
		Shingles: characterShingles(
			normalized,
			shingleRunes,
		),
	}
}

func (m NearDuplicateMatcher) matchPrepared(
	left nearDuplicatePrepared,
	right nearDuplicatePrepared,
) (EvidenceRelation, bool) {
	leftID, rightID := orderedDocumentIDs(
		left.Document.ID,
		right.Document.ID,
	)
	if leftID == "" || rightID == "" ||
		leftID == rightID {
		return EvidenceRelation{}, false
	}

	if left.Document.ContentHash != "" &&
		left.Document.ContentHash ==
			right.Document.ContentHash {
		return EvidenceRelation{
			LeftDocumentID:  leftID,
			RightDocumentID: rightID,
			Kind:            EvidenceRelationExactReprint,
			Method:          "content_hash",
			Score:           1,
			Threshold:       1,
			Containment:     1,
			Jaccard:         1,
		}, true
	}

	if left.SourceKey == "" ||
		right.SourceKey == "" ||
		left.SourceKey == right.SourceKey {
		return EvidenceRelation{}, false
	}

	minRunes := m.MinRunes
	if minRunes <= 0 {
		minRunes = DefaultNearDuplicateMinRunes
	}
	minContainment := m.MinContainment
	if minContainment <= 0 {
		minContainment =
			DefaultNearDuplicateContainment
	}
	minJaccard := m.MinJaccard
	if minJaccard <= 0 {
		minJaccard = DefaultNearDuplicateJaccard
	}

	if left.RuneCount < minRunes ||
		right.RuneCount < minRunes {
		return EvidenceRelation{}, false
	}
	if len(left.Shingles) == 0 ||
		len(right.Shingles) == 0 {
		return EvidenceRelation{}, false
	}

	intersection := shingleIntersectionCount(
		left.Shingles,
		right.Shingles,
	)
	minCount := len(left.Shingles)
	if len(right.Shingles) < minCount {
		minCount = len(right.Shingles)
	}
	unionCount := len(left.Shingles) +
		len(right.Shingles) - intersection

	containment := float64(intersection) /
		float64(minCount)
	jaccard := float64(intersection) /
		float64(unionCount)

	relation := EvidenceRelation{
		LeftDocumentID:  leftID,
		RightDocumentID: rightID,
		Kind:            EvidenceRelationNearReprint,
		Method:          "char_shingle_containment_jaccard",
		Score:           containment,
		Threshold:       minContainment,
		Containment:     containment,
		Jaccard:         jaccard,
	}
	if containment < minContainment ||
		jaccard < minJaccard {
		return relation, false
	}

	return relation, true
}
func normalizeNearDuplicateText(
	value string,
) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, current := range value {
		switch {
		case unicode.IsLetter(current):
			builder.WriteRune(
				unicode.ToLower(current),
			)
		case unicode.IsNumber(current):
			builder.WriteRune(current)
		}
	}

	return builder.String()
}

func characterShingles(
	value string,
	width int,
) map[string]struct{} {
	runes := []rune(value)
	if width <= 0 || len(runes) < width {
		return nil
	}

	shingles := make(
		map[string]struct{},
		len(runes)-width+1,
	)
	for start := 0;
		start+width <= len(runes);
		start++ {
		shingles[string(
			runes[start:start+width],
		)] = struct{}{}
	}
	return shingles
}

func shingleIntersectionCount(
	left map[string]struct{},
	right map[string]struct{},
) int {
	if len(left) > len(right) {
		left, right = right, left
	}

	count := 0
	for shingle := range left {
		if _, ok := right[shingle]; ok {
			count++
		}
	}
	return count
}

func orderedDocumentIDs(
	left string,
	right string,
) (string, string) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if right < left {
		left, right = right, left
	}
	return left, right
}

func sortEvidenceRelations(
	relations []EvidenceRelation,
) {
	sort.SliceStable(
		relations,
		func(i int, j int) bool {
			left := relations[i]
			right := relations[j]
			if left.LeftDocumentID !=
				right.LeftDocumentID {
				return left.LeftDocumentID <
					right.LeftDocumentID
			}
			if left.RightDocumentID !=
				right.RightDocumentID {
				return left.RightDocumentID <
					right.RightDocumentID
			}
			return left.Kind < right.Kind
		},
	)
}
