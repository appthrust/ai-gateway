package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/sse"
)

// Closing the translated response must also interrupt an upstream read, not
// leave the translator alive until the upstream idle deadline.
type translatedBody struct {
	*io.PipeReader
	upstream io.Closer
}

func (b *translatedBody) Close() error {
	_ = b.upstream.Close()
	return b.PipeReader.Close()
}

func translateStream(response *http.Response, includeUsage bool) *http.Response {
	reader, writer := io.Pipe()
	translated := *response
	translated.Header = response.Header.Clone()
	translated.Header.Set("Content-Type", "text/event-stream")
	translated.Header.Del("Content-Length")
	translated.ContentLength = -1
	translated.Body = &translatedBody{PipeReader: reader, upstream: response.Body}
	go func() {
		defer response.Body.Close()
		err := anthropicStream(response.Body, writer, includeUsage)
		_ = writer.CloseWithError(err)
	}()
	return &translated
}

func anthropicStream(src io.Reader, dst io.Writer, includeUsage bool) error {
	var id, model string
	var usage anthropicUsage
	created := time.Now().Unix()
	toolIndexes := make(map[int]int)
	stopped := false
	emit := func(delta map[string]any, finish any, finalUsage bool) error {
		choices := []any{}
		if !finalUsage {
			choices = append(choices, map[string]any{"index": 0, "delta": delta, "finish_reason": finish})
		}
		chunk := map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": choices}
		if finalUsage {
			chunk["usage"] = openAIUsage(usage)
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(dst, "data: %s\n\n", data)
		return err
	}
	err := sse.Scan(src, func(_ []byte, data []byte) error {
		if len(data) == 0 {
			return nil
		}
		var event struct {
			Type         string           `json:"type"`
			Index        int              `json:"index"`
			Message      anthropicMessage `json:"message"`
			ContentBlock anthropicBlock   `json:"content_block"`
			Delta        struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				InputTokens  *int64 `json:"input_tokens"`
				OutputTokens *int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			return errors.New("invalid Anthropic stream event")
		}
		switch event.Type {
		case "message_start":
			id, model, usage = event.Message.ID, event.Message.Model, event.Message.Usage
			return emit(map[string]any{"role": "assistant", "content": ""}, nil, false)
		case "content_block_start":
			block := event.ContentBlock
			switch block.Type {
			case "text":
				if block.Text != "" {
					return emit(map[string]any{"content": block.Text}, nil, false)
				}
			case "tool_use":
				index := len(toolIndexes)
				toolIndexes[event.Index] = index
				return emit(map[string]any{"tool_calls": []any{map[string]any{"index": index, "id": block.ID, "type": "function", "function": map[string]string{"name": block.Name, "arguments": ""}}}}, nil, false)
			}
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				return emit(map[string]any{"content": event.Delta.Text}, nil, false)
			case "input_json_delta":
				index, ok := toolIndexes[event.Index]
				if !ok {
					return errors.New("tool delta without a tool block")
				}
				return emit(map[string]any{"tool_calls": []any{map[string]any{"index": index, "function": map[string]string{"arguments": event.Delta.PartialJSON}}}}, nil, false)
			}
		case "message_delta":
			if event.Usage.InputTokens != nil {
				usage.InputTokens = *event.Usage.InputTokens
			}
			if event.Usage.OutputTokens != nil {
				usage.OutputTokens = *event.Usage.OutputTokens
			}
			if event.Delta.StopReason != "" {
				return emit(map[string]any{}, finishReason(event.Delta.StopReason), false)
			}
		case "message_stop":
			if includeUsage {
				if err := emit(nil, nil, true); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(dst, "data: [DONE]\n\n"); err != nil {
				return err
			}
			stopped = true
			return io.EOF
		case "error":
			return errors.New("Anthropic streaming provider error")
		}
		return nil
	})
	if stopped && errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil && !stopped {
		return errors.New("Anthropic stream ended without message_stop")
	}
	return err
}
