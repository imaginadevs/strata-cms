package stub

import (
	"context"
	"fmt"
	"strings"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// Renderer returns a minimal valid PDF summarizing the entity (placeholder until a real processor exists).
type Renderer struct{}

func New() *Renderer { return &Renderer{} }

func (r *Renderer) Render(_ context.Context, req outbound.PDFRenderRequest) (string, []byte, error) {
	filename := req.Filename
	if filename == "" {
		filename = fmt.Sprintf("%s-%s.pdf", req.Collection, req.ItemID)
	}
	title := fmt.Sprintf("%s / %s / %s", req.Collection, req.ItemID, req.Template)
	title = strings.ReplaceAll(title, "\\", "\\\\")
	title = strings.ReplaceAll(title, "(", "\\(")
	title = strings.ReplaceAll(title, ")", "\\)")

	content := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET", title)
	objects := []string{
		"1 0 obj<< /Type /Catalog /Pages 2 0 R >>endobj\n",
		"2 0 obj<< /Type /Pages /Kids [3 0 R] /Count 1 >>endobj\n",
		"3 0 obj<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources<< /Font<< /F1 5 0 R >> >> >>endobj\n",
		fmt.Sprintf("4 0 obj<< /Length %d >>stream\n%s\nendstream\nendobj\n", len(content), content),
		"5 0 obj<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>endobj\n",
	}

	var body strings.Builder
	body.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = body.Len()
		body.WriteString(obj)
	}
	xref := body.Len()
	body.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))
	body.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		body.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	body.WriteString(fmt.Sprintf("trailer<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref))
	return filename, []byte(body.String()), nil
}
