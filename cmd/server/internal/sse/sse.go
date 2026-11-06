// Package sse reads bounded events without buffering an entire conversation.
package sse

import (
	"bufio"
	"bytes"
	"io"
)

// Scan preserves event bytes for transparent proxying, including comments and
// CRLF. The data argument joins multiple data lines according to the SSE spec.
// An event, not the complete stream, is limited to 4 MiB.
func Scan(r io.Reader, visit func(event, data []byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	scanner.Split(splitEvent)
	for scanner.Scan() {
		event := scanner.Bytes()
		var data []byte
		seen, joined := false, false
		for line := range bytes.SplitSeq(event, []byte{'\n'}) {
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			line = bytes.TrimPrefix(line[5:], []byte{' '})
			if !seen {
				// The common single-data-line event needs no copy.
				data, seen = line, true
				continue
			}
			if !joined {
				// Keep the original event intact for transparent proxying.
				data, joined = bytes.Clone(data), true
			}
			data = append(data, '\n')
			data = append(data, line...)
		}
		if err := visit(event, data); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func splitEvent(data []byte, atEOF bool) (int, []byte, error) {
	lineStart := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		if i == lineStart || i == lineStart+1 && data[lineStart] == '\r' {
			return i + 1, data[:i+1], nil
		}
		lineStart = i + 1
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
