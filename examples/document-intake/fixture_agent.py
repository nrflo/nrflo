import json
import os
import sys

sys.path.insert(0, os.environ['NRFLO_SDK_DIR'])
import nrflo_sdk

c = nrflo_sdk.client()
c.findings.add('extraction', json.dumps({
    'title': 'Supplier onboarding request',
    'reference': 'INTAKE-001',
}))
c.agent.finished()
