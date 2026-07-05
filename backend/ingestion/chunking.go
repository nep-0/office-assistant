package ingestion

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"office-assistant/backend/domain"
)

const DefaultMaxChunkRunes = 1800

type ChunkingStrategy interface {
	Chunk(markdown string, anchors []map[string]any) ([]domain.DocumentChunk, error)
}

type MarkdownChunkingStrategy struct {
	MaxRunes int
}

var markdownHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

func (s MarkdownChunkingStrategy) Chunk(markdown string, anchors []map[string]any) ([]domain.DocumentChunk, error) {
	maxRunes := s.MaxRunes
	if maxRunes <= 0 {
		maxRunes = DefaultMaxChunkRunes
	}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	var chunks []domain.DocumentChunk
	var headingStack []string
	var current strings.Builder
	currentHeading := ""

	flush := func() error {
		content := strings.TrimSpace(current.String())
		if content == "" {
			current.Reset()
			return nil
		}
		anchorJSON, err := chooseSourceAnchorJSON(anchors, len(chunks))
		if err != nil {
			return err
		}
		chunks = append(chunks, domain.DocumentChunk{
			ChunkNo:          len(chunks) + 1,
			Content:          content,
			HeadingPath:      currentHeading,
			SourceAnchorJSON: anchorJSON,
			TokenCount:       EstimatedTokenCount(content),
		})
		current.Reset()
		return nil
	}

	for _, line := range lines {
		if match := markdownHeadingPattern.FindStringSubmatch(line); match != nil {
			if err := flush(); err != nil {
				return nil, err
			}
			level := len(match[1])
			title := strings.TrimSpace(match[2])
			for len(headingStack) >= level {
				headingStack = headingStack[:len(headingStack)-1]
			}
			headingStack = append(headingStack, title)
			currentHeading = strings.Join(headingStack, " > ")
		}
		if current.Len() > 0 && current.Len()+len(line)+1 > maxRunes {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(chunks) == 0 && strings.TrimSpace(markdown) != "" {
		anchorJSON, err := chooseSourceAnchorJSON(anchors, 0)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, domain.DocumentChunk{
			ChunkNo:          1,
			Content:          strings.TrimSpace(markdown),
			SourceAnchorJSON: anchorJSON,
			TokenCount:       EstimatedTokenCount(markdown),
		})
	}
	return chunks, nil
}

func chooseSourceAnchorJSON(anchors []map[string]any, chunkIndex int) (string, error) {
	if len(anchors) == 0 {
		return "{}", nil
	}
	index := chunkIndex
	if index >= len(anchors) {
		index = len(anchors) - 1
	}
	data, err := json.Marshal(anchors[index])
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func EstimatedTokenCount(content string) int {
	words := len(strings.Fields(content))
	if words > 0 {
		return words
	}
	runes := utf8.RuneCountInString(content)
	if runes == 0 {
		return 0
	}
	return (runes + 3) / 4
}
