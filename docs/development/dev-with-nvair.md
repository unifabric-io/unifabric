# Unifabric Development with NVAIR

## 1. Deploy the Environment

```bash
bash e2e/topology/install.sh
```

## 2. Configure KubeConfig

```bash
export KUBECONFIG=./kubeconfig-unifable-e2e-topology-external.yaml
```

## 3. Install Observability Components

```bash
bash hack/install-monitoring-operators.sh

nvair add forward prometheus --target-node node-gpu-1 --target-port 30090
nvair add forward grafana --target-node node-gpu-1 --target-port 30300
nvair get forwards
```

## 4. Set Up Soft-RoCE

```bash
bash hack/setup-rdma-rxe.sh
```

## 5. Deploy Unifabric

```bash
IMAGE_REGISTRY="ghcr.io"
IMAGE_REPOSITORY="unifabric-io"

IMAGE_TAG="YOU_TAG"

helm upgrade --install unifabric ./chart \
  --namespace unifabric-system \
  --create-namespace \
  --set-string controller.image.registry="${IMAGE_REGISTRY}" \
  --set-string controller.image.repository="${IMAGE_REPOSITORY}/unifabric-controller" \
  --set-string controller.image.tag="${IMAGE_TAG}" \
  --set-string controller.image.pullPolicy=Always \
  --set-string agent.image.registry="${IMAGE_REGISTRY}" \
  --set-string agent.image.repository="${IMAGE_REPOSITORY}/unifabric-agent" \
  --set-string agent.image.tag="${IMAGE_TAG}" \
  --set-string agent.image.pullPolicy=Always \
  --set-string agent.lldp.image.registry="${IMAGE_REGISTRY}" \
  --set-string agent.lldp.image.repository="${IMAGE_REPOSITORY}/unifabric-agent" \
  --set-string agent.lldp.image.tag="${IMAGE_TAG}" \
  --set-string agent.lldp.image.pullPolicy=Always \
  --set-string 'nodeTopologyDiscovery.scaleOutInterfaceSelector=interface=eth1\,eth2\,eth3\,eth4\,eth5\,eth6\,eth7\,eth8' \
  --set-string nodeTopologyDiscovery.storageInterfaceSelector=interface=eth9 \
  --set nvidiaTopograph.enable=false \
  --set grafanaDashboard.enabled=true \
  --set-string grafanaDashboard.kind=GrafanaDashboard \
  --wait --debug
```

## 6. Uninstall Unifabric

```bash
helm uninstall unifabric \
  --namespace unifabric-system \
  --wait --debug
```

## 7. Delete the Simulation Environment

```bash
nvair delete simulation unifable-e2e-topology
```
