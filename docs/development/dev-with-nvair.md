# Unifabric Development with NVAIR

## 1. Deploy the Environment

```bash
bash e2e/topology/setup.sh all
```

## 2. Configure KubeConfig

```bash
export KUBECONFIG=./kubeconfig-unifable-e2e-topology-external.yaml
```

## 3. Run Individual Stages

```bash
bash e2e/topology/setup.sh step1-install-topology --delete-if-exists
bash e2e/topology/setup.sh step2-setup-rdma-rxe node-gpu-1,node-gpu-2,node-gpu-3,node-gpu-4
bash e2e/topology/setup.sh step3-install-monitoring-operators
IMAGE_REGISTRY="ghcr.io" IMAGE_NAMESPACE="unifabric-io" IMAGE_TAG="YOU_TAG" \
  bash e2e/topology/setup.sh step4-install-unifabric
IMAGE_REGISTRY="ghcr.io" IMAGE_NAMESPACE="unifabric-io" IMAGE_TAG="YOU_TAG" \
  bash e2e/topology/setup.sh step5-deploy-switch-agent
```

## 4. Uninstall Unifabric

```bash
helm uninstall unifabric \
  --namespace unifabric-system \
  --wait --debug
```

## 5. Delete the Simulation Environment

```bash
nvair delete simulation unifable-e2e-topology
```
