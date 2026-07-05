package store

import "context"

func (s *Store) CreateKnowledgeBase(ctx context.Context, ownerID int64, name, visibility string) (KnowledgeBase, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO knowledge_bases (owner_user_id, name, visibility)
VALUES (?, ?, ?)
`, ownerID, name, visibility)
	if err != nil {
		return KnowledgeBase{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return KnowledgeBase{}, err
	}
	return s.FindKnowledgeBaseByID(ctx, id)
}

func (s *Store) ListKnowledgeBasesForUser(ctx context.Context, current User) ([]KnowledgeBase, error) {
	query := `
SELECT knowledge_bases.id, knowledge_bases.owner_user_id, users.username, knowledge_bases.name, knowledge_bases.visibility, knowledge_bases.created_at, knowledge_bases.updated_at
FROM knowledge_bases
JOIN users ON users.id = knowledge_bases.owner_user_id
WHERE knowledge_bases.deleted_at IS NULL
`
	args := []any{}
	if current.Role != roleAdmin {
		query += ` AND (knowledge_bases.owner_user_id = ? OR knowledge_bases.visibility = 'public')`
		args = append(args, current.ID)
	}
	query += ` ORDER BY knowledge_bases.updated_at DESC, knowledge_bases.id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bases []KnowledgeBase
	for rows.Next() {
		kb, err := scanKnowledgeBase(rows)
		if err != nil {
			return nil, err
		}
		bases = append(bases, kb)
	}
	return bases, rows.Err()
}

func (s *Store) FindKnowledgeBaseByID(ctx context.Context, id int64) (KnowledgeBase, error) {
	return scanKnowledgeBase(s.db.QueryRowContext(ctx, `
SELECT knowledge_bases.id, knowledge_bases.owner_user_id, users.username, knowledge_bases.name, knowledge_bases.visibility, knowledge_bases.created_at, knowledge_bases.updated_at
FROM knowledge_bases
JOIN users ON users.id = knowledge_bases.owner_user_id
WHERE knowledge_bases.id = ? AND knowledge_bases.deleted_at IS NULL
`, id))
}

func (s *Store) UpdateKnowledgeBase(ctx context.Context, id int64, name, visibility string) (KnowledgeBase, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_bases
SET name = ?, visibility = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND deleted_at IS NULL
`, name, visibility, id)
	if err != nil {
		return KnowledgeBase{}, err
	}
	return s.FindKnowledgeBaseByID(ctx, id)
}

func (s *Store) DeleteKnowledgeBase(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE knowledge_bases
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND deleted_at IS NULL
`, id)
	return err
}
