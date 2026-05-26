# switch-agent hostProc LLDP 采集方式

English version: [switch-agent-host-proc.md](./switch-agent-host-proc.md)

默认情况下，switch-agent 通过挂载出来的 `/run/lldpd.socket` 读取交换机本地 LLDP 数据。如果交换机系统没有把 `lldpd` socket 暴露到宿主机，或者 socket 权限无法满足容器访问，可以显式切换到 `hostProc` 模式。

`hostProc` 模式会挂载宿主机 `/proc`，并通过 `nsenter` 进入宿主机 mount/net namespace 执行宿主机上的 `lldpcli`。这种方式使用交换机系统自带的 `lldpcli` / `lldpd` 版本组合，不使用 switch-agent 镜像里打包的 CLI。

## 适用场景和取舍

默认的 `socket` 模式只需要挂载 `lldpd` socket，容器权限面更小，优先用于能够稳定暴露 `/run/lldpd.socket` 的交换机系统。

`hostProc` 模式需要 `--privileged`，并挂载宿主机 `/proc`，权限要求更大。它的优势是兼容性更高：当交换机系统没有暴露 `lldpd` socket、socket 权限无法满足容器访问，或者 socket 挂载方式不可用时，可以通过进入宿主机 namespace 直接复用宿主机上的 `lldpcli`。

## 启动方式

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

## 验证

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

如果日志中出现 `lldpcli command not found`，需要先确认交换机宿主机环境中可以直接执行：

```bash
lldpcli -f json0 show neighbors
```

## 注意事项

- `hostProc` 模式需要 `--privileged` 和 `/proc:/host/proc:ro`。
- `hostProc` 模式不会使用 `UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION`。
- 如果交换机已经可以稳定挂载 `/run/lldpd.socket`，优先使用默认的 `socket` 模式。
