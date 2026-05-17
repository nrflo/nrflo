"""S38 — Slack notification channel POSTs to its webhook on completion.

Tests:
  - `POST /api/v1/workflows/{wid}/notification-channels` with
    `kind='slack'` and `config.webhook_url=<in-process mock>` registers
    a channel.
  - On `orchestration.completed`, the notify Dispatcher
    (`be/internal/notify/notify.go:18`) enqueues a delivery; the
    Slack transport POSTs `{"text": "..."}` to the webhook URL within
    a few seconds.

Expected PASS:
  - The in-process capture receives ≥1 POST within 15 s.
  - The POST body is JSON containing a non-empty `text` field.
"""

from __future__ import annotations

from lib.http_mock import WebhookCapture
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Run the listed command and stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )

    with WebhookCapture() as capture:
        ctx.client.create_notification_channel(
            pid, wid,
            name=f"s38-{wid}",
            kind="slack",
            config={"webhook_url": capture.url},
            event_types=["orchestration.completed"],
        )

        wfi = ctx.client.run_project_workflow(
            pid, wid, instructions="webhook",
        )["instance_id"]
        wait_for_workflow(ctx, pid, instance_id=wfi)

        if not capture.wait_for(n=1, timeout_s=20.0):
            return ("S38 notification webhook", "FAIL",
                    "no POST to webhook within 20s after completion")

        req = capture.received[0]

    body = req.body_json or {}
    text = body.get("text") if isinstance(body, dict) else None
    if not isinstance(text, str) or text.strip() == "":
        return ("S38 notification webhook", "FAIL",
                f"POST body missing `text`: raw={req.body_raw[:200]!r}")

    return ("S38 notification webhook", "PASS",
            f"POST {req.path} text_bytes={len(text)}")
