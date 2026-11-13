package models

type (
	ModelID       string
	ModelProvider string
)

type Model struct {
	ID                  ModelID       `json:"id"`
	Name                string        `json:"name"`
	Provider            ModelProvider `json:"provider"`
	APIModel            string        `json:"api_model"`
	CostPer1MIn         float64       `json:"cost_per_1m_in"`
	CostPer1MOut        float64       `json:"cost_per_1m_out"`
	CostPer1MInCached   float64       `json:"cost_per_1m_in_cached"`
	CostPer1MOutCached  float64       `json:"cost_per_1m_out_cached"`
	ContextWindow       int64         `json:"context_window"`
	DefaultMaxTokens    int64         `json:"default_max_tokens"`
	CanReason           bool          `json:"can_reason"`
	SupportsAttachments bool          `json:"supports_attachments"`
}

const (
	// ForTests
	ProviderMock ModelProvider = "__mock"
)

// ProviderPopularity orders providers for display; only local models are
// supported in this build.
var ProviderPopularity = map[ModelProvider]int{
	ProviderLocal: 0,
}

// SupportedModels is populated at init time from the local endpoint. See
// local.go for how models are discovered.
var SupportedModels = map[ModelID]Model{}
