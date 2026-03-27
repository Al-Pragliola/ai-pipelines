package issuehistory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store tracks completed pipeline runs so that issues are not re-processed
// after their PipelineRun CRs are deleted.
type Store struct {
	db *sql.DB
}

// Record represents a completed issue entry.
type Record struct {
	PipelineNamespace string    `json:"pipelineNamespace"`
	PipelineName      string    `json:"pipelineName"`
	IssueKey          string    `json:"issueKey"`
	Phase             string    `json:"phase"`
	RunName           string    `json:"runName"`
	CompletedAt       time.Time `json:"completedAt"`
}

// New opens (or creates) a SQLite database at the given path and runs migrations.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening history database: %w", err)
	}
	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running history migrations: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS completed_issues (
			pipeline_namespace TEXT NOT NULL,
			pipeline_name      TEXT NOT NULL,
			issue_key          TEXT NOT NULL,
			phase              TEXT NOT NULL,
			run_name           TEXT NOT NULL,
			completed_at       DATETIME NOT NULL,
			PRIMARY KEY (pipeline_namespace, pipeline_name, issue_key)
		)
	`)
	return err
}

// IsCompleted checks whether an issue has already been processed to a terminal state.
func (s *Store) IsCompleted(ctx context.Context, pipelineNamespace, pipelineName, issueKey string) (bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM completed_issues
		 WHERE pipeline_namespace = ? AND pipeline_name = ? AND issue_key = ?`,
		pipelineNamespace, pipelineName, issueKey,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// MarkCompleted records that an issue has been processed.
// Uses INSERT OR REPLACE so re-completing an issue updates the record.
func (s *Store) MarkCompleted(ctx context.Context, rec Record) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO completed_issues
		 (pipeline_namespace, pipeline_name, issue_key, phase, run_name, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rec.PipelineNamespace, rec.PipelineName, rec.IssueKey,
		rec.Phase, rec.RunName, rec.CompletedAt,
	)
	return err
}

// List returns all completion records, ordered by completion time descending.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT pipeline_namespace, pipeline_name, issue_key, phase, run_name, completed_at
		 FROM completed_issues ORDER BY completed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.PipelineNamespace, &r.PipelineName, &r.IssueKey,
			&r.Phase, &r.RunName, &r.CompletedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// Delete removes a completion record, allowing the issue to be re-processed.
func (s *Store) Delete(ctx context.Context, pipelineNamespace, pipelineName, issueKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM completed_issues
		 WHERE pipeline_namespace = ? AND pipeline_name = ? AND issue_key = ?`,
		pipelineNamespace, pipelineName, issueKey,
	)
	return err
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
