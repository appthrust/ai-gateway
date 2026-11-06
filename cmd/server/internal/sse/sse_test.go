package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type fragmentReader struct{ text string }

func (r *fragmentReader) Read(p []byte) (int, error) {
	if r.text == "" {
		return 0, io.EOF
	}
	p[0] = r.text[0]
	r.text = r.text[1:]
	return 1, nil
}

func TestFragmentedEventsPreserveWireBytesAndJoinData(t *testing.T) {
	input := ": heartbeat\r\n\r\ndata: first\r\ndata: second\r\n\r\ndata: [DONE]\n\n"
	var wire strings.Builder
	var payloads []string
	err := Scan(&fragmentReader{text: input}, func(event, data []byte) error {
		wire.Write(event)
		payloads = append(payloads, string(data))
		return nil
	})
	if err != nil || wire.String() != input || len(payloads) != 3 || payloads[0] != "" || payloads[1] != "first\nsecond" || payloads[2] != "[DONE]" {
		t.Fatalf("wire=%q data=%q error=%v", wire.String(), payloads, err)
	}
}

func TestVisitorCanTerminateWithoutReadingToEOF(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := Scan(strings.NewReader("data: [DONE]\n\ndata: ignored\n\n"), func(_, _ []byte) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestOversizedEventFailsBoundedly(t *testing.T) {
	called := false
	err := Scan(strings.NewReader("data: "+strings.Repeat("x", 4<<20)+"\n\n"), func(_, _ []byte) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("oversized event accepted: called=%t error=%v", called, err)
	}
}

func TestEmptyDataLinesKeepTheirSeparators(t *testing.T) {
	input := "data:\ndata: second\ndata:\n\n"
	err := Scan(strings.NewReader(input), func(event, data []byte) error {
		if string(data) != "\nsecond\n" || string(event) != input {
			t.Fatalf("event=%q data=%q", event, data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
