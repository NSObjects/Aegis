package event

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPDFiumTextExtractorExtractsEmbeddedText(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	result, err := (PDFiumTextExtractor{
		MaxTextChars: 1_000,
	}).Extract(
		ctx,
		minimalTextPDF("Hello Aegis PDF"),
	)
	if err != nil {
		t.Fatalf(
			"Extract() error = %v",
			err,
		)
	}
	if !strings.Contains(
		result.Text,
		"Hello Aegis PDF",
	) {
		t.Fatalf(
			"extracted text = %q",
			result.Text,
		)
	}
	if result.Truncated {
		t.Fatal(
			"small PDF text should not be truncated",
		)
	}
}

func TestPDFTextNormalizationAndBounding(
	t *testing.T,
) {
	normalized := normalizePDFText(
		"  第一段\r\n\t第二段   Aegis  ",
	)
	if normalized != "第一段 第二段 Aegis" {
		t.Fatalf(
			"normalized = %q",
			normalized,
		)
	}

	bounded, truncated := truncateTextChars(
		"甲乙丙丁",
		3,
	)
	if bounded != "甲乙丙" || !truncated {
		t.Fatalf(
			"bounded = %q truncated=%v",
			bounded,
			truncated,
		)
	}
}

func minimalTextPDF(
	text string,
) []byte {
	text = strings.NewReplacer(
		"\\", "\\\\",
		"(", "\\(",
		")", "\\)",
	).Replace(text)

	stream := fmt.Sprintf(
		"BT /F1 12 Tf 72 720 Td (%s) Tj ET",
		text,
	)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf(
			"<< /Length %d >>\nstream\n%s\nendstream",
			len(stream),
			stream,
		),
	}

	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.4\n")

	offsets := make(
		[]int,
		len(objects)+1,
	)
	for index, object := range objects {
		offsets[index+1] = buffer.Len()
		fmt.Fprintf(
			&buffer,
			"%d 0 obj\n%s\nendobj\n",
			index+1,
			object,
		)
	}

	xrefOffset := buffer.Len()
	fmt.Fprintf(
		&buffer,
		"xref\n0 %d\n",
		len(offsets),
	)
	buffer.WriteString(
		"0000000000 65535 f \n",
	)
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(
			&buffer,
			"%010d 00000 n \n",
			offsets[index],
		)
	}
	fmt.Fprintf(
		&buffer,
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets),
		xrefOffset,
	)

	return buffer.Bytes()
}
