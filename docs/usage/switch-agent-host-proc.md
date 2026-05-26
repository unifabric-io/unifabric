# switch-agent hostProc LLDP collection

中文版: [switch-agent-host-proc.zh.md](./switch-agent-host-proc.zh.md)

By default, switch-agent reads local switch LLDP data through the mounted `/run/lldpd.socket`. If the switch OS does not expose the `lldpd` socket on the host, or the socket permissions prevent container access, explicitly switch to `hostProc` mode.

`hostProc` mode mounts the host `/proc` and uses `nsenter` to run the host `lldpcli` from the host mount and network namespaces. This mode uses the switch OS built-in `lldpcli` / `lldpd` version pair, not the CLI binaries packaged in the switch-agent image.

## When to use hostProc

The default `socket` mode only mounts the `lldpd` socket and has a smaller container permission surface. Prefer it when the switch OS can reliably expose `/run/lldpd.socket`.

`hostProc` mode requires `--privileged` and mounts the host `/proc`, so it has higher permission requirements. Its advantage is compatibility: when the switch OS does not expose the `lldpd` socket, the socket permissions prevent container access, or socket mounting is not usable, hostProc can enter the host namespaces and reuse the host `lldpcli` directly.

## Start switch-agent

```bash
export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:${LATEST_TAG}"

docker pull "${SWITCH_AGENT_IMAGE}"

docker rm -f unifabric-switch-agent 2>/dev/null || true

docker run -d \
  --name unifabric-switch-agent \
  --restart unless-stopped \
  --network host \
  --uts host \
  --privileged \
  -e UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE=hostProc \
  -v /proc:/host/proc:ro \
  -v /opt/unifabric-switch-agent/mtls:/etc/unifabric/switch-mtls:ro \
  "${SWITCH_AGENT_IMAGE}" \
  /usr/bin/unifabric/switch-agent
```

## Verify

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

If the logs show `lldpcli command not found`, first confirm that the switch host can run:

```bash
lldpcli -f json0 show neighbors
```

## Notes

- `hostProc` mode requires `--privileged` and `/proc:/host/proc:ro`.
- `hostProc` mode does not use `UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION`.
- If the switch can mount `/run/lldpd.socket` reliably, prefer the default `socket` mode.
