#!/usr/bin/env python3
"""Discover workspace host from an externally supplied hlp_token and start ACP bridge."""
import base64, json, os, sys

token = os.environ.get("HLP_TOKEN", "").strip()
if not token:
    sys.exit("Set HLP_TOKEN in the environment; do not commit it.")
try:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    claims = json.loads(base64.urlsafe_b64decode(payload))
    host = f"{claims['prt']}-{claims['env']}.workspace.developer.huaweicloud.com"
except Exception as exc:
    sys.exit(f"Could not derive workspace host from HLP_TOKEN: {exc}")

here = os.path.dirname(os.path.abspath(__file__))
bridge = os.path.join(here, "codearts_acp_bridge.py")
os.execvp("python3", ["python3", bridge, "--host", host, "--cookie", f"hlp_token={token}", "--port", os.environ.get("ACP_BRIDGE_PORT", "8099")])
