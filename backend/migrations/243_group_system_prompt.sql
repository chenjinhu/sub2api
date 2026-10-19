ALTER TABLE groups ADD COLUMN IF NOT EXISTS system_prompt_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS system_prompt text NOT NULL DEFAULT '';
