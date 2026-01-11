-- Browser Automation Workflow System Schema
-- MySQL 8.0+

-- Workflow definitions from recorded events
CREATE TABLE IF NOT EXISTS workflow_definitions (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    events_file_path TEXT NOT NULL,
    is_workflow_generated BOOLEAN DEFAULT FALSE,
    start_url TEXT,
    semantic_context JSON,
    parameters JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_created_at (created_at),
    INDEX idx_is_generated (is_workflow_generated)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Individual semantic actions within workflows
CREATE TABLE IF NOT EXISTS semantic_actions (
    id VARCHAR(36) PRIMARY KEY,
    workflow_id VARCHAR(36) NOT NULL,
    sequence_id INT NOT NULL,
    action_type VARCHAR(50) NOT NULL,
    target JSON,
    value TEXT,
    embeddings BLOB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_workflow_sequence (workflow_id, sequence_id),
    INDEX idx_action_type (action_type),
    FOREIGN KEY (workflow_id) REFERENCES workflow_definitions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Workflow execution runs
CREATE TABLE IF NOT EXISTS workflow_runs (
    id VARCHAR(36) PRIMARY KEY,
    workflow_id VARCHAR(36) NOT NULL,
    temporal_run_id VARCHAR(255),
    temporal_workflow_id VARCHAR(255),
    status ENUM('pending', 'running', 'success', 'failed', 'canceled') DEFAULT 'pending',
    parameters JSON,
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    error_message TEXT,
    
    INDEX idx_workflow_id (workflow_id),
    INDEX idx_status (status),
    INDEX idx_started_at (started_at),
    INDEX idx_temporal_run (temporal_run_id),
    FOREIGN KEY (workflow_id) REFERENCES workflow_definitions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Individual action results within runs
CREATE TABLE IF NOT EXISTS action_results (
    id VARCHAR(36) PRIMARY KEY,
    run_id VARCHAR(36) NOT NULL,
    action_id VARCHAR(36) NOT NULL,
    sequence_id INT NOT NULL,
    status ENUM('pending', 'running', 'success', 'failed') DEFAULT 'pending',
    retry_count INT DEFAULT 0,
    screenshot_path TEXT,
    generated_code TEXT,
    error_message TEXT,
    executed_at TIMESTAMP NULL,
    duration_ms BIGINT DEFAULT 0,
    
    INDEX idx_run_id (run_id),
    INDEX idx_run_sequence (run_id, sequence_id),
    INDEX idx_status (status),
    FOREIGN KEY (run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (action_id) REFERENCES semantic_actions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Stored embeddings for similarity search (optional future feature)
CREATE TABLE IF NOT EXISTS embedding_store (
    id VARCHAR(36) PRIMARY KEY,
    action_id VARCHAR(36) NOT NULL,
    embedding_vector BLOB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_action_id (action_id),
    FOREIGN KEY (action_id) REFERENCES semantic_actions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
