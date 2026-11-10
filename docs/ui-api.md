# AI Gateway UI API

Contract between `cmd/server` and `web/`. Design authority:
platform `design/platform/ai-gateway/ui.md`. TypeScript mirror:
`web/src/api/types.ts` (kept identical by hand; no codegen).

Every authenticated `/v1/*` answer (success or error) carries
`X-Request-ID: <ring entry id>` so the Playground can find its own record in
`GET /api/ai-gateway/v1/invocations` exactly.

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
      {"kind":"CredentialSlot","name":"openai-secondary","satisfied":false},
      {"kind":"CredentialSlot","name":"anthropic-secondary","satisfied":false},
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
- `health`: the same pure any-provider evaluation as `/.well-known/appthrust/health`
  (`state` `Serving|Degraded|Failing|Unknown`, `reason` apphealth reason or empty).
  Health refreshes static probes after five minutes and refreshes links through
  workload-only discovery (resolver cache: 60 seconds), never caller authority.
  Status reads discover links and read existing key observations, without probing
  keys. `Unknown` means unchecked keys/discovery unavailable; idle alone is not
  Failing. A usable alternative plus a failed key is Serving; all known
  alternatives failing or known empty configuration is Failing. Policy Pending
  and required Full logging unavailable still block the whole capability.
  The existing traffic-derived Degraded/UpstreamDenied signal for a recent
  `ai_model_denied` is retained for up to five minutes while a provider is usable;
  a subsequent non-policy-denied chat outcome clears it. Failed alternative keys do not
  cause Degraded: they remain per-key findings and `nextSteps`.
  See [provider-keys.md](provider-keys.md) for the decision table and ownership.
- `defaultRoute`: result of resolving a request with `model` omitted.
  `source`: `Policy` (Enforced `Document.DefaultModel`), `EnvOverride`
  (`AI_GATEWAY_DEFAULT_PROVIDER/MODEL`), `Capability` (first in-Project
  provider), `Credential` (static adapter default). `null` when nothing resolves.
- `endpoints.internal` null when the internal Service name is unknown.
- `logging` null when the policy has no logging block.
- `nextSteps`: derived, ordered. Kinds: `BindCredential{slot}`,
  `InstallApp{definitionId}`, `BindPolicy`, `FixProvider{provider,slot?}`,
  `ContactAdmin{reason}` (Pending, Full logging). Each rejected/unreachable bound
  key gets its own FixProvider, even when another key is usable. Unbound optional
  slots do not cause remediation when the gateway is configured.

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
  "keys":[],
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
  "keys":[
    {"slot":"openai","bound":true,"state":"rejected","observedAt":"2026-11-09T09:58:00Z"},
    {"slot":"openai-secondary","bound":false,"state":"unchecked","observedAt":null}
  ],
  "models":[{"id":"gpt-5-mini","allowed":true,"default":false,"deniedBy":null},
            {"id":"gpt-4o","allowed":false,"default":false,"deniedBy":{"provider":"openai","patterns":["gpt-5*"]}}]
}]}
```

- Order: in-Project providers first, then registered static provider types
  (currently `openai`, `anthropic`), then unavailable policy-listed providers.
- `kind`: `InProjectApp` or the adapter registration's display kind (currently
  `OpenAI`, `Anthropic`); clients render unfamiliar adapter kinds without branching.
- `source`: exactly one of `{link, linkState}` or `{credentialSlot, bound}`.
  For static providers, `credentialSlot` is the selected slot (the primary slot
  if none are bound), and `bound` means at least one key is bound.
- `keys`: every declared slot of this provider, in registration order; `[]` for
  links. `slot` is a non-secret identifier, not a credential prefix or hash.
  `state`: `usable` (200), `rejected` (401/403), `unreachable` (transport or other
  unsuccessful status), `unchecked` (never checked or unbound). `observedAt` is
  the last completed probe/chat observation, null if unchecked. Last-known
  per-key state does not expire into a fabricated failure. Empty optional slots
  stay visible with `bound:false`; no key value is returned or accepted.
- Provider `state`: `Serving | Denied | Unreachable | NotConfigured |
  PolicyExcluded | NotInstalled | Unknown`. Static state/models/observedAt describe
  the selected key, while `keys` keeps failures of other keys visible.
  Selection prefers usable, then unchecked, then failed, in registration order.
  PolicyExcluded overrides the provider state but not its key observations.
  Link readiness and latest chat evidence determine capability health; the
  provider view exposes current link readiness. No retries or load balancing.
- `models`: from the provider's models listing (empty on failure);
  `allowed` per `Document.Allows`; `deniedBy` names the first failing rule's
  patterns when `allowed=false`, else null. Unrestricted: all allowed.
- `observedAt` null when never probed.

## `POST /api/ai-gateway/v1/providers/{name}/check` → `ProviderView`

Forces probes for bound keys of one static provider; each probe is single-flight.
`429 ai_check_rate_limited` when a targeted key was checked less than 30 s ago
(body carries `retryAfterSeconds`, header `Retry-After`). Earlier keys may have
been checked before a later key hits its cooldown. `404 ai_provider_unknown`.
In-Project providers return the current resolver view without probing.

## `POST /api/ai-gateway/v1/providers/{name}/keys/{slot}/check` → `ProviderView`

Checks exactly one bound credential slot, using its injected environment key,
and returns the refreshed parent provider including all `keys`.
Same authentication and per-key 30-second cooldown as the provider check.
Other slots have independent cooldowns. `404 ai_key_unknown` with message
`The AI provider key is unknown or unbound` for an unknown/mismatched/unbound
slot. Rate-limit message: `Provider was checked recently`.
There is no request body and no key-value API. The Providers page shows every
slot, its provider, binding/check state, time and re-check action; changes to the
value go to the Dashboard AI Gateway installation. Settings → AI edits policy.

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
