package secrets

import "g7.mph.tech/mph-tech/autodev/internal/app"

func NewDefaultProvider(env app.Env) Provider {
	return NewChain(
		KeychainProvider{Service: env.LocalKeychainSvc},
		GCPProvider{Project: env.GCPProject},
	)
}
