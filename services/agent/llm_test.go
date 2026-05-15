package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

func TestBuildLMStudioChatURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/api/v1/chat"},
		{name: "api v1 base", baseURL: "http://127.0.0.1:1234/api/v1", want: "http://127.0.0.1:1234/api/v1/chat"},
		{name: "full endpoint", baseURL: "http://127.0.0.1:1234/api/v1/chat", want: "http://127.0.0.1:1234/api/v1/chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioChatURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioChatURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestBuildLMStudioModelsURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/api/v1/models"},
		{name: "api v1 base", baseURL: "http://127.0.0.1:1234/api/v1", want: "http://127.0.0.1:1234/api/v1/models"},
		{name: "chat endpoint", baseURL: "http://127.0.0.1:1234/api/v1/chat", want: "http://127.0.0.1:1234/api/v1/models"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioModelsURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioModelsURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestBuildLMStudioModelLoadURL(t *testing.T) {
	if got, want := buildLMStudioModelLoadURL("http://127.0.0.1:1234"), "http://127.0.0.1:1234/api/v1/models/load"; got != want {
		t.Fatalf("buildLMStudioModelLoadURL() = %q, want %q", got, want)
	}
}

func TestCleanModelTextResponse(t *testing.T) {
	raw := "```json\n{\"action\":{\"type\":\"RETURN_ANSWER\"}}\n```"
	want := "{\"action\":{\"type\":\"RETURN_ANSWER\"}}"

	if got := cleanModelTextResponse(raw); got != want {
		t.Fatalf("cleanModelTextResponse() = %q, want %q", got, want)
	}
}

func TestDecodeActionResponseRecoversFencedJSONAfterReasoning(t *testing.T) {
	raw := "The user wants to know which files are visible based only on the shared workspace files.\n\n" +
		"Therefore, the answer should list the files present in the working directory contents.```json\n" +
		"{\n" +
		"  \"action\": {\n" +
		"    \"type\": \"RETURN_ANSWER\",\n" +
		"    \"answer_action\": {\n" +
		"      \"answer\": \"The file visible in the shared workspace is: test-app/Dockerfile.\"\n" +
		"    }\n" +
		"  }\n" +
		"}"

	action, err := decodeActionResponse(raw)
	if err != nil {
		t.Fatalf("decodeActionResponse() error = %v", err)
	}
	if action.Type != models.ActionTypeReturnAnswer {
		t.Fatalf("action.Type = %q, want %q", action.Type, models.ActionTypeReturnAnswer)
	}
	if action.AnswerAction == nil || action.AnswerAction.Answer != "The file visible in the shared workspace is: test-app/Dockerfile." {
		t.Fatalf("action.AnswerAction = %#v", action.AnswerAction)
	}
}

func TestDecodeActionResponseRecoversBalancedJSONWithBracesInString(t *testing.T) {
	raw := `I will return the action now.
{"action":{"type":"EXECUTE_COMMAND","command_action":{"command":"printf '{hello}'"}}}
Done.`

	action, err := decodeActionResponse(raw)
	if err != nil {
		t.Fatalf("decodeActionResponse() error = %v", err)
	}
	if action.Type != models.ActionTypeExecuteCommand {
		t.Fatalf("action.Type = %q, want %q", action.Type, models.ActionTypeExecuteCommand)
	}
	if action.CommandAction == nil || action.CommandAction.Command != "printf '{hello}'" {
		t.Fatalf("action.CommandAction = %#v", action.CommandAction)
	}
}

func TestDecodeActionResponseFailsWithoutValidAction(t *testing.T) {
	if _, err := decodeActionResponse("The file visible in the shared workspace is: test-app/Dockerfile."); err == nil {
		t.Fatal("expected decodeActionResponse() to fail without a valid action object")
	}
}

func TestBuildPromptLMStudioKeepsFullContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", "off")
	req := &proto.GetActionRequest{
		Goal:    "wait for 2 seconds",
		History: strings.Repeat("history-line\n", 400),
		Variables: map[string]string{
			"SHORT": "ok",
			"LONG":  strings.Repeat("very-long-variable-value-", 40),
		},
		DirectoryListing: map[string]string{
			"pipeline.yaml": strings.Repeat("step: build-and-test\n", 300),
			"README.md":     strings.Repeat("documentation\n", 300),
			"script.sh":     strings.Repeat("echo hello\n", 300),
		},
	}

	prompt := client.buildPrompt(req)

	if strings.Contains(prompt, "[directory listing truncated") || strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected LM Studio prompt to keep full context, got: %s", prompt)
	}
	for _, expected := range []string{
		strings.Repeat("history-line\n", 400),
		strings.Repeat("very-long-variable-value-", 40),
		"--- File: README.md ---",
		"--- File: pipeline.yaml ---",
		"--- File: script.sh ---",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain full LM Studio context fragment %q", expected)
		}
	}
}

func TestBuildPromptGeminiKeepsLargeContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderGemini, "", "gemini-2.5-flash", "", "")
	req := &proto.GetActionRequest{
		Goal:    "show file",
		History: strings.Repeat("history-line\n", 50),
		DirectoryListing: map[string]string{
			"pipeline.yaml": strings.Repeat("step: build-and-test\n", 120),
		},
	}

	prompt := client.buildPrompt(req)

	if strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected Gemini prompt to remain untruncated")
	}
}

func TestLMStudioLoadsAreSerializedButChatsCanRunConcurrently(t *testing.T) {
	var activeLoads int32
	var maxActiveLoads int32
	var activeChats int32
	var maxActiveChats int32
	var loadedMu sync.Mutex
	loaded := map[string]bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			loadedMu.Lock()
			modelALoaded := loaded["model-a"]
			modelBLoaded := loaded["model-b"]
			loadedMu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s,%s]}`,
				lmStudioModelListItemForTest("model-a", modelALoaded),
				lmStudioModelListItemForTest("model-b", modelBLoaded),
			)
		case "/api/v1/models/load":
			var req struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode load request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			current := atomic.AddInt32(&activeLoads, 1)
			recordMaxInt32(&maxActiveLoads, current)
			time.Sleep(25 * time.Millisecond)
			atomic.AddInt32(&activeLoads, -1)

			loadedMu.Lock()
			loaded[req.Model] = true
			loadedMu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"type":"llm","instance_id":%q,"status":"loaded"}`, req.Model)
		case "/api/v1/chat":
			current := atomic.AddInt32(&activeChats, 1)
			recordMaxInt32(&maxActiveChats, current)
			time.Sleep(150 * time.Millisecond)
			atomic.AddInt32(&activeChats, -1)

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"output":[{"type":"message","content":"true"}]}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clientA := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", server.URL, "")
	clientB := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-b", server.URL, "")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, client := range []*LLMClient{clientA, clientB} {
		wg.Add(1)
		go func(client *LLMClient) {
			defer wg.Done()
			if _, err := client.callLMStudioForBoolean(t.Context(), "is this serialized?"); err != nil {
				errs <- err
			}
		}(client)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("callLMStudioForBoolean() error = %v", err)
	}
	if got := atomic.LoadInt32(&maxActiveLoads); got != 1 {
		t.Fatalf("max concurrent LM Studio loads = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&maxActiveChats); got != 2 {
		t.Fatalf("max concurrent LM Studio chats = %d, want 2", got)
	}
}

func lmStudioModelListItemForTest(model string, loaded bool) string {
	instances := "[]"
	if loaded {
		instances = fmt.Sprintf(`[{"id":%q}]`, model)
	}
	return fmt.Sprintf(`{"type":"llm","key":%q,"loaded_instances":%s}`, model, instances)
}

func recordMaxInt32(target *int32, current int32) {
	for {
		observed := atomic.LoadInt32(target)
		if current <= observed || atomic.CompareAndSwapInt32(target, observed, current) {
			return
		}
	}
}
