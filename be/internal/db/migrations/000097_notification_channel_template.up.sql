-- Add message_template column to notification_channels.
-- Default template literals here must match be/internal/notify/defaults.go::DefaultTemplate.
ALTER TABLE notification_channels ADD COLUMN message_template TEXT NOT NULL DEFAULT '';

UPDATE notification_channels SET message_template='*nrflo* — ${event_type} ${link}
${summary}
agent: ${agent_type} | workflow: ${workflow} | reason: ${reason} | instance: ${instance_id}' WHERE kind='slack';

UPDATE notification_channels SET message_template='*nrflo* — ${event_type} ${link}
${summary}
agent: ${agent_type} \| workflow: ${workflow} \| reason: ${reason} \| instance: ${instance_id}' WHERE kind='telegram';
