package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/appthrust/ai-gateway/cmd/server/internal/sse"
)

func requestBody(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestAnthropicRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "provider-key" || r.Header.Get("Anthropic-Version") != "2023-06-01" || r.Header.Get("Authorization") != "" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("provider request %s %s %v", r.Method, r.URL, r.Header)
		}
		var body struct {
			Model  string `json:"model"`
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
			MaxTokens int      `json:"max_tokens"`
			Stop      []string `json:"stop_sequences"`
			Stream    bool     `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "claude-model" || body.MaxTokens != 25 || body.Stream || len(body.System) != 1 || body.System[0].Text != "be brief" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content[0].Text != "hello" || len(body.Stop) != 1 || body.Stop[0] != "END" {
			t.Errorf("translated request=%+v", body)
		}
		fmt.Fprint(w, `{"id":"msg-1","model":"claude-model","content":[{"type":"text","text":"hi "},{"type":"text","text":"there"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":3}}`)
	}))
	defer server.Close()
	p := NewAnthropic("provider-key", NewClient()).(*anthropic)
	p.base = server.URL + "/v1"
	response, err := p.ChatCompletions(context.Background(), Request{Accept: "application/json", Body: requestBody(t,
		`{"model":"claude-model","messages":[{"role":"system","content":"be brief"},{"role":"user","content":[{"type":"text","text":"hello"}]}],"max_completion_tokens":25,"stop":"END"}`)}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message      struct{ Role, Content string }
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Total      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "msg-1" || body.Object != "chat.completion" || body.Model != "claude-model" || len(body.Choices) != 1 || body.Choices[0].Message.Content != "hi there" || body.Choices[0].Message.Role != "assistant" || body.Choices[0].FinishReason != "stop" || body.Usage.Prompt != 8 || body.Usage.Completion != 2 || body.Usage.Total != 10 {
		t.Fatalf("translated response=%+v", body)
	}
}

const anthropicEvents = `event: message_start
data: {"type":"message_start","message":{"id":"msg-1","model":"claude-model","usage":{"input_tokens":5,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`

func TestAnthropicStreamingTranslation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if string(body["stream"]) != "true" {
			t.Errorf("stream=%s", body["stream"])
		}
		if _, ok := body["stream_options"]; ok {
			t.Error("OpenAI-only stream_options leaked to Anthropic")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, anthropicEvents)
	}))
	defer server.Close()
	p := NewAnthropic("key", NewClient()).(*anthropic)
	p.base = server.URL
	response, err := p.ChatCompletions(context.Background(), Request{Body: requestBody(t, `{"model":"claude-model","messages":[{"role":"user","content":"hi"}],"stream_options":{"include_usage":true}}`)}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var text, finish string
	var prompt, completion int
	done := false
	err = sse.Scan(response.Body, func(_ []byte, data []byte) error {
		if string(data) == "[DONE]" {
			done = true
			return nil
		}
		var chunk struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				Prompt     int `json:"prompt_tokens"`
				Completion int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if chunk.ID != "msg-1" || chunk.Model != "claude-model" || chunk.Object != "chat.completion.chunk" {
			t.Errorf("chunk identity=%s", data)
		}
		for _, choice := range chunk.Choices {
			text += choice.Delta.Content
			if choice.FinishReason != nil {
				finish = *choice.FinishReason
			}
		}
		if chunk.Usage != nil {
			prompt, completion = chunk.Usage.Prompt, chunk.Usage.Completion
			if len(chunk.Choices) != 0 {
				t.Error("usage chunk must have empty choices")
			}
		}
		return nil
	})
	if err != nil || !done || text != "hello world" || finish != "stop" || prompt != 5 || completion != 2 {
		t.Fatalf("translation text=%q finish=%q tokens=%d/%d done=%t err=%v", text, finish, prompt, completion, done, err)
	}
}

func TestAnthropicToolRequestAndResponse(t *testing.T) {
	body, err := anthropicRequest(requestBody(t, `{"model":"m","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]},{"role":"tool","tool_call_id":"call1","content":"result"}],"tools":[{"type":"function","function":{"name":"lookup","description":"Find","parameters":{"type":"object"}}}],"tool_choice":"required"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"tool_use"`, `"input":{"id":1}`, `"type":"tool_result"`, `"tool_use_id":"call1"`, `"input_schema":{"type":"object"}`, `"tool_choice":{"type":"any"}`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("missing %s in %s", want, encoded)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"m1","model":"m","content":[{"type":"tool_use","id":"c1","name":"lookup","input":{"id":1}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":2}}`)
	}))
	defer server.Close()
	p := NewAnthropic("key", NewClient()).(*anthropic)
	p.base = server.URL
	response, err := p.ChatCompletions(context.Background(), Request{Body: requestBody(t, `{"model":"m","messages":[{"role":"user","content":"lookup"}]}`)}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	encoded, err = io.ReadAll(response.Body)
	if err != nil || !strings.Contains(string(encoded), `"arguments":"{\"id\":1}"`) || !strings.Contains(string(encoded), `"finish_reason":"tool_calls"`) {
		t.Fatalf("tool response=%s err=%v", encoded, err)
	}
}

func TestAnthropicRejectsUnsupportedContentAndBrokenStreams(t *testing.T) {
	_, err := anthropicRequest(requestBody(t, `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/image"}}]}]}`), false)
	if err == nil {
		t.Fatal("must not silently discard image input")
	}
	for _, stream := range []string{
		"data: {\"type\":\"error\",\"error\":{\"message\":\"private\"}}\n\n",
		"data: {\"type\":\"message_start\",\"message\":{}}\n\n",
	} {
		var output strings.Builder
		if err := anthropicStream(strings.NewReader(stream), &output, true); err == nil || strings.Contains(output.String(), "[DONE]") || strings.Contains(output.String(), "private") {
			t.Fatalf("broken stream output=%s error=%v", output.String(), err)
		}
	}
}

func TestAnthropicModelsSynthesizeOpenAIEntriesAcrossPages(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.URL.Query().Get("limit") != "1000" || r.Header.Get("X-Api-Key") != "provider-key" {
			t.Errorf("model request=%s %s headers=%v", r.Method, r.URL, r.Header)
		}
		switch calls {
		case 1:
			if r.URL.Query().Get("after_id") != "" {
				t.Errorf("first page query=%s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"data":[{"id":"model-a","type":"model","display_name":"Model A"}],"has_more":true,"last_id":"model-a"}`)
		case 2:
			if r.URL.Query().Get("after_id") != "model-a" {
				t.Errorf("second page query=%s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"data":[{"id":"model-b","type":"model"}],"has_more":false}`)
		default:
			t.Error("unexpected extra model page")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	p := NewAnthropic("provider-key", server.Client()).(*anthropic)
	p.base = server.URL + "/v1"
	models, err := p.Models(context.Background())
	if err != nil || calls != 2 || len(models) != 2 || models[0].ID != "model-a" || models[1].ID != "model-b" {
		t.Fatalf("models=%v calls=%d err=%v", models, calls, err)
	}
	encoded, err := json.Marshal(models)
	if err != nil || string(encoded) != `[{"created":0,"id":"model-a","object":"model","owned_by":"anthropic"},{"created":0,"id":"model-b","object":"model","owned_by":"anthropic"}]` {
		t.Fatalf("models wire=%s err=%v", encoded, err)
	}
}
