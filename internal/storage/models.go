package storage

type ProviderSettings struct {
	Embedding ProviderSlot `json:"embedding"`
	Chat      ProviderSlot `json:"chat"`
}

type ProviderSlot struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
}

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
	CreatedAt    string `json:"created_at"`
}

type KnowledgeBase struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

type Document struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	UploadedBy      string `json:"uploaded_by"`
	OriginalName    string `json:"original_name"`
	StoragePath     string `json:"-"`
	ContentType     string `json:"content_type"`
	SizeBytes       int64  `json:"size_bytes"`
	SHA256          string `json:"sha256"`
	Status          string `json:"status"`
	StatusReason    string `json:"status_reason,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type Chunk struct {
	ID               string `json:"id"`
	DocumentID       string `json:"document_id"`
	KnowledgeBaseID  string `json:"knowledge_base_id"`
	Content          string `json:"content"`
	SourceFileName   string `json:"source_file_name"`
	PageNumber       *int   `json:"page_number,omitempty"`
	ChunkIndex       int    `json:"chunk_index"`
	ContentType      string `json:"content_type"`
	TokenOrCharCount int    `json:"token_or_char_count"`
	MetadataJSON     string `json:"metadata_json,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type CitationRecord struct {
	ID             string
	MessageID      string
	DocumentID     string
	ChunkID        string
	SourceFileName string
	PageNumber     *int
	Preview        string
	Score          *float64
}
