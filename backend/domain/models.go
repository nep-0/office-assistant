package domain

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type ProviderSetting struct {
	Purpose   string
	BaseURL   string
	Model     string
	APIKey    string
	UpdatedAt time.Time
}

type KnowledgeBase struct {
	ID         int64
	OwnerID    int64
	OwnerName  string
	Name       string
	Visibility string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DocumentRecord struct {
	ID               int64
	KnowledgeBaseID  int64
	OwnerID          int64
	OriginalFilename string
	DisplayName      string
	ContentType      string
	SizeBytes        int64
	SHA256           string
	StorageKey       string
	Status           string
	ErrorCode        string
	ErrorMessage     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type IngestionJob struct {
	ID           int64
	DocumentID   int64
	Status       string
	Attempts     int
	MaxAttempts  int
	ErrorCode    string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DocumentVersion struct {
	ID                 int64
	DocumentID         int64
	VersionNo          int
	MarkdownStorageKey string
	SchemaVersion      string
	MetadataJSON       string
	IndexingStatus     string
	EmbeddingModel     string
	CreatedAt          time.Time
}

type DocumentChunk struct {
	ID                int64
	DocumentID        int64
	DocumentVersionID int64
	ChunkNo           int
	Content           string
	HeadingPath       string
	SourceAnchorJSON  string
	TokenCount        int
	EmbeddingModel    string
	IndexingStatus    string
	CreatedAt         time.Time
}

type ChatSession struct {
	ID              string
	UserID          int64
	KnowledgeBaseID int64
	Title           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ChatMessage struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Metadata  string
	CreatedAt time.Time
}

type RetrievalChunk struct {
	ChunkID          int64
	DocumentID       int64
	DocumentName     string
	Content          string
	HeadingPath      string
	SourceAnchorJSON string
}

type ActivityEvent struct {
	ID         int64
	UserID     int64
	EventType  string
	EntityType string
	EntityID   string
	Details    string
	CreatedAt  time.Time
}

type WorkflowMetric struct {
	ID        int64
	Name      string
	ValueMS   int64
	Count     int64
	Details   string
	CreatedAt time.Time
}

type DebugSetting struct {
	Enabled   bool
	Source    string
	UpdatedAt time.Time
}
