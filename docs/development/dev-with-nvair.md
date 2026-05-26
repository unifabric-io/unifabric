# Unifabric Development with NVAIR

## 1. Deploy the Environment

```bash
CONTROLLER_REGISTRY="ghcr.io" CONTROLLER_REPOSITORY="unifabric-io/unifabric-controller" CONTROLLER_TAG="YOU_TAG" \
AGENT_REGISTRY="ghcr.io" AGENT_REPOSITORY="unifabric-io/unifabric-agent" AGENT_TAG="YOU_TAG" \
SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:YOU_TAG" \
  bash e2e/topology/setup.sh all --delete-if-exists
```

## 2. Configure KubeConfig

```bash
export KUBECONFIG=./.tmp/kubeconfig-unifable-e2e-topology-external.yaml
```

## 3. Run Individual Stages

```bash
bash e2e/topology/setup.sh step1-install-topology --delete-if-exists
bash e2e/topology/setup.sh step2-setup-rdma-rxe node-gpu-1,node-gpu-2,node-gpu-3,node-gpu-4
bash e2e/topology/setup.sh step3-install-monitoring-operators
CONTROLLER_REGISTRY="ghcr.io" CONTROLLER_REPOSITORY="unifabric-io/unifabric-controller" CONTROLLER_TAG="YOU_TAG" \
  AGENT_REGISTRY="ghcr.io" AGENT_REPOSITORY="unifabric-io/unifabric-agent" AGENT_TAG="YOU_TAG" \
  bash e2e/topology/setup.sh step4-install-unifabric
SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:YOU_TAG" \
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

## 6. GitHub Actions Credentials

The E2E workflow uses the same nvair SSH key pair every run. This avoids
`nvair login` generating a fresh key on each GitHub Actions runner and replacing
the SSH key registered in the NVIDIA Air account.

Configure these GitHub Actions repository secrets:

- `NVAIR_USER`: NVIDIA Air account email.
- `NVAIR_API_TOKEN`: NVIDIA Air API token.
- `NVAIR_SSH_PRIVATE_KEY`: Full contents of `~/.ssh/nvair.unifabric.io`.
- `NVAIR_SSH_PUBLIC_KEY`: Full contents of `~/.ssh/nvair.unifabric.io.pub`.

If the local nvair key does not exist yet, create it once:

```bash
nvair login -u user@example.com -p '<api-token>'
```

Then store the key pair as CI secrets:

```bash
gh secret set NVAIR_SSH_PRIVATE_KEY < ~/.ssh/nvair.unifabric.io
gh secret set NVAIR_SSH_PUBLIC_KEY < ~/.ssh/nvair.unifabric.io.pub
```

Keep `NVAIR_SSH_PRIVATE_KEY` and `NVAIR_SSH_PUBLIC_KEY` in sync with the local
key pair used by the same NVIDIA Air account. The workflow writes them to
`~/.ssh/nvair.unifabric.io` and `~/.ssh/nvair.unifabric.io.pub` before running
`nvair login`.
