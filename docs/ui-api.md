# AI Gateway UI API

Contract between `cmd/server` and `web/`. Design authority:
platform `design/platform/ai-gateway/ui.md`. TypeScript mirror:
`web/src/api/types.ts` (kept identical by hand; no codegen).

All routes below require the same authenticated Project-member caller as
`/v1/*` (`Authorization` bearer attached by the OIDC edge). Errors use the
gateway envelope `{"error":{"code","message"}}`; `401 ai_unauthorized`,
`503 identity_unavailable`. Responses are `application/json`,
`Cache-Control: no-store`. Timestamps are RFC 3339 UTC. No field ever carries a
credential, an upstream URL, or a request/response body.

## `GET /config` (unauthenticated, like other apps)

```json
{"projectId":"pj-…","publicUrl":"https://ai-gateway.pj-….appthrust.ink","dashboardUrl":"https://dashboard.appthrust.ink"}
```

`dashboardUrl` is omitted when `PLATFORM_DASHBOARD_URL` is unset.

## `GET /api/ai-gateway/v1/status` → `GatewayStatus`

```json
{
  "mode": "Enforced",
  "policy": {"name":"default","uid":"…","generation":3},
  "configured": {
    "satisfied": true,
    "candidates": [
      {"kind":"CredentialSlot","name":"openai","satisfied":false},
      {"kind":"CredentialSlot","name":"anthropic","satisfied":false},
      {"kind":"Link","name":"llm","satisfied":true}
    ]
  },
  "health": {"state":"Serving","reason":"","message":"","observedAt":"2026-11-09T10:00:00Z"},
  "defaultRoute": {"provider":"llm","model":"gemma3:4b","source":"Policy"},
  "endpoints": {"public":"https://ai-gateway.pj-….appthrust.ink/v1","internal":"http://ai-gateway-web.pj-….svc/v1"},
  "release": {"version":"0.3.0","artifactDigest":"sha256:…"},
  "logging": {"conversation":"HashOnly","toolCalls":"Off","retention":"90d"},
  "nextSteps": [
    {"kind":"BindCredential","slot":"openai"},
    {"kind":"InstallApp","definitionId":"inference"},
    {"kind":"BindPolicy"}
  ]
}
```

- `mode`: `Unrestricted | Enforced | Pending`. `policy` null when Unrestricted.
- `configured.candidates[].kind`: `CredentialSlot` (env key present) or `Link`
  (a Ready `chat-completions.openai/v1` binding). `satisfied` is any-of.
- `health`: the same evaluation as `/.well-known/appthrust/health`
  (`state` `Serving|Degraded|Failing|Unknown`, `reason` apphealth reason or empty).
- `defaultRoute`: result of resolving a request with `model` omitted.
  `source`: `Policy` (Enforced `Document.DefaultModel`), `EnvOverride`
  (`AI_GATEWAY_DEFAULT_PROVIDER/MODEL`), `Capability` (first in-Project
  provider), `Credential` (static adapter default). `null` when nothing resolves.
- `endpoints.internal` null when the internal Service name is unknown.
- `logging` null when the policy has no logging block.
- `nextSteps`: derived, ordered; empty when configured and serving. Kinds:
  `BindCredential{slot}`, `InstallApp{definitionId}`, `BindPolicy`,
  `FixProvider{provider}` (Denied/Unreachable), `ContactAdmin{reason}`
  (Pending, Full logging).

## `GET /api/ai-gateway/v1/providers` → `{"items": ProviderView[]}`

```json
{"items":[{
  "name":"llm",
  "kind":"InProjectApp",
  "source":{"link":"llm","linkState":"Ready"},
  "state":"Serving",
  "reason":"",
  "message":"",
  "observedAt":"2026-11-09T10:00:00Z",
  "embeddings":false,
  "models":[{"id":"gemma3:4b","allowed":true,"default":true,"deniedBy":null}]
},{
  "name":"openai",
  "kind":"OpenAI",
  "source":{"credentialSlot":"openai","bound":true},
  "state":"Denied",
  "reason":"UpstreamDenied",
  "message":"Provider rejected the configured credential",
  "observedAt":"2026-11-09T09:58:00Z",
  "embeddings":true,
  "models":[{"id":"gpt-5-mini","allowed":true,"default":false,"deniedBy":null},
            {"id":"gpt-4o","allowed":false,"default":false,"deniedBy":{"provider":"openai","patterns":["gpt-5*"]}}]
}]}
```

- Order: in-Project providers (Ready links) first, then `openai`, `anthropic`;
  then policy-listed providers that are not available (`state`
  `NotConfigured | NotInstalled | PolicyExcluded`).
- `kind`: `InProjectApp | OpenAI | Anthropic`.
- `source`: exactly one of `{link, linkState}` or `{credentialSlot, bound}`.
- `state`: `Serving | Denied | Unreachable | NotConfigured | PolicyExcluded | NotInstalled | Unknown`.
  Denied = probe 401/403; Unreachable = connect/5xx; NotConfigured = slot
  unbound; PolicyExcluded = Enforced and provider not in `Document.Providers`;
  NotInstalled = policy or catalog names the provider but no Ready link;
  Unknown = not probed yet.
- `models`: from the provider's models listing (empty on failure);
  `allowed` per `Document.Allows`; `deniedBy` names the first failing rule's
  patterns when `allowed=false`, else null. Unrestricted: all allowed.
- `observedAt` null when never probed.

## `POST /api/ai-gateway/v1/providers/{name}/check` → `ProviderView`

Forces a probe for one static provider, single-flight. `429
ai_check_rate_limited` when the last probe is younger than 30 s (body carries
`retryAfterSeconds`). `404 ai_provider_unknown`. In-Project providers return
the current resolver view without probing (their readiness is the link's).

## `GET /api/ai-gateway/v1/invocations` → `InvocationPage`

Query: `outcome=Allowed|Denied|Error`, `provider=`, `model=`, `caller=me|all`
(default `all`), `limit=` (default 100, max 500). Newest first.

```json
{
  "since":"2026-11-09T09:00:00Z",
  "instance":"ai-gateway-web-00007-deployment-abc12",
  "capacity":500,
  "items":[{
    "id":"01J…",
    "time":"2026-11-09T10:01:02Z",
    "route":"chat",
    "caller":{"subject":"…","displayName":"Jane Doe"},
    "provider":"llm","model":"gemma3:4b",
    "outcome":"Allowed","status":200,"reason":"",
    "durationMs":1234,
    "promptTokens":120,"completionTokens":48,
    "requestDigest":"sha256:…","completionDigest":"sha256:…",
    "policy":{"name":"default","uid":"…"},
    "stream":true,
    "report":{"state":"Delivered"}
  }]
}
```

- `route`: `chat | embeddings | models`.
- `caller.displayName` is `identity.Name`, else `Email`, else empty.
- `reason`: the gateway error code on non-Allowed outcomes (`ai_model_denied`…).
- `report.state`: `Pending | Delivered | Dropped`; `Dropped` adds
  `reason` (`queue_full|queue_bytes|rejected|expired|shutdown|encode`) and
  `status` for `rejected`.
- `since` is process start; `instance` is `HOSTNAME`. Ring is process-local.
- Unauthenticated requests (401 before identity) are not recorded.
