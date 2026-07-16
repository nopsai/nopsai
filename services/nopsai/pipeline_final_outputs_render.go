package nopsai

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"nopsai/pkg/models"
)

var outputFileNameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

type pipelinePDFConverter interface {
	ConvertHTML(ctx context.Context, documentHTML []byte, filename string) ([]byte, error)
}

func renderPipelineFinalOutputDownload(
	ctx context.Context,
	output models.PipelineRunFinalOutput,
	pdfConverter pipelinePDFConverter,
) ([]byte, string, string, error) {
	if strings.TrimSpace(output.Status) != finalOutputStatusSuccess {
		return nil, "", "", fmt.Errorf("final output is not ready")
	}
	content := strings.TrimSpace(output.Content)
	if content == "" {
		return nil, "", "", fmt.Errorf("final output is empty")
	}
	outputType := normalizePipelineFinalOutputType(output.Type)
	fileName := pipelineFinalOutputFileName(output.Name, outputType)
	switch outputType {
	case "markdown":
		return []byte(content), "text/markdown; charset=utf-8", fileName, nil
	case "json", "dashboard":
		return []byte(content), "application/json; charset=utf-8", fileName, nil
	case "html":
		payload, err := renderPipelineFinalOutputHTML(content)
		if err != nil {
			return nil, "", "", err
		}
		return payload, "text/html; charset=utf-8", fileName, nil
	case "pdf":
		if pdfConverter == nil {
			return nil, "", "", fmt.Errorf("PDF renderer is not configured")
		}
		documentHTML, err := renderPipelineFinalOutputDocumentHTML(output.Name, content)
		if err != nil {
			return nil, "", "", err
		}
		payload, err := pdfConverter.ConvertHTML(ctx, documentHTML, fileName)
		if err != nil {
			return nil, "", "", fmt.Errorf("render PDF: %w", err)
		}
		return payload, "application/pdf", fileName, nil
	case "excel":
		payload, err := buildPipelineFinalOutputXLSX(output.Name, content)
		if err != nil {
			return nil, "", "", err
		}
		return payload, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", fileName, nil
	default:
		return []byte(content), "text/plain; charset=utf-8", pipelineFinalOutputFileName(output.Name, "txt"), nil
	}
}

func pipelineFinalOutputFileName(name, outputType string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = strings.ReplaceAll(base, " ", "-")
	base = outputFileNameUnsafe.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-_.")
	if base == "" {
		base = "final-output"
	}
	ext := pipelineFinalOutputExtension(outputType)
	return base + "." + ext
}

func pipelineFinalOutputExtension(outputType string) string {
	switch normalizePipelineFinalOutputType(outputType) {
	case "markdown":
		return "md"
	case "pdf":
		return "pdf"
	case "excel":
		return "xlsx"
	case "json":
		return "json"
	case "dashboard":
		return "json"
	case "html":
		return "html"
	default:
		return strings.Trim(strings.TrimSpace(outputType), ".")
	}
}
