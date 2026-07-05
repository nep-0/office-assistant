package chat

import (
	"encoding/json"
	"fmt"

	"office-assistant/backend/domain"
)

type RetrievalToolArgs struct {
	Query string `json:"query" jsonschema:"Focused search query proposed by the model."`
	Limit int    `json:"limit,omitempty" jsonschema:"Optional desired result count. The backend enforces a maximum."`
}

type RetrievalToolResult struct {
	Results []RetrievalEvidence `json:"results"`
}

type RetrievalEvidence struct {
	CitationID   string         `json:"citation_id"`
	DocumentID   int64          `json:"document_id"`
	DocumentName string         `json:"document_name"`
	ChunkID      int64          `json:"chunk_id"`
	HeadingPath  string         `json:"heading_path,omitempty"`
	SourceAnchor map[string]any `json:"source_anchor,omitempty"`
	Text         string         `json:"text"`
}

func FindPersistedCitation(messages []domain.ChatMessage, citationID string) (RetrievalEvidence, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		var metadata struct {
			Citations []RetrievalEvidence `json:"citations"`
		}
		if err := json.Unmarshal([]byte(messages[i].Metadata), &metadata); err != nil {
			continue
		}
		for _, citation := range metadata.Citations {
			if citation.CitationID == citationID {
				return citation, true
			}
		}
	}
	return RetrievalEvidence{}, false
}

func EvidenceFromChunk(index int, chunk domain.RetrievalChunk) RetrievalEvidence {
	var anchor map[string]any
	_ = json.Unmarshal([]byte(chunk.SourceAnchorJSON), &anchor)
	return RetrievalEvidence{
		CitationID:   fmt.Sprintf("c%d", index+1),
		DocumentID:   chunk.DocumentID,
		DocumentName: chunk.DocumentName,
		ChunkID:      chunk.ChunkID,
		HeadingPath:  chunk.HeadingPath,
		SourceAnchor: anchor,
		Text:         chunk.Content,
	}
}
