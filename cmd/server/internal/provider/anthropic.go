package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const anthropicDefaultModel = "claude-sonnet-4-5"

type anthropic struct {
	base    string
	headers http.Header
	client  *http.Client
}

func NewAnthropic(key string, client *http.Client) Provider {
	return &anthropic{base: "https://api.anthropic.com/v1", headers: http.Header{
		"X-Api-Key": {key}, "Anthropic-Version": {"2023-06-01"},
	}, client: client}
}

func (p *anthropic) Name() string         { return "anthropic" }
func (p *anthropic) DefaultModel() string { return anthropicDefaultModel }

func (p *anthropic) Probe(ctx context.Context) (int, error) {
	return probe(ctx, p.client, p.base+"/models?limit=1", p.headers)
}

func (p *anthropic) Models(ctx context.Context) ([]ModelEntry, error) {
	var models []ModelEntry
	after := ""
	for {
		endpoint := p.base + "/models?limit=1000"
		if after != "" {
			endpoint += "&after_id=" + url.QueryEscape(after)
		}
		response, err := request(ctx, p.client, http.MethodGet, endpoint, nil, "application/json", p.headers)
		if err != nil {
			return nil, err
		}
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, errors.New("provider model listing failed")
		}
		err = json.NewDecoder(response.Body).Decode(&body)
		_ = response.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, model := range body.Data {
			models = append(models, NewModelEntry(model.ID, p.Name()))
		}
		if !body.HasMore {
			return models, nil
		}
		if body.LastID == "" || body.LastID == after {
			return nil, errors.New("invalid provider model pagination")
		}
		after = body.LastID
	}
}

func (p *anthropic) ChatCompletions(ctx context.Context, in Request, stream bool) (*http.Response, error) {
	body, err := anthropicRequest(in.Body, stream)
	if err != nil {
		return nil, err
	}
	response, err := request(ctx, p.client, http.MethodPost, p.base+"/messages", body, in.Accept, p.headers)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, err
	}
	if stream {
		var options struct {
			IncludeUsage bool `json:"include_usage"`
		}
		_ = json.Unmarshal(in.Body["stream_options"], &options)
		return translateStream(response, options.IncludeUsage), nil
	}
	defer response.Body.Close()
	var message anthropicMessage
	if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
		return nil, err
	}
	var text strings.Builder
	var tools []any
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			tools = append(tools, map[string]any{"id": block.ID, "type": "function", "function": map[string]any{"name": block.Name, "arguments": string(block.Input)}})
		}
	}
	outMessage := map[string]any{"role": "assistant", "content": text.String()}
	if len(tools) != 0 {
		outMessage["tool_calls"] = tools
	}
	out := map[string]any{"id": message.ID, "object": "chat.completion", "created": time.Now().Unix(), "model": message.Model,
		"choices": []any{map[string]any{"index": 0, "message": outMessage, "finish_reason": finishReason(message.StopReason)}},
		"usage":   openAIUsage(message.Usage)}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	translated := *response
	translated.Header = response.Header.Clone()
	translated.Header.Set("Content-Type", "application/json")
	translated.Header.Del("Content-Length")
	translated.ContentLength = int64(len(encoded))
	translated.Body = io.NopCloser(bytes.NewReader(encoded))
	return &translated, nil
}

type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type anthropicBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type anthropicMessage struct {
	ID         string           `json:"id"`
	Model      string           `json:"model"`
	Content    []anthropicBlock `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      anthropicUsage   `json:"usage"`
}

func openAIUsage(usage anthropicUsage) map[string]int64 {
	prompt := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	return map[string]int64{"prompt_tokens": prompt, "completion_tokens": usage.OutputTokens, "total_tokens": prompt + usage.OutputTokens}
}

func finishReason(reason string) string {
	switch reason {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// anthropicRequest supports text/content-part messages and function tools. It
// rejects unsupported content rather than silently dropping a user's input.
func anthropicRequest(in map[string]json.RawMessage, stream bool) (map[string]any, error) {
	var messages []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(in["messages"], &messages); err != nil || len(messages) == 0 {
		return nil, errors.New("messages are required")
	}
	out := map[string]any{"model": in["model"], "max_tokens": 4096, "stream": stream}
	for _, field := range []string{"max_tokens", "temperature", "top_p"} {
		if value, ok := in[field]; ok {
			out[field] = value
		}
	}
	if value, ok := in["max_completion_tokens"]; ok {
		out["max_tokens"] = value
	}
	if value, ok := in["stop"]; ok && string(value) != "null" {
		var stop string
		if json.Unmarshal(value, &stop) == nil {
			out["stop_sequences"] = []string{stop}
		} else {
			out["stop_sequences"] = value
		}
	}
	for _, field := range []string{"response_format", "audio", "modalities", "logprobs"} {
		if _, ok := in[field]; ok {
			return nil, errors.New("unsupported Anthropic chat option")
		}
	}
	if value, ok := in["n"]; ok && string(value) != "1" {
		return nil, errors.New("Anthropic supports one completion")
	}
	var system []any
	var converted []any
	for _, message := range messages {
		blocks, err := textBlocks(message.Content)
		if err != nil {
			return nil, err
		}
		role := message.Role
		switch role {
		case "system", "developer":
			system = append(system, blocks...)
			continue
		case "tool":
			role = "user"
			blocks = []any{map[string]any{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": blocks}}
		case "user", "assistant":
		default:
			return nil, errors.New("unsupported message role")
		}
		for _, call := range message.ToolCalls {
			if call.Type != "function" || !json.Valid([]byte(call.Function.Arguments)) {
				return nil, errors.New("invalid function tool call")
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": json.RawMessage(call.Function.Arguments)})
		}
		converted = append(converted, map[string]any{"role": role, "content": blocks})
	}
	out["messages"] = converted
	if len(system) != 0 {
		out["system"] = system
	}
	if raw, ok := in["tools"]; ok {
		var tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &tools); err != nil {
			return nil, err
		}
		convertedTools := make([]any, 0, len(tools))
		for _, tool := range tools {
			if tool.Type != "function" {
				return nil, errors.New("unsupported tool type")
			}
			convertedTools = append(convertedTools, map[string]any{"name": tool.Function.Name, "description": tool.Function.Description, "input_schema": tool.Function.Parameters})
		}
		out["tools"] = convertedTools
	}
	if raw, ok := in["tool_choice"]; ok {
		var choice string
		if json.Unmarshal(raw, &choice) == nil {
			switch choice {
			case "none":
				delete(out, "tools")
			case "auto":
				out["tool_choice"] = map[string]string{"type": "auto"}
			case "required":
				out["tool_choice"] = map[string]string{"type": "any"}
			default:
				return nil, errors.New("unsupported tool choice")
			}
		} else {
			var choice struct {
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if json.Unmarshal(raw, &choice) != nil || choice.Type != "function" || choice.Function.Name == "" {
				return nil, errors.New("invalid tool choice")
			}
			out["tool_choice"] = map[string]string{"type": "tool", "name": choice.Function.Name}
		}
	}
	return out, nil
}

func textBlocks(raw json.RawMessage) ([]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []any{map[string]string{"type": "text", "text": text}}, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, errors.New("invalid message content")
	}
	blocks := make([]any, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" {
			return nil, errors.New("unsupported non-text content")
		}
		blocks = append(blocks, map[string]string{"type": "text", "text": part.Text})
	}
	return blocks, nil
}
