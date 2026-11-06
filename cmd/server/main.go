package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/appthrust/appserver-go/aigatewaydiscovery"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/platformclient"
	"github.com/appthrust/appserver-go/appserver/platformenv"
)

func policyFromEnv() (aipolicy.Document, error) {
	mode, modeSet := os.LookupEnv(aipolicy.EnvMode)
	document, documentSet := os.LookupEnv(aipolicy.EnvDocument)
	if !modeSet && !documentSet {
		return aipolicy.Decode(string(aipolicy.ModeUnrestricted), "")
	}
	// An explicitly wired but broken policy connection must never become an
	// unrestricted gateway. This is stricter than Decode's shell-only mode.
	if !json.Valid([]byte(document)) {
		return aipolicy.Document{}, errors.New("invalid AI policy document")
	}
	return aipolicy.Decode(mode, document)
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("usage: ai-gateway-server [--listen :8080]")
	}
	platform, err := platformenv.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := platformenv.ValidateIdentityListen(*listen, platform.OIDCIssuer); err != nil {
		log.Fatal(err)
	}
	auth, err := identity.New(gatewayTrust())
	if err != nil {
		log.Fatal(err)
	}
	document, err := policyFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	client := platformclient.NewGatewayDiscovery(platform.APIURL, platform.ProjectID, os.Getenv(aigatewaydiscovery.KeyEnv), platformclient.WithHTTPClient(auth.HTTPClient()))
	resolver := link.NewResolver(client, "ai-gateway", nil, nil)
	gateway := &gateway{
		auth:   gatewayIdentity{auth: auth, platform: platformclient.New(platform.APIURL, platform.ProjectID, platformclient.WithHTTPClient(auth.HTTPClient())), project: platform.ProjectID},
		policy: document, providers: registryFromEnv(resolver),
		defaultProvider: os.Getenv("AI_GATEWAY_DEFAULT_PROVIDER"), defaultModel: os.Getenv("AI_GATEWAY_DEFAULT_MODEL"),
		// Reporting outlives the signal context so HTTP shutdown can finish enqueueing.
		recorder: newRecorder(context.Background(), platform.APIURL, platform.ProjectID, auth.HTTPClient().Transport),
	}
	server := &http.Server{Addr: *listen, Handler: gateway.handler(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	// No WriteTimeout or whole-request middleware: upstream inference headers and
	// read-idle each allow 300 seconds; model discovery headers allow 30 seconds.
	// Upstream connect/TLS deadlines remain 10 seconds.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("HTTP shutdown: %v", err)
			_ = server.Close()
		}
		if err := gateway.recorder.Close(shutdown); err != nil {
			log.Printf("AI invocation reporting shutdown: %v", err)
		}
	}()
	log.Printf("ai-gateway-server listening on %s (project %s)", *listen, platform.ProjectID)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone
}
