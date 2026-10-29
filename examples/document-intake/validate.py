import json
import os
import sys

sys.path.insert(0, os.environ['NRFLO_SDK_DIR'])
import nrflo_sdk

c = nrflo_sdk.client()
document = c.findings.get('prepare', key='document')
extraction = c.findings.get('extract', key='extraction')
if isinstance(document, str):
    document = json.loads(document)
if isinstance(extraction, str):
    extraction = json.loads(extraction)

for field in ('title', 'reference'):
    value = extraction[field]
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f'{field} must be a nonempty string')
    if value not in document['text']:
        raise ValueError(f'{field} must quote the source exactly')

c.findings.add('validated', json.dumps(extraction))
c.findings.add('workflow_final_result', 'Validated document intake: ' + extraction['reference'])
c.agent.finished()
