// Dev-only API mock: `bun src/dev/mock-server.ts` (or `bun run mock`). Serves
// the gateway contract (./mock-gateway: `/config`, `/api/ai-gateway/v1/*`,
// `/v1/*`) and the app switcher's `GET /launcher` on one port; Vite proxies all
// of them here (vite.config.ts). MOCK_EMPTY_RING=1 shows the empty ring,
// MOCK_NO_DASHBOARD=1 omits `dashboardUrl` (deep links become text).
import { createMockLauncherHandler, mockLauncherEntries } from '@appthrust/app-launcher'
import { createMockGateway } from './mock-gateway'

const port = Number(process.env.AI_GATEWAY_MOCK_PORT ?? 5186)
// The shared fixture lists the other first-party apps; this app is the current entry.
const launcher = createMockLauncherHandler([
  ...mockLauncherEntries('ai-gateway'),
  {
    definitionId: 'ai-gateway',
    name: 'AI Gateway',
    description: 'Send AI requests only to the providers and models your AI policy allows.',
    icon: 'sparkles',
    url: `http://127.0.0.1:${process.env.AI_GATEWAY_PORT ?? 5188}/`,
    phase: 'Ready',
    current: true,
  },
])
const gateway = createMockGateway()

const server = Bun.serve({
  port,
  hostname: '127.0.0.1',
  idleTimeout: 60,
  async fetch(request) {
    const url = new URL(request.url)
    return (
      launcher(request) ??
      (await gateway(request, url)) ??
      Response.json({ error: { code: 'not_found', message: `no mock for ${request.method} ${url.pathname}` } }, { status: 404 })
    )
  },
})

console.log(`ai-gateway mock API on http://${server.hostname}:${server.port}`)
