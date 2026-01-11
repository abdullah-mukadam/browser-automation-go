-- Add interaction_rank and timestamp columns to semantic_actions table
ALTER TABLE semantic_actions
ADD COLUMN interaction_rank VARCHAR(20) DEFAULT 'Medium',
ADD COLUMN timestamp BIGINT DEFAULT 0;

-- Add index for interaction_rank
CREATE INDEX idx_interaction_rank ON semantic_actions(interaction_rank);
