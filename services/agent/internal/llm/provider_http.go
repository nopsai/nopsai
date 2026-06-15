package llm

import (
	"fmt"
	"io"
)

const maxLLMResponseBytes = 2 << 20

func readLLMResponseBody(body io.Reader) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(body, maxLLMResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxLLMResponseBytes {
		return nil, fmt.Errorf("LLM response exceeded %d bytes", maxLLMResponseBytes)
	}
	return contents, nil
}
