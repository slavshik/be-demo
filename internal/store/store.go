// Package store keeps players and scores in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Sentinel errors that handlers map to HTTP status codes.
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

// Player is a registered player.
type Player struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// Score is one stored score submission.
type Score struct {
	Username  string    `json:"username"`
	Score     int64     `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// Entry is one leaderboard row: a player and their best score.
type Entry struct {
	Username string `json:"username"`
	Score    int64  `json:"score"`
}

// Store is a SQLite-backed store for players and scores.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the database at path and applies the schema.
func Open(path string) (*Store, error) {
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// One connection serializes writes, so SQLite never reports SQLITE_BUSY.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreatePlayer registers a player. It returns ErrUsernameTaken if the
// username exists, compared case-insensitively.
func (s *Store) CreatePlayer(ctx context.Context, username string) (Player, error) {
	p := Player{Username: username, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO players (username, created_at) VALUES (?, ?)`,
		p.Username, formatTime(p.CreatedAt))
	if err != nil {
		// Rely on the UNIQUE constraint rather than checking first,
		// so concurrent registrations can't both succeed.
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return Player{}, ErrUsernameTaken
		}
		return Player{}, fmt.Errorf("insert player: %w", err)
	}
	return p, nil
}

// AddScore stores a score for an existing player. It returns
// ErrPlayerNotFound if no player has that username.
func (s *Store) AddScore(ctx context.Context, username string, score int64) (Score, error) {
	sc := Score{Username: username, Score: score, CreatedAt: now()}
	// The player lookup and the insert are one statement, so there is no
	// window where the player could be checked but not yet written against.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO scores (player_id, score, created_at)
		 SELECT id, ?, ? FROM players WHERE username = ?`,
		sc.Score, formatTime(sc.CreatedAt), sc.Username)
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

// Leaderboard returns up to limit players ranked by their best score,
// highest first, ties broken by username. Players without scores are
// left out. The result is never nil.
func (s *Store) Leaderboard(ctx context.Context, limit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.username, MAX(s.score) AS best
		 FROM players p JOIN scores s ON s.player_id = p.id
		 GROUP BY p.id
		 ORDER BY best DESC, p.username ASC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query leaderboard: %w", err)
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Username, &e.Score); err != nil {
			return nil, fmt.Errorf("scan leaderboard: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read leaderboard: %w", err)
	}
	return entries, nil
}

func now() time.Time {
	return time.Now().UTC()
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}
