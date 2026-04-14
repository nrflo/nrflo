UPDATE default_templates
SET template = '## Callback Instructions

This agent is being re-run due to a callback from **${CALLBACK_FROM_AGENT}**.

${CALLBACK_INSTRUCTIONS}
',
default_template = '## Callback Instructions

This agent is being re-run due to a callback from **${CALLBACK_FROM_AGENT}**.

${CALLBACK_INSTRUCTIONS}
'
WHERE id = 'callback' AND type = 'injectable';

UPDATE default_templates
SET template = '## User Instructions

${USER_INSTRUCTIONS}
',
default_template = '## User Instructions

${USER_INSTRUCTIONS}
'
WHERE id = 'user-instructions' AND type = 'injectable';

UPDATE default_templates
SET template = '## Continuation From Saved State

Your previous run was interrupted at low context. Below is the summary it saved:

${PREVIOUS_DATA}
',
default_template = '## Continuation From Saved State

Your previous run was interrupted at low context. Below is the summary it saved:

${PREVIOUS_DATA}
'
WHERE id = 'low-context' AND type = 'injectable';
