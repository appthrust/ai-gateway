# AI Gateway

AI Gateway is AppThrust's Project AI gateway. It routes OpenAI-compatible chat
completions, embeddings, and model discovery to Project-local capability
providers or configured OpenAI and Anthropic adapters, enforces the Project AI
policy, and records invocations through the Platform API. It is released
independently of the platform.

The server exposes `POST /v1/chat/completions`, `POST /v1/embeddings`, and
`GET /v1/models`. AI routes require an authenticated caller; provider discovery
uses the gateway workload credential rather than forwarding caller authority.
Embeddings require an adapter that supports them. `/healthz`, `/readyz`,
`/releasez`, and application health are also available.

The operational UI is served at `/` with SPA navigation. `GET /config` exposes
public Project configuration; `GET /launcher` supplies the first-party app switcher.
Authenticated operational routes are `GET /api/ai-gateway/v1/status`,
`GET /api/ai-gateway/v1/providers`,
`POST /api/ai-gateway/v1/providers/{name}/check`, and
`GET /api/ai-gateway/v1/invocations`. These reads never record inference traffic.
Provider checks share the five-minute probe/model cache; forced static-provider
checks are single-flight and reject repeats within 30 seconds with HTTP 429.
The invocation view retains 500 metadata-only entries since process start on
this instance, including audit delivery outcomes; it is not durable history.
See [the UI API contract](docs/ui-api.md) for response shapes and filters.

## Develop

Use Go 1.26 or newer and Bun 1.4.1. The app depends on the public
[`github.com/appthrust/appserver-go`](https://github.com/appthrust/appserver-go)
SDK at `v0.3.0`, including `aipolicy`. No platform checkout or local module
replacement is required. The SPA build requires GitHub Packages access through
`NODE_AUTH_TOKEN` (a token with package-read access).

```sh
go build ./... && go test ./...
bun test hack/
(cd web && bun install --frozen-lockfile && bun run build:image)
docker build --platform linux/amd64 --secret id=NODE_AUTH_TOKEN,env=NODE_AUTH_TOKEN -t ai-gateway-local .
```

Tests use local HTTP fixtures and mocked catalog responses; they need no live
providers or Cloud credentials. The binary accepts `--listen` (default `:8080`)
and `--static-dir` (default `AI_GATEWAY_STATIC_DIR` or `/srv/ai-gateway`).
Use `--static-dir web/dist` for a local production SPA build; a missing asset
directory logs once and leaves the API available, with HTTP 404 for `/`.
`AI_GATEWAY_PUBLIC_URL` supplies the public origin. `PLATFORM_DASHBOARD_URL` is
optional; no Dashboard origin is guessed. The internal endpoint is shown only
when `AI_GATEWAY_URL` or the own Service's `K_SERVICE` and `POD_NAMESPACE`
(or `NAMESPACE`) are injected.
A deployed instance receives Project identity, discovery credentials, and AI
policy configuration from AppThrust. Optional provider credential slots supply
`AI_GATEWAY_OPENAI_API_KEY` and `AI_GATEWAY_ANTHROPIC_API_KEY`.
`AI_GATEWAY_DEFAULT_PROVIDER` and `AI_GATEWAY_DEFAULT_MODEL` select defaults in
unrestricted mode. An explicitly configured malformed policy fails startup;
it never falls back to unrestricted mode.

The Dockerfile builds the SPA with Bun and cross-compiles a static Go binary in
build-host stages, then copies both into `gcr.io/distroless/static:nonroot`.
No JavaScript runtime is needed. It runs `/ai-gateway-server` as UID 65532 on
port 8080. There is no target-architecture `RUN`, so a single Buildx builder can
produce both Linux amd64 and arm64 without QEMU.

## Release

The first app-owned release is **`0.3.0`**, tagged **`v0.3.0`**. Existing catalog
versions `0.1.0` and `0.2.0` are immutable platform-image compatibility floors;
neither version may be reused for the new image repository.

```sh
git tag v0.3.0
git push origin v0.3.0
```

`.github/workflows/publish.yml` runs on `ubuntu-24.04`, builds and pushes
`ghcr.io/appthrust/ai-gateway:<version>` with a single
`docker buildx build --platform linux/amd64,linux/arm64`, resolves the image
index digest, and signs it with Sigstore keyless signing. Actions are pinned
to commit SHAs. The workflow requires `packages: write` and `id-token: write`
and serializes publishers with concurrency group `ai-gateway-catalog-publish`.
Only the `appthrust/ai-gateway` repository can publish.

Configure repository secret `CLOUD_CATALOG_ADMIN_TOKEN`, variable
`CLOUD_API_URL` (default `https://api.appthrust.com`), and access to organization
secrets `APPTHRUST_GITHUB_ACTIONS_APP_ID` and
`APPTHRUST_GITHUB_ACTIONS_APP_PRIVATE_KEY`. The GitHub App must be installed for
`appthrust/cloud` with Contents write permission for repository dispatches.
These app-repository prerequisites are provisioned. Cloud separately needs its
`CLOUD_CATALOG_API_URL` variable and `CLOUD_CATALOG_ADMIN_TOKEN` secret. Make
`ghcr.io/appthrust/ai-gateway` public after its first image push; source-repository
visibility does not determine package visibility.

Publication has two phases, also available manually with ORAS 1.3.0 and
Cosign 3.1.3 installed:

```sh
bun hack/publish-entry.ts submit --version 0.3.0 --digest sha256:<image-index-digest>
# Only if submit returns state=snapshot, use a Cloud installation token:
gh api -X POST repos/appthrust/cloud/dispatches -f event_type=catalog-publish -f 'client_payload[snapshot_id]=<snapshot-id>'
bun hack/publish-entry.ts await --version 0.3.0 --digest sha256:<image-index-digest>
```

`submit` merges the current Cloud version history with `catalog/entry.yaml`,
PUTs `/api/v1/catalog/entries/ai-gateway`, and POSTs `/api/v1/catalog/publish`.
It accepts HTTP 202 `{id, state: "queued"}` when Cloud dispatched publication,
or HTTP 200 `{id, state: "snapshot"}` when dispatch is needed. It emits
`snapshot_id` and `state` to `$GITHUB_OUTPUT` without polling. For `snapshot`
only, the workflow mints a short-lived GitHub App token scoped to Cloud and
sends the `catalog-publish` repository event with the snapshot id.

`await` needs no Cloud admin token. It checks the immutable `index.json` layer
of `ghcr.io/appthrust/catalog:latest` every 15 seconds for at most 10 minutes.
Success requires the exact AI Gateway version and image digest, a verified
layer digest, and the catalog's signature from issuer
`https://token.actions.githubusercontent.com` and identity
`https://github.com/appthrust/cloud/.github/workflows/catalog-publish.yml@refs/heads/main`.
Dispatch alone is not publication proof. API, dispatch, signature, and timeout
failures fail the release.

## Catalog and migration

`catalog/entry.yaml` preserves the complete definition and both version floors
from the ink `ApplicationStackDefinition/ai-gateway`. The committed template
contains no placeholder digest for an unpublished release. The publisher adds
`versions[].name: 0.3.0`, `stream: "0.3"`, and
`images.web: ghcr.io/appthrust/ai-gateway@sha256:<built-index-digest>` on the first
tag. This new version becomes the only default; earlier versions remain served.
Subsequent tags merge persisted history rather than discarding earlier app
releases. Repeating a version with the same digest is idempotent; changing an
existing version's image is rejected.

Moving an installation from `ghcr.io/appthrust/platform/ai-gateway` to
`ghcr.io/appthrust/ai-gateway` requires an explicit version update with
`replaceRepository: true`. Afterwards `follow` tracks the selected stream;
`pinned` requires explicit updates. `/releasez` reports catalog-rendered
`AI_GATEWAY_RELEASE_VERSION` and `AI_GATEWAY_ARTIFACT_DIGEST`; no platform
release closure is involved.

Original server: platform `cmd/ai-gateway-server/`. Architecture and migration
authority: [first-party app repositories](https://github.com/appthrust/platform/blob/main/design/platform/marketplace-apps/first-party-app-repositories.md),
section **App repository**.
