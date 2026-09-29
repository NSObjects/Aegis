package event

import (
	"fmt"
	"time"
)

type TaskEvaluator struct {
	Corroboration *CorroborationResult
}

func (e TaskEvaluator) Evaluate(
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

	corroboration := CorroborationResult{}
	if e.Corroboration != nil {
		corroboration = *e.Corroboration
	} else {
		corroboration = (EvidenceCorroborator{}).Corroborate(
			documents,
		)
	}
	independentSources := corroboration.IndependentSourceCount
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

