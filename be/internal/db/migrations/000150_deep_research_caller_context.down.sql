-- Remove the caller-context section from the scope prompt (see the up migration).
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'The user''s research question is provided above as your instructions.

Caller-supplied context (may be empty — if blank, ignore it and research the question on its own terms):
${EXTERNAL_CONTEXT}

When that context is present, bias the angles toward what it implies the caller cares about (their domain, tech stack and versions, constraints, and what they already know) while still covering the question objectively.',
       'The user''s research question is provided above as your instructions.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'scope';
