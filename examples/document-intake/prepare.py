import json
import os
import sys
from pathlib import Path

sys.path.insert(0, os.environ['NRFLO_SDK_DIR'])
import nrflo_sdk

c = nrflo_sdk.client()
source = Path('examples/document-intake/fixture.txt').read_text()
if not source.strip():
    raise ValueError('Input document is empty')
c.findings.add('document', json.dumps({'text': source}))
c.agent.finished()
