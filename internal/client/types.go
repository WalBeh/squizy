package client

// chatRequest is the JSON body sent to /v1/chat/completions.
type chatRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Stream      bool        `json:"stream"`
	StreamOpts  *streamOpts `json:"stream_options,omitempty"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature *float64    `json:"temperature,omitempty"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chunk is one SSE `data:` payload from a streaming completion.
type chunk struct {
	ID      string        `json:"id"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage"`
}

type chunkChoice struct {
	Delta        delta   `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// delta carries incremental content. The thinking stream is exposed under
// different field names across servers: omlx/DeepSeek use reasoning_content,
// vLLM uses reasoning. content is always the answer.
type delta struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
}

// reasoning returns the thinking delta regardless of which field the server used.
func (d delta) reasoning() string {
	if d.ReasoningContent != "" {
		return d.ReasoningContent
	}
	return d.Reasoning
}

// Usage is the server-reported token accounting (final chunk).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Model is one entry from /v1/models.
type Model struct {
	ID          string `json:"id"`
	OwnedBy     string `json:"owned_by"`
	MaxModelLen *int   `json:"max_model_len"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}
