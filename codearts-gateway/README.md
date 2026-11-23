# CodeArts gateway, in the same container as Magpie

The Docker image runs the CodeArts Gateway v3 together with Magpie, so
blitz (and any plain `docker run` of this image) gets a working `codearts`
provider with no second container and no separate tunnel.

## What is in this folder

- `codearts-gateway` — the vendored gateway binary, v3.1.2, musl static
  (`target/x86_64-unknown-linux-musl/release/codearts-gateway` from the
  `codearts-gateway-v3` source tree), 12,744,448 bytes,
  SHA256 `ed1f57a930926b6863af247c3162d2f87bf6c26e949f7f6eaf6cf3616b35c6cf`.
  Rebuild it from that source tree and replace this file to upgrade it;
  update the version and hash here when you do. v3.1.2 adds per-stage chat
  timings: each chat request records `session_ms` (MaaS session setup, 0
  when the cached session is reused) and `upstream_ms` (the upstream chat
  call itself; time to first byte for streaming) in `logs.db`
  (`request_logs`, alongside total `latency_ms`) and a `chat timings ...`
  line in `gateway.log`. A `logs.db` from an older version gains the two
  columns on startup.
- `docker-entrypoint.sh` — the container entrypoint (see below).
- `README.md` — this file.

## How it works

`docker-entrypoint.sh` runs before `magpie serve`:

1. If `CODEARTS_AK` / `CODEARTS_SK` are unset, it skips the gateway and
   starts Magpie alone, so the image still deploys with no credentials.
2. Otherwise it starts the gateway on `127.0.0.1:16211`, with its state
   under `/config/codearts-gateway/` (the same `/config` volume Magpie
   uses, so keys and the account store survive redeploys):
   `accounts.json`, `logs.db`, `usage.db`, plus `gw-key` and `gw-secret`
   (both `0600`). The gateway API key and the store secret come from
   `CODEARTS_GW_KEY` / `CODEARTS_GW_SECRET` when set; when unset they are
   generated once on first start and reused afterwards.
3. It waits up to 30 s for `GET /v1/models` to answer, then wires the
   provider with the Magpie CLI: `magpie provider add codearts
   chat=http://127.0.0.1:16211/v1 key=<gw-key>` when there is no `codearts`
   provider yet, or `magpie provider set codearts chat=...` +
   `magpie provider key codearts <gw-key>` when one already exists, then
   `magpie provider refresh codearts` to fetch the model list. A provider
   that pointed at another gateway (for example a tunnel to a server) is
   repointed at the in-container one.
4. It `exec`s `magpie serve` in the foreground. Any container command
   other than `serve` (healthcheck, version, …) runs straight through
   without starting the gateway.

## Blitz setup

Set these env vars in the blitz dashboard (values are yours; nothing
secret is in this repo):

- `CODEARTS_AK` — Huawei Cloud access key id
- `CODEARTS_SK` — Huawei Cloud secret access key
- `CODEARTS_GW_KEY` — optional, the gateway API key (generated if unset)
- `CODEARTS_GW_SECRET` — optional, the store encryption secret
  (generated if unset)
- `CODEARTS_GATEWAY` — optional, upstream base URL override

Then redeploy (blitz redeploys on push to the branch it tracks). The
`codearts` provider should appear with the live upstream model list (19
models on blitz as of 2026-11-23, including Kimi K3 / K3.1 Pro, DeepSeek
V4, GLM-5.3; the 9-model list — 5 route aliases + 4 fallbacks — is only
what the gateway shows when it cannot reach the upstream to probe).

## Known limitation

The gateway's upstream is Huawei CodeArts MaaS
(`https://snap-access.cn-north-4.myhuaweicloud.com`). The model list is
the gateway's local fallback, so it answers even when the upstream is
unreachable — but a real chat then fails with an upstream-network error.
That was the case from the Huawei ECS host since 2026-11-19 (last
successful chat 2026-11-17). Blitz's egress was verified working on
2026-11-23 (codearts/auto answered a real chat); the ECS host's fault is
unfixed, so the same image deployed there still cannot serve real chats.
