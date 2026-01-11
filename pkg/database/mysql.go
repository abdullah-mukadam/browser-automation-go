package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"dev/bravebird/browser-automation-go/pkg/models"

	_ "github.com/go-sql-driver/mysql"
)

// DB represents the database connection
type DB struct {
	conn *sql.DB
}

// New creates a new database connection
func New(dsn string) (*DB, error) {
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// ==================== Workflow Definitions ====================

// CreateWorkflowDefinition creates a new workflow definition
func (db *DB) CreateWorkflowDefinition(ctx context.Context, def *models.WorkflowDefinition) error {
	query := `
		INSERT INTO workflow_definitions (id, name, events_file_path, start_url, semantic_context, parameters, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	def.CreatedAt = now
	def.UpdatedAt = now

	_, err := db.conn.ExecContext(ctx, query,
		def.ID,
		def.Name,
		def.EventsFilePath,
		def.StartURL,
		def.SemanticContext,
		def.ParametersJSON,
		def.CreatedAt,
		def.UpdatedAt,
	)

	return err
}

// GetWorkflowDefinition retrieves a workflow definition by ID
func (db *DB) GetWorkflowDefinition(ctx context.Context, id string) (*models.WorkflowDefinition, error) {
	query := `
		SELECT id, name, events_file_path, is_workflow_generated, start_url, 
		       semantic_context, parameters, created_at, updated_at
		FROM workflow_definitions
		WHERE id = ?
	`

	var def models.WorkflowDefinition
	err := db.conn.QueryRowContext(ctx, query, id).Scan(
		&def.ID,
		&def.Name,
		&def.EventsFilePath,
		&def.IsWorkflowGenerated,
		&def.StartURL,
		&def.SemanticContext,
		&def.ParametersJSON,
		&def.CreatedAt,
		&def.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	return &def, nil
}

// ListWorkflowDefinitions retrieves all workflow definitions
func (db *DB) ListWorkflowDefinitions(ctx context.Context) ([]models.WorkflowDefinition, error) {
	query := `
		SELECT id, name, events_file_path, is_workflow_generated, start_url,
		       semantic_context, parameters, created_at, updated_at
		FROM workflow_definitions
		ORDER BY created_at DESC
	`

	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	defer rows.Close()

	var definitions []models.WorkflowDefinition
	for rows.Next() {
		var def models.WorkflowDefinition
		err := rows.Scan(
			&def.ID,
			&def.Name,
			&def.EventsFilePath,
			&def.IsWorkflowGenerated,
			&def.StartURL,
			&def.SemanticContext,
			&def.ParametersJSON,
			&def.CreatedAt,
			&def.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workflow: %w", err)
		}
		definitions = append(definitions, def)
	}

	return definitions, nil
}

// UpdateWorkflowDefinition updates a workflow definition
func (db *DB) UpdateWorkflowDefinition(ctx context.Context, def *models.WorkflowDefinition) error {
	query := `
		UPDATE workflow_definitions
		SET name = ?, is_workflow_generated = ?, semantic_context = ?, 
		    parameters = ?, updated_at = ?
		WHERE id = ?
	`

	def.UpdatedAt = time.Now()

	_, err := db.conn.ExecContext(ctx, query,
		def.Name,
		def.IsWorkflowGenerated,
		def.SemanticContext,
		def.ParametersJSON,
		def.UpdatedAt,
		def.ID,
	)

	return err
}

// DeleteWorkflowDefinition deletes a workflow definition
func (db *DB) DeleteWorkflowDefinition(ctx context.Context, id string) error {
	query := `DELETE FROM workflow_definitions WHERE id = ?`
	_, err := db.conn.ExecContext(ctx, query, id)
	return err
}

// ==================== Semantic Actions ====================

// CreateSemanticActions creates semantic actions for a workflow
func (db *DB) CreateSemanticActions(ctx context.Context, workflowID string, actions []models.SemanticAction) error {
	query := `
		INSERT INTO semantic_actions (id, workflow_id, sequence_id, action_type, target, value, embeddings, interaction_rank, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, action := range actions {
		targetJSON, _ := json.Marshal(action.Target)
		embeddingsJSON, _ := json.Marshal(action.Embeddings)

		_, err := stmt.ExecContext(ctx,
			action.ID,
			workflowID,
			action.SequenceID,
			action.ActionType,
			string(targetJSON),
			action.Value,
			embeddingsJSON,
			action.InteractionRank,
			action.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("failed to insert action: %w", err)
		}
	}

	return tx.Commit()
}

// GetSemanticActions retrieves all semantic actions for a workflow
func (db *DB) GetSemanticActions(ctx context.Context, workflowID string) ([]models.SemanticAction, error) {
	query := `
		SELECT id, workflow_id, sequence_id, action_type, target, value, embeddings, interaction_rank, timestamp
		FROM semantic_actions
		WHERE workflow_id = ?
		ORDER BY sequence_id
	`

	rows, err := db.conn.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get actions: %w", err)
	}
	defer rows.Close()

	var actions []models.SemanticAction
	for rows.Next() {
		var action models.SemanticAction
		var targetJSON, embeddingsJSON string

		err := rows.Scan(
			&action.ID,
			&action.WorkflowID,
			&action.SequenceID,
			&action.ActionType,
			&targetJSON,
			&action.Value,
			&embeddingsJSON,
			&action.InteractionRank,
			&action.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action: %w", err)
		}

		json.Unmarshal([]byte(targetJSON), &action.Target)
		json.Unmarshal([]byte(embeddingsJSON), &action.Embeddings)

		actions = append(actions, action)
	}

	return actions, nil
}

// ==================== Workflow Runs ====================

// CreateWorkflowRun creates a new workflow run
func (db *DB) CreateWorkflowRun(ctx context.Context, run *models.WorkflowRun) error {
	query := `
		INSERT INTO workflow_runs (id, workflow_id, temporal_run_id, temporal_workflow_id, status, parameters)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.ExecContext(ctx, query,
		run.ID,
		run.WorkflowID,
		run.TemporalRunID,
		run.TemporalWorkflowID,
		run.Status,
		run.ParametersJSON,
	)

	return err
}

// GetWorkflowRun retrieves a workflow run by ID
func (db *DB) GetWorkflowRun(ctx context.Context, id string) (*models.WorkflowRun, error) {
	query := `
		SELECT id, workflow_id, temporal_run_id, temporal_workflow_id, status,
		       parameters, started_at, completed_at, error_message
		FROM workflow_runs
		WHERE id = ?
	`

	var run models.WorkflowRun
	err := db.conn.QueryRowContext(ctx, query, id).Scan(
		&run.ID,
		&run.WorkflowID,
		&run.TemporalRunID,
		&run.TemporalWorkflowID,
		&run.Status,
		&run.ParametersJSON,
		&run.StartedAt,
		&run.CompletedAt,
		&run.ErrorMessage,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	return &run, nil
}

// ListWorkflowRuns retrieves runs for a workflow
func (db *DB) ListWorkflowRuns(ctx context.Context, workflowID string) ([]models.WorkflowRun, error) {
	query := `
		SELECT id, workflow_id, temporal_run_id, temporal_workflow_id, status,
		       parameters, started_at, completed_at, error_message
		FROM workflow_runs
		WHERE workflow_id = ?
		ORDER BY started_at DESC
	`

	rows, err := db.conn.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}
	defer rows.Close()

	var runs []models.WorkflowRun
	for rows.Next() {
		var run models.WorkflowRun
		err := rows.Scan(
			&run.ID,
			&run.WorkflowID,
			&run.TemporalRunID,
			&run.TemporalWorkflowID,
			&run.Status,
			&run.ParametersJSON,
			&run.StartedAt,
			&run.CompletedAt,
			&run.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// UpdateWorkflowRunStatus updates the status of a workflow run
func (db *DB) UpdateWorkflowRunStatus(ctx context.Context, id string, status models.RunStatus, errorMsg string) error {
	query := `
		UPDATE workflow_runs
		SET status = ?, error_message = ?, 
		    completed_at = CASE WHEN ? IN ('success', 'failed', 'canceled') THEN NOW() ELSE completed_at END
		WHERE id = ?
	`

	_, err := db.conn.ExecContext(ctx, query, status, errorMsg, status, id)
	return err
}

// ==================== Action Results ====================

// CreateActionResult creates an action result
func (db *DB) CreateActionResult(ctx context.Context, result *models.ActionResult) error {
	query := `
		INSERT INTO action_results (id, run_id, action_id, sequence_id, status, generated_code)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.ExecContext(ctx, query,
		result.ID,
		result.RunID,
		result.ActionID,
		result.SequenceID,
		result.Status,
		result.GeneratedCode,
	)

	return err
}

// UpdateActionResult updates an action result
func (db *DB) UpdateActionResult(ctx context.Context, result *models.ActionResult) error {
	query := `
		UPDATE action_results
		SET status = ?, retry_count = ?, screenshot_path = ?, 
		    error_message = ?, executed_at = ?, duration_ms = ?
		WHERE id = ?
	`

	_, err := db.conn.ExecContext(ctx, query,
		result.Status,
		result.RetryCount,
		result.ScreenshotPath,
		result.ErrorMessage,
		result.ExecutedAt,
		result.Duration,
		result.ID,
	)

	return err
}

// GetActionResults retrieves action results for a run
func (db *DB) GetActionResults(ctx context.Context, runID string) ([]models.ActionResult, error) {
	query := `
		SELECT id, run_id, action_id, sequence_id, status, retry_count,
		       screenshot_path, generated_code, error_message, executed_at, duration_ms
		FROM action_results
		WHERE run_id = ?
		ORDER BY sequence_id
	`

	rows, err := db.conn.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get results: %w", err)
	}
	defer rows.Close()

	var results []models.ActionResult
	for rows.Next() {
		var result models.ActionResult
		err := rows.Scan(
			&result.ID,
			&result.RunID,
			&result.ActionID,
			&result.SequenceID,
			&result.Status,
			&result.RetryCount,
			&result.ScreenshotPath,
			&result.GeneratedCode,
			&result.ErrorMessage,
			&result.ExecutedAt,
			&result.Duration,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, result)
	}

	return results, nil
}
