package event

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TaskEvaluator struct{}

func (TaskEvaluator) Evaluate(
	task ResearchTask,
	documents []CanonicalDocument,
	attempts []AcquisitionAttempt,
) ResearchTaskResult {
	result := ResearchTaskResult{
		TaskID:      task.ID,
		Status:      TaskSatisfied,
		Documents:   make([]string, 0, len(documents)),
		Attempts:    make([]string, 0, len(attempts)),
		Gaps:        make([]ResearchGap, 0),
		CompletedAt: time.Now().UTC(),
	}

	for _, document := range documents {
		result.Documents = appendUnique(
			result.Documents,
			document.ID,
		)
	}
	for _, attempt := range attempts {
		result.Attempts = appendUnique(
			result.Attempts,
			attempt.ID,
		)
	}

	if len(documents) == 0 {
		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type:        GapNoUsableDocuments,
				Description: "no usable canonical document was collected",
			},
		)
	}

	if required := task.Requirements.MinDocuments; required > 0 &&
		len(documents) < required {
		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type: GapInsufficientDocuments,
				Description: fmt.Sprintf(
					"collected %d canonical documents; require at least %d",
					len(documents),
					required,
				),
				Required: required,
				Observed: len(documents),
			},
		)
	}

	independentSources := countIndependentSources(documents)
	if required := task.Requirements.MinIndependentSources; required > 0 &&
		independentSources < required {
		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type: GapInsufficientIndependentSources,
				Description: fmt.Sprintf(
					"observed %d independent sources; require at least %d",
					independentSources,
					required,
				),
				Required: required,
				Observed: independentSources,
			},
		)
	}

	observedSourceTypes := make(map[SourceType]struct{})
	observedAuthorities := make(map[EvidenceAuthority]struct{})
	observedEvidenceKinds := make(map[EvidenceKind]struct{})
	hasPrimary := false
	for _, document := range documents {
		if document.Source.SourceType != "" {
			observedSourceTypes[document.Source.SourceType] = struct{}{}
		}
		if document.Source.Authority != "" {
			observedAuthorities[document.Source.Authority] = struct{}{}
		}
		if document.Source.EvidenceKind != "" {
			observedEvidenceKinds[document.Source.EvidenceKind] = struct{}{}
		}
		if document.Source.IsPrimary {
			hasPrimary = true
		}
	}

	for _, requiredType := range task.Requirements.RequiredSourceTypes {
		if _, ok := observedSourceTypes[requiredType]; ok {
			continue
		}

		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type: GapMissingRequiredSourceType,
				Description: fmt.Sprintf(
					"required source type %q is missing",
					requiredType,
				),
				SourceType: requiredType,
			},
		)
	}

	for _, requiredAuthority := range task.Requirements.RequiredAuthorities {
		if _, ok := observedAuthorities[requiredAuthority]; ok {
			continue
		}

		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type: GapMissingRequiredAuthority,
				Description: fmt.Sprintf(
					"required evidence authority %q is missing",
					requiredAuthority,
				),
				Authority: requiredAuthority,
			},
		)
	}

	for _, requiredKind := range task.Requirements.RequiredEvidenceKinds {
		if _, ok := observedEvidenceKinds[requiredKind]; ok {
			continue
		}

		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type: GapMissingRequiredEvidenceKind,
				Description: fmt.Sprintf(
					"required evidence kind %q is missing",
					requiredKind,
				),
				EvidenceKind: requiredKind,
			},
		)
	}

	if task.Requirements.RequirePrimarySource && !hasPrimary {
		result.Gaps = append(
			result.Gaps,
			ResearchGap{
				Type:        GapMissingPrimarySource,
				Description: "a primary source is required but none was collected",
			},
		)
	}

	switch {
	case len(result.Gaps) == 0:
		result.Status = TaskSatisfied
	case len(documents) == 0:
		result.Status = TaskUnsatisfied
	default:
		result.Status = TaskPartial
	}

	return result
}

func countIndependentSources(
	documents []CanonicalDocument,
) int {
	seenSources := make(map[string]struct{})
	seenContent := make(map[string]struct{})
	count := 0

	for _, document := range documents {
		contentKey := strings.TrimSpace(document.ContentHash)
		_, duplicateContent := seenContent[contentKey]
		if contentKey != "" {
			seenContent[contentKey] = struct{}{}
		}

		sourceKey := strings.ToLower(
			strings.TrimSpace(document.Source.Publisher),
		)
		if sourceKey == "" {
			sourceKey = sourceHost(document.Source.URL)
		}
		if sourceKey == "" {
			continue
		}

		if _, exists := seenSources[sourceKey]; exists {
			continue
		}
		seenSources[sourceKey] = struct{}{}

		if contentKey != "" && duplicateContent {
			continue
		}

		count++
	}

	return count
}

func sourceHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
