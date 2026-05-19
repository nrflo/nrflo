"""S43 — Artifacts: stage upload + input_artifacts + agent add/list.

End-to-end exercise of the artifact subsystem
(be/internal/service/artifact.go, be/internal/spawner/artifact_stage.go,
be/internal/cli/artifacts.go):

  1. POST /api/v1/artifact-uploads stages a file as a multipart upload.
  2. The workflow run carries `input_artifacts=[{upload_id, name}]`;
     the spawner materialises every artifact for the workflow instance
     under `$NRF_ARTIFACTS_DIR` before the agent starts.
  3. The agent reads the staged file (asserting NRF_ARTIFACTS_DIR is
     populated), writes a derived output, and calls
     `nrflo agent artifact add` to register it.
  4. GET /api/v1/workflow-instances/{iid}/artifacts lists both rows
     with the right source labels.

Expected PASS:
  - agent run finishes pass
  - findings.input_payload == 's43-marker' (proves the agent read the
    staged input file)
  - artifacts list has exactly 2 rows: one source='input' named
    'hello.txt' and one source='agent' named 'out.txt'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. The current workflow instance has an
input artifact staged under $NRF_ARTIFACTS_DIR. Run each command below
via the Bash tool, in order, then stop.

1. Run: `nrflo findings add input_payload "$(cat "$NRF_ARTIFACTS_DIR/hello.txt")"`
2. Run: `printf 'produced-by-agent' > /tmp/s43-out.txt`
3. Run: `nrflo agent artifact add /tmp/s43-out.txt out.txt`
4. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=10, prompt=PROMPT,
    )

    staged = ctx.client.stage_artifact_upload(
        pid, filename="hello.txt", data=b"s43-marker",
        content_type="text/plain",
    )
    upload_id = staged.get("upload_id") if isinstance(staged, dict) else None
    if not upload_id:
        return ("S43 artifacts e2e", "FAIL",
                f"stage upload missing upload_id: {staged!r}")

    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="artifacts e2e",
        input_artifacts=[{"upload_id": upload_id, "name": "hello.txt"}],
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S43 artifacts e2e", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    payload = (sess.get("findings") or {}).get("input_payload")
    if payload != "s43-marker":
        return ("S43 artifacts e2e", "FAIL",
                f"findings.input_payload = {payload!r}, want 's43-marker' "
                "(agent did not read NRF_ARTIFACTS_DIR file)")

    artifacts = ctx.client.list_artifacts(wfi, pid)
    if not isinstance(artifacts, list):
        return ("S43 artifacts e2e", "FAIL",
                f"list_artifacts returned {type(artifacts).__name__}: {artifacts!r}")
    by_name = {a.get("name"): a for a in artifacts}
    inp = by_name.get("hello.txt")
    out = by_name.get("out.txt")
    if inp is None or out is None:
        return ("S43 artifacts e2e", "FAIL",
                f"expected hello.txt + out.txt, got names={sorted(by_name)}")
    if inp.get("source") != "input":
        return ("S43 artifacts e2e", "FAIL",
                f"hello.txt source = {inp.get('source')!r}, want 'input'")
    if out.get("source") != "agent":
        return ("S43 artifacts e2e", "FAIL",
                f"out.txt source = {out.get('source')!r}, want 'agent'")
    return ("S43 artifacts e2e", "PASS",
            f"input={inp.get('size_bytes')}b agent_out={out.get('size_bytes')}b")
