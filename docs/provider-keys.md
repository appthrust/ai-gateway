# Provider keys and capability health

## Domain model and ownership

- **Provider**: a routing/policy identity (`openai`, `anthropic`, or a linked app definition). An adapter registration supplies its kind and constructor; adding an adapter does not add provider-name branches to registry, health, or UI code.
- **Credential slot**: a named, optional input owned by the catalog. A provider can have several slots. `credentialSlots` in `cmd/server/credentials.go` is the registration authority; `catalog/entry.yaml` is its published projection, compared to the table by a Go test.
- **ExternalAPICredentialRevision**: immutable platform credential revision backed by a platform-owned Secret. A slot binds one revision; several slots can use the same provider. The gateway is only an environment consumer/observer, not a second secret store.
- **Key observation**: process-local state and check time for one slot, independent of other keys of the same provider. It contains no value, prefix, hash, revision content, or upstream diagnostics.
- **Link**: workload-credential discovery's Ready chat-completions binding. A link is not an API key. Routing uses the resolver's current binding, never a second long-lived routing cache.

## Existing platform contracts (read-only investigation)

The deployed built-in-app contract is documented in platform `design/platform/builtin-app.md`, “Domain model” / “Projection model”: `BuiltinApp.spec.credentials[].{name,revisionRef}` projects to `Application.spec.credentialBindings`; the catalog declares slot names, provider types and environment names. Its documented route is `PUT /api/v1/projects/{projectId}/builtin-apps/{id}/credentials/{name}`. `design/platform/ai-gateway/README.md`, “ai-gateway built-in app”, declares `openai` / `anthropic` and the corresponding `AI_GATEWAY_*_API_KEY` inputs.

The current platform checkout has moved installation orchestration to `platform-api/internal/handler/app_installation.go` (app-installations routes), while credentials belong to the generated Application: `ApplicationHandler.Mount` in `platform-api/internal/handler/application.go` registers PUT/DELETE `/api/v1/projects/{projectId}/applications/{id}/credentials/{name}`. `mutateApplicationCredential` in `application_credential.go` looks up a declared slot by name, replaces that name's binding with a new revision, and retires the previous revision. It does not enforce one revision per provider. Arbitrary undeclared names are rejected (`application credential is not declared`). Authorization there requires a team admin.

`model/README.md`, “Application credential slots”, specifies up to eight slots per Component, unique environment names, no provider-level uniqueness, and no credential values in status. `appthrust-operator/pkg/controllers/core/applicationbranch/credential_resolver.go:composeCredentials` calls `applicationcredential.Resolve` and projects `slot.Env` into the runtime environment Secret; unbinding removes the old value without falling back to user config. The gateway reads injected environment values at process startup only.

Dashboard `dashboard/src/features/project-ai-policy/components/project-ai-policy-tab.tsx:ProjectAIPolicyTab` implements Project Settings → AI as policy binding/narrowing, not provider-key CRUD. The gateway links to the AI Gateway installation (`/dashboard/projects/{projectId}/marketplace/ai-gateway`) for credential changes and to Settings for policy. It must not imply that Settings → AI already edits keys.

## Registration and selection

The existing slots keep their names and environment variables. Optional second slots are `openai-secondary` (`AI_GATEWAY_OPENAI_SECONDARY_API_KEY`) and `anthropic-secondary` (`AI_GATEWAY_ANTHROPIC_SECONDARY_API_KEY`). The registration table supplies display names, provider references and environment names; all keys are read through one loop. Empty values are unbound. There is no key-value mutation API.

Policy and inference continue to use provider names, never slot names. Within a provider, prefer a key whose latest check succeeded, then an unchecked key, then a failed key, in registration order. This is deterministic selection, not round-robin or automatic retry: a request is sent once. A subsequent request can use another key after failure evidence. Linked providers remain ahead of static providers for an omitted unrestricted provider. Observations never authorize a withdrawn link.

## Lifecycle and invariants

1. **Add**: bind a declared slot in the Dashboard/platform credential API. Platform projects its Secret and rolls the environment; the new process registers the key as unchecked.
2. **Rotate**: replace the same slot's revision in the platform. On rollout the slot keeps its identity but its process-local observation starts unchecked. Old observations never attest to a new process's credential.
3. **Disable**: unbind the slot. After the new environment is served, it is excluded from routing and shown unbound/unchecked. This is not a reversible pause retaining the binding.
4. **Remove**: unbind; platform retires the revision/Secret. The catalog-declared empty slot remains visible. Removing arbitrary user-created slot definitions is not a gateway operation.
5. **Re-check**: authenticated observation only, using the injected key. A per-slot single-flight probe has a 30-second cooldown; other slots remain independently checkable. Values are never accepted, persisted, logged, returned, fingerprinted or rendered by this API/UI.

Per-key states: `usable` (HTTP 200), `rejected` (401/403), `unreachable` (transport or other unsuccessful HTTP status), `unchecked` (unbound or no completed check). `bound` distinguishes an absent key. State is last-known, with the actual last check time; idle time does not relabel a rejected key. Ordinary checks refresh after five minutes. Real chat outcomes update only the selected key/provider; one request's failure cannot overwrite every alternative.

## Health decision

The pure decision combines eligible provider observations, not the number of static keys. Policy Pending or required Full logging unavailable remains Failing. Policy-excluded providers do not count as usable alternatives.

| Evidence | Capability |
| --- | --- |
| At least one usable key/Ready link, no failed eligible alternative | Serving |
| At least one usable key/Ready link, another eligible key/provider failed | Serving (the failing key remains visible) |
| No usable alternative, at least one unchecked provider or discovery unavailable | Unknown / NoEvidence or DependencyUnavailable |
| No usable or unchecked alternative; discovery succeeded empty, no configuration, or all eligible providers failed | Failing |

A failed alternative is a per-key/provider finding, not a capability failure.
The platform maps required-capability Degraded to `Healthy=False` and the
Dashboard's degraded phase, so reporting Degraded merely for a redundant broken
key would preserve the original misjudgment. The existing recent policy-denial
signal (`ai_model_denied` → Degraded/UpstreamDenied) is retained only while a
provider is usable; it expires after five minutes or is cleared by the next
non-policy-denied chat outcome. No new Degraded source is added.

Health refreshes discovery using the **gateway workload credential** already owned by `main.go:NewGatewayDiscovery`; it supplies an empty caller bearer and a new bounded background context, with no request values or authorization. This changes the old test's prohibition on all unauthenticated discovery, not its security intent. It neither retains caller credentials nor uses caller authority. The link resolver retains its existing 60-second cache; health probes never retain binding URLs as a second routing cache. An idle link-only gateway therefore stays Serving when discovery says Ready. Discovery unavailable with no other proven usable provider is Unknown, not Failing. This matches platform `design/platform/builtin-app-health/README.md`: Failing means no request can be served, Degraded means partial availability, Unknown means insufficient evidence. Probe refreshes make idle state independent of inference traffic.

## Required platform changes

- **Arbitrary user-named/count provider keys**: product must decide limits/naming and expose declaration plus binding in installation configuration. Extend the catalog/installation-to-Component contract (`model/api/core/v1alpha1/component.go:ComponentCredentialSlot`, platform `platform-api/internal/handler/application_stack_marketplace_render.go`, `component_credential_slot.go`, `application_credential.go`), and deliver non-secret registration metadata to the gateway rather than embedding per-Project slot names in an app release. The current two slots per adapter are a bounded catalog choice, not dynamic CRUD.
- **Project Settings → AI credential management**: extend `dashboard/src/features/project-ai-policy/components/project-ai-policy-tab.tsx` and its API client contracts to list slots and invoke the owning Application credential routes (or a stable installation facade). Do not create another credential store. Retain platform authorization, idempotency and revision rotation.
- **Reversible disable distinct from unbind**: add an enabled/disabled binding contract to the platform's Application credential model, `application_credential.go` and `appthrust-operator/pkg/controllers/core/applicationbranch/credential_resolver.go`. Today unbind is the available disable operation.

## Deviations and open product decisions

- The supplied deployed builtin-app credential contract and the current platform checkout's Application/app-installation contracts differ; this repo does not migrate either platform API.
- Product owner: approve the two-keys-per-provider catalog names/count, versus a future arbitrary-named slot inventory (bounded by the platform's eight-slot Component limit today).
- Product owner: decide whether to add a reversible disable control and whether Settings → AI should own the credential-management entry point. This implementation points to the existing installation owner instead.
- No load balancing, per-request key choice, retry, gateway secret storage, release or deployment is introduced. Checks and status are process-local, not durable key audit history.

### Observation precedence

A static key's completed chat result updates only that selected slot (successful
HTTP 200, 401/403, transport failure or 5xx); ordinary caller/model validation
errors do not poison per-key health. A chat observation newer than an in-flight probe
wins. For a linked provider, a failed chat is evidence against that link for
five minutes; afterwards refreshed Ready discovery supersedes that old failure.
Thus “usable/Ready link” in the table means a current Ready link without a
newer, still-current failed chat observation. Other usable providers keep the
capability Serving; the failed alternative remains visible separately.
