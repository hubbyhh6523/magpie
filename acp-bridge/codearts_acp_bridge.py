#!/usr/bin/env python3
"""
codearts_acp_bridge.py — expose a Huawei CodeArts ACP agent as an OpenAI API.

Measured protocol (not guessed), against codeartsAgent 2.02:

    initialize  {protocolVersion:1}
      -> agentInfo {name:"codeartsAgent", version:"2.02"}
         loadSession / fork / resume / list / close, mcpCapabilities http+sse,
         promptCapabilities embeddedContext + image
    session/new {cwd, mcpServers:[]}
      -> {sessionId, configOptions:[{id:"model",category:"model",
          currentValue, options:[18]}, {category:"mode"}]}
    session/set_config_option {sessionId, configId:"model", value}
    session/prompt {sessionId, prompt:[{type:"text", text}]}
      -> session/update stream: agent_message_chunk / usage_update / state_update
         then {stopReason, usage:{totalTokens,inputTokens,...}}

Auth is one cookie, hlp_token (HttpOnly EdDSA JWT, 7 days), plus the workspace's
own session cookies. hlp_token is bound to the workspace uuid, NOT to the
container: a rebuilt container keeps working with the same token. What does
change is the session cwd (/root/job-envs/sandboxes/codearts-<ts>), so this
bridge reads it from the env API every time it opens a session instead of
hard-coding it -- that is what makes a container rebuild survive unattended.
"""

import argparse
import asyncio
import base64
import json
import ssl
import sys
import time
import uuid
from typing import Any

try:
    import websockets
except ImportError:
    sys.exit("pip install websockets")

try:
    import urllib.request
except ImportError:
    sys.exit("urllib missing")


def token_expiry(cookie: str) -> int | None:
    """The hlp_token JWT's exp, or None when there is no readable one.

    EdDSA-signed, so this only reads the payload: it never verifies, and never
    should -- the signature is the gateway's to check, and the payload's exp is
    what tells us when to stop trusting a long-running bridge.
    """
    tok = None
    for part in cookie.split(";"):
        part = part.strip()
        if part.startswith("hlp_token="):
            tok = part[len("hlp_token="):]
            break
    if not tok:
        return None
    try:
        payload = tok.split(".")[1]
        payload += "=" * (-len(payload) % 4)
        return int(json.loads(base64.urlsafe_b64decode(payload))["exp"])
    except Exception:
        return None


def token_state(cookie: str) -> dict:
    """exp, days left, and whether the bridge should be warning about it."""
    exp = token_expiry(cookie)
    if exp is None:
        return {"ok": False, "reason": "no readable hlp_token"}
    left = exp - int(time.time())
    return {
        "ok": left > 0,
        "exp": exp,
        "expires_at": time.strftime("%Y-%m-%d %H:%M:%SZ", time.gmtime(exp)),
        "seconds_left": left,
        "days_left": round(left / 86400, 2),
    }


class EnvProbe:
    """Reads the live workspace env: sandboxId, cwd, state."""

    def __init__(self, host: str, cookie: str):
        self.host = host
        self.cookie = cookie
        self._cache: dict | None = None
        self._at = 0.0

    def read(self, ttl: float = 5.0) -> dict:
        now = time.time()
        if self._cache and now - self._at < ttl:
            return self._cache
        url = f"https://{self.host}/api/workenv/v1/envs/codearts"
        req = urllib.request.Request(url)
        req.add_header("Cookie", self.cookie)
        req.add_header("User-Agent",
                       "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/138")
        with urllib.request.urlopen(req, timeout=20) as r:
            d = json.loads(r.read())
        self._cache, self._at = d, now
        return d

    def cwd(self) -> str:
        return self.read()["cwd"]

    def fingerprint(self) -> str:
        d = self.read()
        return f"{d.get('sandboxId')}|{d.get('state')}"



def _socket_open(ws) -> bool:
    """websockets 12 had .closed; 13+ uses .state / .close_code."""
    if ws is None:
        return False
    if hasattr(ws, "closed"):
        return not ws.closed
    st = getattr(ws, "state", None)
    if st is not None:
        name = getattr(st, "name", str(st))
        return name == "OPEN"
    return getattr(ws, "close_code", None) is None


class AcpClient:
    """One ACP connection. ACP is request/response per id, serialised here."""

    def __init__(self, url: str, cookie: str, env: EnvProbe):
        self.url = url
        self.cookie = cookie
        self.env = env
        self.ws = None
        self._id = 0
        self._pending: dict[int, asyncio.Future] = {}
        self._updates: asyncio.Queue = asyncio.Queue()
        self._reader: asyncio.Task | None = None
        self.info: dict = {}

    async def connect(self):
        headers = {"Cookie": self.cookie} if self.cookie else {}
        self.ws = await websockets.connect(
            self.url, additional_headers=headers, max_size=None,
            ping_interval=20, open_timeout=25, close_timeout=5)
        self._reader = asyncio.create_task(self._read_loop())
        init = await self.request("initialize", {"protocolVersion": 1})
        self.info = init.get("agentInfo", {})
        return init

    async def _read_loop(self):
        try:
            async for raw in self.ws:
                msg = json.loads(raw)
                if "id" in msg and msg["id"] in self._pending:
                    fut = self._pending.pop(msg["id"])
                    if not fut.done():
                        fut.set_result(msg)
                elif msg.get("method") == "session/update":
                    await self._updates.put(msg["params"]["update"])
        except Exception as e:
            for fut in self._pending.values():
                if not fut.done():
                    fut.set_exception(RuntimeError(f"acp closed: {e}"))

    async def request(self, method: str, params: dict, timeout: float = 120) -> dict:
        self._id += 1
        rid = self._id
        fut = asyncio.get_running_loop().create_future()
        self._pending[rid] = fut
        await self.ws.send(json.dumps({"jsonrpc": "2.0", "id": rid,
                                       "method": method, "params": params}))
        msg = await asyncio.wait_for(fut, timeout=timeout)
        if "error" in msg:
            raise RuntimeError(f"acp {method}: {msg['error']}")
        return msg["result"]

    async def new_session(self) -> tuple[str, list]:
        # cwd read live: a rebuilt container has a new sandbox path
        r = await self.request("session/new",
                               {"cwd": self.env.cwd(), "mcpServers": []})
        return r["sessionId"], r.get("configOptions", [])

    async def set_model(self, sid: str, value: str) -> list:
        r = await self.request("session/set_config_option",
                               {"sessionId": sid, "configId": "model",
                                "value": value})
        return r.get("configOptions", [])

    def drain(self):
        while not self._updates.empty():
            self._updates.get_nowait()

    def send_prompt(self, sid: str, text: str) -> asyncio.Task:
        return asyncio.create_task(
            self.request("session/prompt",
                         {"sessionId": sid,
                          "prompt": [{"type": "text", "text": text}]},
                         timeout=300))

    async def close(self):
        if self._reader:
            await self._reader.cancel()
        if self.ws:
            try:
                await self.ws.close()
            except Exception:
                pass


class Bridge:
    """One ACP session, rebuilt transparently when the container is replaced."""

    def __init__(self, url: str, cookie: str, env: EnvProbe):
        self.url = url
        self.cookie = cookie
        self.env = env
        self.client: AcpClient | None = None
        self.session: str | None = None
        self.model: str | None = None
        self.models: list[dict] = []
        self._lock = asyncio.Lock()
        self._fp: str = ""

    async def _rebuild(self):
        if self.client:
            await self.client.close()
        self.client = AcpClient(self.url, self.cookie, self.env)
        await self.client.connect()
        sid, opts = await self.client.new_session()
        self.session = sid
        mo = next((o for o in opts if o.get("category") == "model"), None)
        if mo:
            self.models = mo.get("options", [])
            self.model = mo.get("currentValue")
        self._fp = self.env.fingerprint()

    async def ensure(self):
        async with self._lock:
            fp = self.env.fingerprint()
            if self.client is None or self.session is None:
                await self._rebuild()
            elif fp != self._fp or not self.client.ws or not _socket_open(self.client.ws):
                # container replaced (new sandboxId) or the socket dropped
                await self._rebuild()
            return self.client, self.session

    async def select(self, model_id: str):
        client, sid = await self.ensure()
        short = model_id.split("/", 1)[-1]
        match = next((m for m in self.models
                      if m["value"] == model_id or m["value"].endswith("/" + short)),
                     None)
        if not match:
            raise KeyError(f"unknown model {model_id}")
        if match["value"] == self.model:
            return self.model
        async with self._lock:
            opts = await client.set_model(sid, match["value"])
            self.model = match["value"]
            mo = next((o for o in opts if o.get("category") == "model"), None)
            if mo:
                self.models = mo.get("options", [])
        return self.model

    async def stream(self, model: str | None, prompt: str):
        """Yield text pieces from one ACP turn."""
        client, sid = await self.ensure()
        if model:
            if model.startswith("codearts-acp/"):
                model = model.split("/", 1)[1]
            try:
                await self.select(model)
            except KeyError:
                pass
        client.drain()
        task = client.send_prompt(sid, prompt)
        while True:
            getter = asyncio.ensure_future(client._updates.get())
            done, _ = await asyncio.wait({getter, task}, timeout=300,
                                         return_when=asyncio.FIRST_COMPLETED)
            if task in done and getter not in done:
                getter.cancel()
                break
            if not done:
                getter.cancel()
                break
            upd = getter.result()
            if upd.get("sessionUpdate") == "agent_message_chunk":
                piece = upd.get("content", {}).get("text", "")
                if piece:
                    yield piece
        usage = {}
        if task.done() and not task.cancelled():
            try:
                usage = (task.result() or {}).get("usage", {})
            except Exception:
                pass
        self.last_usage = usage


def sse(obj: Any) -> bytes:
    return b"data: " + json.dumps(obj, ensure_ascii=False).encode() + b"\n\n"


def join_messages(body: dict) -> str:
    parts = []
    for m in body.get("messages") or []:
        c = m.get("content")
        if isinstance(c, str):
            parts.append(c)
        elif isinstance(c, list):
            parts.extend(x.get("text", "") for x in c if x.get("type") == "text")
    return "\n".join(p for p in parts if p)


async def http_app(reader, writer, bridge: Bridge):
    try:
        head = await reader.readuntil(b"\r\n\r\n")
        lines = head.decode().split("\r\n")
        method, path, _ = lines[0].split(" ")
        headers = {}
        for ln in lines[1:]:
            if ":" in ln:
                k, v = ln.split(":", 1)
                headers[k.lower().strip()] = v.strip()
        n = int(headers.get("content-length", 0))
        raw = await reader.readexactly(n) if n else b""

        if path.startswith("/v1/models"):
            await bridge.ensure()
            payload = {"object": "list", "data": [
                {"id": "codearts-acp/" + m["value"].split("/", 1)[-1],
                 "object": "model", "owned_by": "codearts-acp",
                 "root": m["value"]} for m in bridge.models],
                # the one thing that needs a human: surfaced where a client
                # already looks, not buried in a log
                "auth": token_state(bridge.env.cookie)}
            body = json.dumps(payload).encode()
            writer.write(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n"
                         b"Content-Length: " + str(len(body)).encode() +
                         b"\r\nConnection: close\r\n\r\n" + body)
            await writer.drain()
            return

        if path.startswith("/v1/chat/completions") and method == "POST":
            req = json.loads(raw or b"{}")
            prompt = join_messages(req)
            model = req.get("model")
            if req.get("stream"):
                writer.write(b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n"
                             b"Cache-Control: no-store\r\nConnection: close\r\n\r\n")
                await writer.drain()
                cid = "chatcmpl-" + uuid.uuid4().hex[:16]
                created = int(time.time())
                async for piece in bridge.stream(model, prompt):
                    writer.write(sse({"id": cid, "object": "chat.completion.chunk",
                                      "created": created,
                                      "model": model or bridge.model,
                                      "choices": [{"index": 0,
                                                   "delta": {"content": piece},
                                                   "finish_reason": None}]}))
                    await writer.drain()
                u = getattr(bridge, "last_usage", {}) or {}
                writer.write(sse({"id": cid, "object": "chat.completion.chunk",
                                  "created": created,
                                  "model": model or bridge.model,
                                  "choices": [{"index": 0, "delta": {},
                                               "finish_reason": "stop"}],
                                  "usage": {"prompt_tokens": u.get("inputTokens", 0),
                                            "completion_tokens": u.get("outputTokens", 0),
                                            "total_tokens": u.get("totalTokens", 0)}}))
                writer.write(b"data: [DONE]\n\n")
                await writer.drain()
                return

            text = []
            async for piece in bridge.stream(model, prompt):
                text.append(piece)
            u = getattr(bridge, "last_usage", {}) or {}
            out = {"id": "chatcmpl-" + uuid.uuid4().hex[:16],
                   "object": "chat.completion", "created": int(time.time()),
                   "model": bridge.model,
                   "choices": [{"index": 0,
                                "message": {"role": "assistant",
                                            "content": "".join(text)},
                                "finish_reason": "stop"}],
                   "usage": {"prompt_tokens": u.get("inputTokens", 0),
                             "completion_tokens": u.get("outputTokens", 0),
                             "total_tokens": u.get("totalTokens", 0)}}
            body = json.dumps(out, ensure_ascii=False).encode()
            writer.write(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n"
                         b"Content-Length: " + str(len(body)).encode() +
                         b"\r\nConnection: close\r\n\r\n" + body)
            await writer.drain()
            return

        writer.write(b"HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n"
                     b"Connection: close\r\n\r\n")
        await writer.drain()
    except Exception as e:
        try:
            body = json.dumps({"error": {"message": str(e),
                                         "type": "bridge_error"}}).encode()
            writer.write(b"HTTP/1.1 500 Internal Server Error\r\n"
                         b"Content-Type: application/json\r\nContent-Length: " +
                         str(len(body)).encode() + b"\r\nConnection: close\r\n\r\n" + body)
            await writer.drain()
        except Exception:
            pass
    finally:
        try:
            writer.close()
        except Exception:
            pass


async def watchdog(bridge: Bridge, every: float = 60):
    """Rebuild the session when the container is replaced; warn before the
    hlp_token dies. The token is the one thing this bridge cannot renew on its
    own (EdDSA, private key server-side), so it says so while there is still
    time to act instead of failing 401 at 3am."""
    warned: set[int] = set()
    while True:
        await asyncio.sleep(every)
        try:
            fp = bridge.env.fingerprint()
            if bridge._fp and fp != bridge._fp:
                print(f"[watch] container changed: {bridge._fp} -> {fp}, rebuilding")
                await bridge.ensure()
                print(f"[watch] rebuilt, session={bridge.session} model={bridge.model}")
        except Exception as e:
            print(f"[watch] {type(e).__name__}: {e}")

        st = token_state(bridge.env.cookie)
        if not st.get("ok"):
            print(f"[token] EXPIRED or unreadable: {st.get('reason') or st.get('expires_at')}"
                  " -- refresh hlp_token and restart the bridge")
            continue
        days = st["days_left"]
        for mark in (2, 1):
            if days <= mark and mark not in warned:
                warned.add(mark)
                print(f"[token] {days} day(s) left, expires {st['expires_at']}"
                      " -- refresh hlp_token from DevTools > Application > Cookies")


async def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--host", required=True,
                    help="workspace host, e.g. 5066-xxxx.workspace.developer.huaweicloud.com")
    ap.add_argument("--cookie", default="")
    ap.add_argument("--cookie-file", default="")
    ap.add_argument("--addr", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=8099)
    args = ap.parse_args()

    cookie = args.cookie
    if args.cookie_file:
        cookie = open(args.cookie_file).read().strip()

    host = args.host.strip()
    url = f"wss://{host}/codearts-acp/ws"

    env = EnvProbe(host, cookie)
    st = env.read(ttl=0)
    print(f"[bridge] env state={st.get('state')} sandbox={st.get('sandboxId')}")
    print(f"[bridge] cwd={st.get('cwd')}")

    bridge = Bridge(url, cookie, env)
    await bridge.ensure()
    print(f"[bridge] agent={bridge.client.info} models={len(bridge.models)}")

    asyncio.create_task(watchdog(bridge))
    server = await asyncio.start_server(
        lambda r, w: http_app(r, w, bridge), args.addr, args.port)
    print(f"[bridge] listening on http://{args.addr}:{args.port}/v1")
    async with server:
        await server.serve_forever()


if __name__ == "__main__":
    asyncio.run(main())