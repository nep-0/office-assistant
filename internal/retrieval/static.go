package retrieval

import "context"

type StaticTool struct{}

func (StaticTool) Retrieve(ctx context.Context, input Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{
		Chunks: []Chunk{
			{
				DocumentID:     "doc_spike",
				ChunkID:        "chunk_spike_001",
				SourceFileName: "spike-source.pdf",
				PageNumber:     1,
				Content:        "The office assistant indexes local documents, answers questions from retrieved chunks, and returns citations for every factual answer.",
				Preview:        "The office assistant indexes local documents, answers questions from retrieved chunks, and returns citations...",
				Score:          1,
			},
		},
	}, nil
}
