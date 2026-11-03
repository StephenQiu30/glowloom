-- Agent LangGraph Checkpoint storage. Run explicitly as schema owner.
CREATE TABLE IF NOT EXISTS agent.graph_checkpoint (
    run_id uuid NOT NULL,
    checkpoint_id uuid NOT NULL,
    node_name text NOT NULL,
    state jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, checkpoint_id)
);
CREATE INDEX IF NOT EXISTS graph_checkpoint_latest ON agent.graph_checkpoint(run_id, created_at DESC);
GRANT SELECT, INSERT ON agent.graph_checkpoint TO glowloom_app;
