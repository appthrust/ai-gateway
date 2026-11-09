import { readFileSync } from 'node:fs'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig, type ProxyOptions } from 'vite'

// Dev transport mirrors production: cmd/server serves the SPA, the read model
// (`/api/ai-gateway/v1/*`), the public OpenAI-compatible routes (`/v1/*`),
// `/config` and the app switcher's `/launcher` from one origin, and the OIDC
// edge (Envoy) in front of it attaches Authorization. In dev all of them go to
// the in-repo mock (`bun run mock`, src/dev/mock-server.ts), which needs no
// token. To exercise a real gateway instead, point AI_GATEWAY_API_UPSTREAM at it
// and either set AI_GATEWAY_DEV_BEARER to the token the edge would attach (a
// local Go server) or AI_GATEWAY_DEV_COOKIE_FILE to a file holding the `Cookie`
// header of an authenticated browser session (re-read per request so a
// refresher can rotate it). The proxy drops the browser Origin so upstreams
// treat calls as first-party.
// The mock port sits below Linux's ephemeral range (32768+), where outbound connections could already hold it.
const upstream = process.env.AI_GATEWAY_API_UPSTREAM ?? `http://127.0.0.1:${process.env.AI_GATEWAY_MOCK_PORT ?? 5186}`
const devBearer = process.env.AI_GATEWAY_DEV_BEARER
const devCookieFile = process.env.AI_GATEWAY_DEV_COOKIE_FILE
const proxy: ProxyOptions = {
  target: upstream,
  changeOrigin: true,
  configure(server) {
    server.on('proxyReq', (request) => {
      request.removeHeader('origin')
      if (devBearer) request.setHeader('authorization', `Bearer ${devBearer}`)
      if (devCookieFile) request.setHeader('cookie', readFileSync(devCookieFile, 'utf8').trim())
    })
  },
}

export default defineConfig({
  plugins: [tailwindcss()],
  esbuild: { jsx: 'automatic' },
  server: {
    port: Number(process.env.AI_GATEWAY_PORT ?? 5188),
    strictPort: true,
    proxy: { '/api': proxy, '/v1': proxy, '/launcher': proxy, '/config': proxy },
  },
})
