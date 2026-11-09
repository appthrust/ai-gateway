package main

import (
	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
)

// These pure decisions are shared by inference and the UI's omitted-model preview.
func decideEnforced(document aipolicy.Document, record *aipolicy.InvocationRecord) {
	if record.Model == "auto" {
		record.Model = ""
	}
	if record.Model == "" {
		if record.Provider == "" {
			record.Provider, record.Model = document.DefaultModel()
		} else {
			record.Model = document.DefaultModelFor(record.Provider)
		}
		return
	}
	if record.Provider == "" {
		for _, candidate := range document.Providers {
			if document.Allows(candidate.Name, record.Model).Allowed {
				record.Provider = candidate.Name
				return
			}
		}
	}
}

func decideProvider(mode aipolicy.Mode, defaultProvider, defaultModel string, candidates []provider.Provider, record *aipolicy.InvocationRecord) provider.Provider {
	if mode == aipolicy.ModeUnrestricted && record.Provider == "" {
		record.Provider = defaultProvider
	}
	var selected provider.Provider
	for _, candidate := range candidates {
		if record.Provider == "" || candidate.Name() == record.Provider {
			selected = candidate
			record.Provider = selected.Name()
			break
		}
	}
	if selected != nil && mode == aipolicy.ModeUnrestricted && record.Model == "" {
		record.Model = defaultModel
		if record.Model == "" {
			record.Model = selected.DefaultModel()
		}
	}
	return selected
}
