# CodeArts gateway, in the same container as Magpie

The Docker image runs the CodeArts Gateway v3 together with Magpie, so
blitz (and any plain `docker run` of this image) gets a working `codearts`
provider with no second container and no separate tunnel.

## What is in this folder

- `codearts-gateway` — the vendored gateway binary, v3.1.1, musl static
  (`target/x86_64-unknown-linux-musl/release/codearts-gateway` from the
  `codearts-gateway-v3` source tree), 12,719,600 bytes,
  SHA256 `94e443dbaa9499d45fc6d951dc3c588495248bbd2455c5b30e9dda0563500a4d`.
  Rebuild it from that source tree and replace this file to upgrade it;
  update the version and hash here when you do.
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
`codearts` provider should appear with 9 models.

## Known limitation

The gateway's upstream is Huawei CodeArts MaaS
(`https://snap-access.cn-north-4.myhuaweicloud.com`). The model list is
the gateway's local fallback, so it answers even when the upstream is
unreachable — but a real chat then fails with an upstream-network error.
That was the case from the Huawei ECS host since 2026-11-19 (last
successful chat 2026-11-17); whether blitz's egress reaches the upstream
has to be checked after the redeploy, with a real chat.
