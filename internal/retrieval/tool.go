package retrieval

import "context"

type Tool interface {
	Retrieve(ctx context.Context, input Input) (Result, error)
}

type Input struct {
	KnowledgeBaseID string
	Query           string
	RecentMessages  []Message
	TopK            int
}

type Message struct {
	Role    string
	Content string
}

type Result struct {
	Chunks []Chunk
}

type Chunk struct {
	DocumentID     string
	ChunkID        string
	SourceFileName string
	PageNumber     int
	Content        string
	Preview        string
	Score          float64
}
