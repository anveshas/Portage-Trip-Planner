import json
import urllib.error
import urllib.request

from django.conf import settings


def plan_trip(payload):
    request = urllib.request.Request(
        settings.GATE_URL.rstrip("/") + "/v1/plan",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        detail = json.loads(exc.read().decode() or "{}")
        raise RuntimeError(detail.get("error") or "the planner rejected the trip") from exc


def healthy():
    with urllib.request.urlopen(settings.GATE_URL.rstrip("/") + "/health", timeout=2) as response:
        if response.status != 200:
            return False
        body = json.load(response)
        return body.get("status") == "ok" and body.get("self_check") == "passed"
