import { describe, expect, test } from 'bun:test'
import { createGatewayApi, GatewayApiError, type ChatChunk } from './client'
import { createSSEParser } from './sse'

describe('server-sent events', () => {
  test('data payloads are joined per event across any slicing; other fields are ignored', () => {
    const events: string[] = []
    const parser = createSSEParser((data) => events.push(data))
    parser.push(': keep-alive\n\nda')
    parser.push('ta: {"a":1}\r\n\r\nevent: x\ndata: line1\ndata: line2\n\ndata: [DONE]')
    parser.flush()
    expect(events).toEqual(['{"a":1}', 'line1\nline2', '[DONE]'])
  })
})

describe('gateway client', () => {
  const noReauth = () => false

  test('a streamed chat yields each chunk and stops at [DONE]', async () => {
    const body = ['data: {"choices":[{"delta":{"content":"Hel"}}]}\n\n', 'data: {"choices":[{"delta":{"content":"lo"}}],"usage":{"prompt_tokens":3}}\n\n', 'data: [DONE]\n\n']
    const fetcher = async (_input: string, init: RequestInit) => {
      expect(JSON.parse(String(init.body))).toEqual({ model: 'm', messages: [{ role: 'user', content: 'hi' }], stream: true })
      return new Response(new ReadableStream({ start: (controller) => (body.forEach((part) => controller.enqueue(new TextEncoder().encode(part))), controller.close()) }), { status: 200 })
    }
    const chunks: ChatChunk[] = []
    const answer = await createGatewayApi(fetcher, noReauth).chatStream({ model: 'm', messages: [{ role: 'user', content: 'hi' }] }, (chunk) => chunks.push(chunk))
    expect(answer.status).toBe(200)
    expect(chunks.map((chunk) => chunk.choices?.[0]?.delta?.content).join('')).toBe('Hello')
    expect(chunks[1]?.usage?.prompt_tokens).toBe(3)
  })

  test('the gateway error envelope becomes a coded error; 429 keeps retryAfterSeconds; a bare 502 is transient', async () => {
    const answer = (status: number, body?: unknown) => async () => new Response(body === undefined ? 'bad gateway' : JSON.stringify(body), { status })
    const denied = await createGatewayApi(answer(403, { error: { code: 'ai_model_denied', message: 'no' } }), noReauth).chat({ messages: [] }).catch((error) => error)
    expect(denied).toBeInstanceOf(GatewayApiError)
    expect([denied.status, denied.code, denied.transient]).toEqual([403, 'ai_model_denied', false])
    const limited = await createGatewayApi(answer(429, { error: { code: 'ai_check_rate_limited', message: 'wait', retryAfterSeconds: 12 } }), noReauth).check('openai').catch((error) => error)
    expect(limited.retryAfterSeconds).toBe(12)
    const proxy = await createGatewayApi(answer(502), noReauth).status().catch((error) => error)
    expect([proxy.code, proxy.transient]).toEqual(['unknown', true])
  })
})
