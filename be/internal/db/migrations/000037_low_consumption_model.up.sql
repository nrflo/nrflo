ALTER TABLE agent_definitions RENAME COLUMN low_consumption_agent TO low_consumption_model;
UPDATE agent_definitions SET low_consumption_model = '' WHERE low_consumption_model != '';
