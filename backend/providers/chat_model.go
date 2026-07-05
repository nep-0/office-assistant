package providers

import (
	"net/http"
	"strings"

	"office-assistant/backend/domain"

	genaiopenai "github.com/achetronic/adk-utils-go/genai/openai"
	"google.golang.org/adk/v2/model"
)

func ChatModel(setting domain.ProviderSetting, fakeProviders bool, client *http.Client, retrievalToolName string, maxRetrievalLimit int) (model.LLM, error) {
	if fakeProviders && strings.Contains(setting.BaseURL, "/fake-openai") {
		return NewFakeADKModel(setting.Model, retrievalToolName, maxRetrievalLimit), nil
	}
	return genaiopenai.New(genaiopenai.Config{
		APIKey:    setting.APIKey,
		BaseURL:   setting.BaseURL,
		ModelName: setting.Model,
		HTTPOptions: genaiopenai.HTTPOptions{
			Client: client,
		},
	}), nil
}
