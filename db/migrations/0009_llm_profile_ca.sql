-- Extra CA certificates (PEM) trusted for one LLM profile's endpoint, on top of
-- the system pool: lets a self-hosted gateway with its own CA (GPUStack behind
-- cf-llm-ctl, for instance) work without changing the container's trust store.
-- A CA certificate is public, so it is stored in clear.
ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS ca_cert TEXT NOT NULL DEFAULT '';
