package utils

import (
	"database/sql"
	"errors"
)

func NotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
