// Minimal `text/event-stream` reader for `stream: true` chat completions: the
// gateway relays the provider's OpenAI events, one `data:` payload per event,
// ending with `data: [DONE]`. Only `data` fields matter; comments, `event:`,
// `id:` and `retry:` lines are ignored.

/** Feeds decoded text in any slicing; `onData` gets each event's joined `data` value. */
export function createSSEParser(onData: (data: string) => void) {
  let buffer = ''
  const emit = (event: string) => {
    const lines = event.split('\n').filter((line) => line.startsWith('data:'))
    if (lines.length === 0) return
    onData(lines.map((line) => line.slice(5).replace(/^ /, '')).join('\n'))
  }
  return {
    push(text: string) {
      buffer = (buffer + text).replace(/\r\n?/g, '\n')
      let end = buffer.indexOf('\n\n')
      while (end !== -1) {
        emit(buffer.slice(0, end))
        buffer = buffer.slice(end + 2)
        end = buffer.indexOf('\n\n')
      }
    },
    /** The body ended: a last event without its blank line still counts. */
    flush() {
      if (buffer.trim()) emit(buffer)
      buffer = ''
    },
  }
}
