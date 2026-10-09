// Package tasks defines the domain without depending on HTTP or SQL.
package tasks

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrNotFound = errors.New("task not found")
var ErrInvalidTitle = errors.New("invalid task title")

type Task struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// NormalizeTitle measures Unicode code points, not bytes or grapheme clusters.
func NormalizeTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 200 {
		return "", ErrInvalidTitle
	}
	return title, nil
}
