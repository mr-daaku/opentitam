package models

import (
	"cmp"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mr-daaku/opentitam/internal/logging"
)

const (
	// ProviderAPI is a remote OpenAI-compatible API such as build.nvidia.com.
	// It is configured entirely through the environment:
	//
	//	OPENTITAM_API_KEY     required, the API key
	//	OPENTITAM_API_BASE_URL optional, defaults to DefaultAPIBaseURL
	//	OPENTITAM_API_MODEL   optional, defaults to the first model the
	//	                      endpoint advertises on GET /models
	ProviderAPI ModelProvider = "api"

	// DefaultAPIBaseURL is NVIDIA's OpenAI-compatible endpoint, the same
	// one used by build.nvidia.com integrations.
	DefaultAPIBaseURL = "https://integrate.api.nvidia.com/v1"

	defaultAPIContextWindow  = int64(32768)
	defaultAPIResponseTokens = int64(4096)
)

// APIBaseURL returns the base URL of the remote OpenAI-compatible API.
func APIBaseURL() string {
	return cmp.Or(strings.TrimSpace(os.Getenv("OPENTITAM_API_BASE_URL")), DefaultAPIBaseURL)
}

// APIKey returns the API key for the remote OpenAI-compatible API, if set.
func APIKey() string {
	return strings.TrimSpace(os.Getenv("OPENTITAM_API_KEY"))
}

// APIModel returns the configured model for the remote API, if set.
func APIModel() string {
	return strings.TrimSpace(os.Getenv("OPENTITAM_API_MODEL"))
}

func init() {
	apiKey := APIKey()
	if apiKey == "" {
		return
	}

	apiModel := APIModel()
	if apiModel == "" {
		// Fall back to the first model the endpoint advertises, the same
		// way local models are picked when LOCAL_ENDPOINT is set.
		apiModel = firstAPIModel(APIBaseURL(), apiKey)
	}
	if apiModel == "" {
		logging.Debug("OPENTITAM_API_KEY is set but no model could be resolved",
			"endpoint", APIBaseURL(),
		)
		return
	}

	id := ModelID("api." + apiModel)
	SupportedModels[id] = Model{
		ID:                  id,
		Name:                friendlyModelName(apiModel),
		Provider:            ProviderAPI,
		APIModel:            apiModel,
		ContextWindow:       defaultAPIContextWindow,
		DefaultMaxTokens:    defaultAPIResponseTokens,
		SupportsAttachments: true,
	}
	logging.Debug("Registered remote API model", "model", id, "endpoint", APIBaseURL())
}

// firstAPIModel asks the endpoint for its model list and returns the first
// model ID it advertises, or "" when the list cannot be retrieved.
func firstAPIModel(baseURL, apiKey string) string {
	endpoint := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		logging.Debug("Failed to list API models", "error", err, "endpoint", endpoint)
		return ""
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		logging.Debug("Failed to list API models",
			"status", res.StatusCode,
			"endpoint", endpoint,
		)
		return ""
	}

	var list localModelList
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		logging.Debug("Failed to decode API model list", "error", err)
		return ""
	}

	for _, m := range list.Data {
		if m.ID != "" {
			return m.ID
		}
	}
	return ""
}
