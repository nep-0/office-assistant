package app

import (
	"context"

	"office-assistant/backend/providers"

	chromem "github.com/philippgille/chromem-go"
)

func (a *app) embeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		setting, err := a.store.FindProviderSetting(ctx, providerPurposeEmbedding)
		if err != nil {
			return nil, err
		}
		return providers.OpenAICompatibleEmbedding(ctx, a.httpClient, setting, text, correlationID(ctx))
	}
}
