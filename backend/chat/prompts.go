package chat

import (
	"office-assistant/backend/domain"
)

func KnowledgeBaseInstruction(kb domain.KnowledgeBase) string {
	return "You answer questions for the selected Knowledge Base named " + kb.Name + ". Before any final answer, call retrieve_knowledge with a focused query. Use only retrieved evidence. If retrieval has no relevant results, say the documents do not contain enough information."
}

func UnsupportedAnswerMessage() string {
	return "The selected Knowledge Base does not contain enough evidence to answer that question."
}
