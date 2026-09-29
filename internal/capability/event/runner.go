package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

type TaskRunOutput struct {
	Attempt          AcquisitionAttempt
	RawDocuments     []RawDocument
	Canonicalization CanonicalizationResult
	Result           ResearchTaskResult
}

// TaskRunner executes exactly one concrete Retriever. It deliberately does not
// choose providers or implement fallback. Retrieval errors are recorded on the
// AcquisitionAttempt and converted into task coverage semantics instead of
// automatically aborting the research task.
type TaskRunner struct {
	Now func() time.Time
}

func (r TaskRunner) Run(
	ctx context.Context,
	task ResearchTask,
	retriever Retriever,
) (TaskRunOutput, error) {
	if retriever == nil {
		return TaskRunOutput{}, errors.New(
			"event retriever is required",
		)
	}

	task.ID = strings.TrimSpace(task.ID)
	if task.ID == "" {
		return TaskRunOutput{}, errors.New(
			"event research task id is required",
		)
	}

	retrieverID := strings.TrimSpace(retriever.ID())
	if retrieverID == "" {
		return TaskRunOutput{}, errors.New(
			"event retriever id is required",
		)
	}
	if retriever.Method() == "" {
		return TaskRunOutput{}, errors.New(
			"event retriever acquisition method is required",
		)
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}

	startedAt := now().UTC()
	attempt := AcquisitionAttempt{
		ID:        acquisitionAttemptID(task.ID, retrieverID, startedAt),
		TaskID:    task.ID,
		Method:    retriever.Method(),
		Tool:      retrieverID,
		StartedAt: startedAt,
		Status:    AcquisitionSucceeded,
	}

	rawDocuments, retrieveErr := retriever.Retrieve(ctx, task)
	finishedAt := now().UTC()
	attempt.FinishedAt = finishedAt

	if retrieveErr != nil {
		attempt.ErrorMessage = retrieveErr.Error()
		switch {
		case len(rawDocuments) > 0:
			attempt.Status = AcquisitionPartial
		case errors.Is(retrieveErr, context.DeadlineExceeded):
			attempt.Status = AcquisitionTimeout
		default:
			attempt.Status = AcquisitionFailed
		}
	}

	rawDocuments = normalizeRawDocuments(
		task,
		attempt,
		finishedAt,
		rawDocuments,
	)
	for _, document := range rawDocuments {
		attempt.DocumentRefs = appendUnique(
			attempt.DocumentRefs,
			document.ID,
		)
	}

	canonicalization, err := (Canonicalizer{}).Canonicalize(
		task,
		rawDocuments,
	)
	if err != nil {
		return TaskRunOutput{
			Attempt:      attempt,
			RawDocuments: rawDocuments,
		}, err
	}

	result := (TaskEvaluator{}).Evaluate(
		task,
		canonicalization.Documents,
		[]AcquisitionAttempt{attempt},
	)
	result.CompletedAt = finishedAt

	return TaskRunOutput{
		Attempt:          attempt,
		RawDocuments:     rawDocuments,
		Canonicalization: canonicalization,
		Result:           result,
	}, nil
}

func normalizeRawDocuments(
	task ResearchTask,
	attempt AcquisitionAttempt,
	retrievedAt time.Time,
	documents []RawDocument,
) []RawDocument {
	for index := range documents {
		documents[index].TaskID = task.ID
		documents[index].AttemptID = attempt.ID

		if documents[index].RetrievedAt.IsZero() {
			documents[index].RetrievedAt = retrievedAt
		}
		if documents[index].FirstSeenAt == nil {
			firstSeenAt := documents[index].RetrievedAt
			documents[index].FirstSeenAt = &firstSeenAt
		}

		documents[index].ID = strings.TrimSpace(
			documents[index].ID,
		)
		if documents[index].ID == "" {
			documents[index].ID = rawDocumentID(
				attempt.ID,
				index,
				documents[index],
			)
		}
	}

	return documents
}

func acquisitionAttemptID(
	taskID string,
	retrieverID string,
	startedAt time.Time,
) string {
	return shortID(
		"attempt",
		taskID,
		retrieverID,
		startedAt.Format(time.RFC3339Nano),
	)
}

func rawDocumentID(
	attemptID string,
	index int,
	document RawDocument,
) string {
	return shortID(
		"raw",
		attemptID,
		strconv.Itoa(index),
		document.URL,
		document.Title,
		document.Content,
	)
}

func shortID(
	prefix string,
	parts ...string,
) string {
	sum := sha256.Sum256(
		[]byte(strings.Join(parts, "\x00")),
	)
	return prefix + "_" + hex.EncodeToString(sum[:8])
}
