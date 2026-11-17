#!/bin/sh
set -e
python3 /opt/acp-bridge/bridge_auto.py &
exec /magpie web --addr 0.0.0.0:3430 --no-open
