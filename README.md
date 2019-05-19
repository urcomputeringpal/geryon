## Geryon

_[/‘dʒɪəriən/ <br> ge-re-on](https://youtu.be/lhbB4FDKg8Y)_ <br>
_noun_

1. A mythological Greek monster. Like the :octocat:, it's not entirely clear how many legs Geryon had.
2. A GitHub App for Kubernetes clusters. Run it on your cluster to let others in your organization connect their repo to that cluster by installing the app.

## Features

### Namespace creation

Geryon will create a Kubernetes namespace named after each GitHub Repository it is installed on.

### ImagePullCredentials for GitHub Package Registry

Namespaces created by Geryon include a Secret containing regularly-refreshed imagePullSecrets credentials to allow them to access Docker images pushed to the GitHub Package Registry. Namespaces' `default` ServiceAccount are also patched to refer these imagePullSecrets.

## Installation

* [Create a new GitHub App](https://github.com/settings/apps/new?name=geryon-your-cluster-name-goes-here&url=https://example.com&callback_url=https://example.com&private=true&packages=read) with the following settings:
  * Name: geryon-your-cluster-name-goes-here
  * Homepage URL: https://example.com/
  * Webhook URL: https://example.com/ (we'll come back in a minute to update if you choose to enable webhooks)
  * Webhook Secret: Generate a unique secret with `openssl rand -base64 32 | tee webhook-secret` 
  * Permissions:
    * Repository metadata: Read-only
    * Packages: Read-only
* Generate and download a new key for your app. Copy it to `private-key.pem`
* Download [`kustomization.example.yaml`](./kustomization.example.yaml) and rename it to `kustomization.yaml`
* Update the `APP_ID` value to reflect the numeric ID of your GitHub app
* Create an Ingress resource at `ingress.yaml` as required by your Kubernetes provider
  * See [this GKE example](https://cloud.google.com/kubernetes-engine/docs/tutorials/http-balancer) for reference
* Create a `geryon` namespace on your Kubernetes cluster: `kubectl create ns geryon`
* Apply `geryon` to your cluster: `kubectl apply -k .`
* Update your GitHub app's Webhook URL to the URL of your Ingress resource

## Development

1. Create a GitHub App and generate a private key
1. Create `.env`:
```
PORT=8081
WEBHOOK_SECRET=asdf
APP_ID=30576
PRIVATE_KEY_FILE=geryon-dev.2019-05-12.private-key.pem
KUBECONFIG=/path/to/your/dev-cluster/.kube/config
```
1. Run the thing:
```
go build ./cmd/geryon && env $(cat .env | xargs) ./geryon
```
