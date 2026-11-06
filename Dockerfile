# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM docker.io/golang:1.27.3 AS server-builder
WORKDIR /workspace
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY cmd/server ./cmd/server
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/ai-gateway-server ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=server-builder /out/ai-gateway-server /ai-gateway-server
USER 65532
EXPOSE 8080
ENTRYPOINT ["/ai-gateway-server"]
