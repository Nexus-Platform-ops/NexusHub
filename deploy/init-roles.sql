-- Dev-only role bootstrap, mounted by docker-compose.yml on first
-- initialisation. Production uses init-roles.sh (env-supplied passwords)
-- instead; schemas are applied afterwards by the one-shot migrate service.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexus_social') THEN
    CREATE ROLE nexus_social LOGIN PASSWORD 'dev_social';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexus_dating') THEN
    CREATE ROLE nexus_dating LOGIN PASSWORD 'dev_dating';
  END IF;
END $$;
