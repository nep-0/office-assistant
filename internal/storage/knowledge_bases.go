package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CreateKnowledgeBase(ctx context.Context, kb KnowledgeBase) (KnowledgeBase, error) {
	if strings.TrimSpace(kb.Name) == "" {
		return KnowledgeBase{}, errors.New("name is required")
	}
	if kb.CreatedAt == "" {
		kb.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `insert into knowledge_bases (id, name, created_by, created_at) values (?, ?, ?, ?)`,
		kb.ID, kb.Name, kb.CreatedBy, kb.CreatedAt)
	if err != nil {
		return KnowledgeBase{}, fmt.Errorf("create knowledge base: %w", err)
	}
	return kb, nil
}

func (s *Store) AddKnowledgeBaseMember(ctx context.Context, knowledgeBaseID, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `insert or ignore into knowledge_base_memberships (knowledge_base_id, user_id, created_at) values (?, ?, ?)`,
		knowledgeBaseID, userID, now)
	if err != nil {
		return fmt.Errorf("add knowledge base member: %w", err)
	}
	return nil
}

func (s *Store) CanAccessKnowledgeBase(ctx context.Context, user User, knowledgeBaseID string) (bool, error) {
	if user.Role == "admin" {
		return true, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `select count(*) from knowledge_base_memberships where knowledge_base_id = ? and user_id = ?`,
		knowledgeBaseID, user.ID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check knowledge base access: %w", err)
	}
	return count > 0, nil
}

func (s *Store) ListKnowledgeBases(ctx context.Context, user User) ([]KnowledgeBase, error) {
	query := `select id, name, created_by, created_at from knowledge_bases order by created_at asc`
	args := []any{}
	if user.Role != "admin" {
		query = `select kb.id, kb.name, kb.created_by, kb.created_at
			from knowledge_bases kb
			join knowledge_base_memberships m on m.knowledge_base_id = kb.id
			where m.user_id = ?
			order by kb.created_at asc`
		args = append(args, user.ID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	defer rows.Close()

	kbs := []KnowledgeBase{}
	for rows.Next() {
		var kb KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.CreatedBy, &kb.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan knowledge base: %w", err)
		}
		kbs = append(kbs, kb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge bases: %w", err)
	}
	return kbs, nil
}
