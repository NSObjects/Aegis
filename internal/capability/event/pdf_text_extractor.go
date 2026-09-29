package event

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

const defaultPDFTextMaxChars = 120_000

// PDFTextExtraction is the bounded, normalized text recovered from one PDF.
// Truncated is explicit so callers can preserve the fact that the captured
// evidence body is incomplete without treating it as an acquisition failure.
type PDFTextExtraction struct {
	Text      string
	Truncated bool
}

// PDFTextExtractor is deliberately smaller than a generic document ingestion
// interface. V2.7.17 only needs one operation: extract text from already
// captured PDF bytes.
type PDFTextExtractor interface {
	Extract(
		ctx context.Context,
		data []byte,
	) (PDFTextExtraction, error)
}

type PDFTextExtractorFunc func(
	context.Context,
	[]byte,
) (PDFTextExtraction, error)

func (f PDFTextExtractorFunc) Extract(
	ctx context.Context,
	data []byte,
) (PDFTextExtraction, error) {
	return f(ctx, data)
}

// PDFiumTextExtractor uses go-pdfium's embedded WebAssembly PDFium build.
// It has no system pdftotext/Poppler dependency and does not require CGO.
type PDFiumTextExtractor struct {
	MaxTextChars int
}

var (
	defaultPDFiumPoolOnce sync.Once
	defaultPDFiumPool     pdfium.Pool
	defaultPDFiumPoolErr  error
)

func (e PDFiumTextExtractor) Extract(
	ctx context.Context,
	data []byte,
) (PDFTextExtraction, error) {
	if err := ctx.Err(); err != nil {
		return PDFTextExtraction{}, err
	}
	if len(data) == 0 {
		return PDFTextExtraction{}, errors.New(
			"PDF body is empty",
		)
	}

	pool, err := getDefaultPDFiumPool()
	if err != nil {
		return PDFTextExtraction{}, err
	}

	instance, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		return PDFTextExtraction{}, fmt.Errorf(
			"acquire PDFium instance: %w",
			err,
		)
	}

	// PDFium methods do not take a request context. When the caller is
	// cancelled, Kill interrupts the WASM worker. WithCloseOnContextDone
	// makes wazero stop in-flight execution rather than waiting forever.
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = instance.Kill()
		case <-closed:
		}
	}()
	defer close(closed)
	defer func() {
		_ = instance.Close()
	}()

	document, err := instance.OpenDocument(
		&requests.OpenDocument{
			File: &data,
		},
	)
	if err != nil {
		return PDFTextExtraction{}, fmt.Errorf(
			"open PDF document: %w",
			err,
		)
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(
			&requests.FPDF_CloseDocument{
				Document: document.Document,
			},
		)
	}()

	pageCount, err := instance.FPDF_GetPageCount(
		&requests.FPDF_GetPageCount{
			Document: document.Document,
		},
	)
	if err != nil {
		return PDFTextExtraction{}, fmt.Errorf(
			"read PDF page count: %w",
			err,
		)
	}
	if pageCount.PageCount <= 0 {
		return PDFTextExtraction{}, errors.New(
			"PDF has no pages",
		)
	}

	maxChars := e.MaxTextChars
	if maxChars <= 0 {
		maxChars = defaultPDFTextMaxChars
	}

	var builder strings.Builder
	totalChars := 0
	truncated := false

	for pageIndex := 0;
		pageIndex < pageCount.PageCount;
		pageIndex++ {
		if err := ctx.Err(); err != nil {
			return PDFTextExtraction{}, err
		}

		pageText, err := instance.GetPageText(
			&requests.GetPageText{
				Page: requests.Page{
					ByIndex: &requests.PageByIndex{
						Document: document.Document,
						Index:    pageIndex,
					},
				},
			},
		)
		if err != nil {
			return PDFTextExtraction{}, fmt.Errorf(
				"extract PDF page %d text: %w",
				pageIndex+1,
				err,
			)
		}

		normalized := normalizePDFText(pageText.Text)
		if normalized == "" {
			continue
		}

		if builder.Len() > 0 {
			if totalChars+2 > maxChars {
				truncated = true
				break
			}
			builder.WriteString("\n\n")
			totalChars += 2
		}

		remaining := maxChars - totalChars
		bounded, wasTruncated := truncateTextChars(
			normalized,
			remaining,
		)
		builder.WriteString(bounded)
		totalChars += utf8.RuneCountInString(bounded)
		if wasTruncated {
			truncated = true
			break
		}
	}

	return PDFTextExtraction{
		Text:      strings.TrimSpace(builder.String()),
		Truncated: truncated,
	}, nil
}

func getDefaultPDFiumPool() (pdfium.Pool, error) {
	defaultPDFiumPoolOnce.Do(func() {
		defaultPDFiumPool, defaultPDFiumPoolErr =
			webassembly.Init(
				webassembly.Config{
					MinIdle:  0,
					MaxIdle:  2,
					MaxTotal: 2,
					ReuseWorkers: true,
					RuntimeConfig: wazero.
						NewRuntimeConfig().
						WithCloseOnContextDone(true),
					Stdout: io.Discard,
					Stderr: io.Discard,
				},
			)
		if defaultPDFiumPoolErr != nil {
			defaultPDFiumPoolErr = fmt.Errorf(
				"initialize PDFium WebAssembly runtime: %w",
				defaultPDFiumPoolErr,
			)
		}
	})
	return defaultPDFiumPool, defaultPDFiumPoolErr
}

func normalizePDFText(value string) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.ReplaceAll(value, "\x00", "")
	return strings.Join(strings.Fields(value), " ")
}

func truncateTextChars(
	value string,
	maxChars int,
) (string, bool) {
	if maxChars <= 0 {
		return "", value != ""
	}
	if utf8.RuneCountInString(value) <= maxChars {
		return value, false
	}

	runes := []rune(value)
	return string(runes[:maxChars]), true
}
