// Package store persists players and scores in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrUsernameTaken  = errors.New("username taken")
	ErrPlayerNotFound = errors.New("player not found")
)

const schema = `
CREATE TABLE IF NOT EXISTS players (
	id         INTEGER PRIMARY KEY,
	username   TEXT NOT NULL UNIQUE COLLATE NOCASE,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS scores (
	id         INTEGER PRIMARY KEY,
	player_id  INTEGER NOT NULL REFERENCES players(id),
	score      INTEGER NOT NULL CHECK (score >= 0),
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scores_player_score ON scores(player_id, score);
`

type Player struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type Score struct {
	Username  string    `json:"username"`
	Score     int64     `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

type Entry struct {
	Username string `json:"username"`
	Score    int64  `json:"score"`
}

type Store struct {
	db *sql.DB
}

// Open opens the database and applies the schema.
func Open(path string) (*Store, error) {
	// Escape filenames so special characters are not treated as URI syntax.
	dsn := url.URL{
		Scheme:   "file",
		Opaque:   (&url.URL{Path: path}).EscapedPath(),
		RawQuery: "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
	}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// Serialize local queries; busy_timeout handles external locks.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// CreatePlayer returns ErrUsernameTaken for case-insensitive duplicates.
func (s *Store) CreatePlayer(ctx context.Context, username string) (Player, error) {
	p := Player{Username: username, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO players (username, created_at) VALUES (?, ?)`,
		p.Username, p.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return Player{}, ErrUsernameTaken
		}
		return Player{}, fmt.Errorf("insert player: %w", err)
	}
	return p, nil
}

// AddScore returns ErrPlayerNotFound for unknown players.
func (s *Store) AddScore(ctx context.Context, username string, score int64) (Score, error) {
	sc := Score{Username: username, Score: score, CreatedAt: time.Now().UTC()}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO scores (player_id, score, created_at)
		 SELECT id, ?, ? FROM players WHERE username = ?`,
		sc.Score, sc.CreatedAt.Format(time.RFC3339Nano), sc.Username)
	if err != nil {
		return Score{}, fmt.Errorf("insert score: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Score{}, fmt.Errorf("insert score: %w", err)
	}
	if n == 0 {
		return Score{}, ErrPlayerNotFound
	}
	return sc, nil
}

// Leaderboard returns best scores and the ranked player count. Entries are never nil.
func (s *Store) Leaderboard(ctx context.Context, limit, offset int) ([]Entry, int, error) {
	// Keep the page and total in the same snapshot.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, fmt.Errorf("begin leaderboard: %w", err)
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT player_id) FROM scores`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count leaderboard: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT p.username, MAX(s.score) AS best
		 FROM players p JOIN scores s ON s.player_id = p.id
		 GROUP BY p.id
		 ORDER BY best DESC, p.username ASC
		 LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query leaderboard: %w", err)
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Username, &e.Score); err != nil {
			return nil, 0, fmt.Errorf("scan leaderboard: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read leaderboard: %w", err)
	}
	return entries, total, nil
}
