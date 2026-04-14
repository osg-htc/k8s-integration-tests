# k8s-integration-tests

Integration tests for Kubernetes-deployed services, written in Go using [Terratest](https://terratest.gruntwork.io/) and [testify](https://github.com/stretchr/testify).

Each test suite creates a randomly-named namespace, applies a Kustomize manifest bundle, runs assertions against the deployed resources, then tears everything down on exit.

---

## Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- A running Kubernetes cluster accessible via `kubectl` (e.g. [minikube](https://minikube.sigs.k8s.io/docs/start/))
- `kubectl` installed and configured (`~/.kube/config` pointing at your cluster)

---

## Repository Structure

```
k8s-integration-tests/
├── .github/
│   └── workflows/
│       └── run-tests.yaml       # GitHub Actions CI workflow
├── manifests/
│   └── ospool-ep/               # Kubernetes manifests for the OSPool EP test suite
│       ├── kustomization.yaml   # Kustomize entry point, lists all resources
│       ├── central-manager.yaml # HTCondor Central Manager Deployment + Service
│       ├── ospool-ep.yaml       # OSPool Execution Point Deployment
│       └── pool-password.yaml   # ConfigMap and Secret for pool auth credentials
├── test/
│   ├── test_utils.go            # Shared test helper functions
│   └── ospool_ep_test.go        # Test suite for the OSPool Execution Point
├── go.mod
└── go.sum
```

### `manifests/`

Each subdirectory corresponds to a single test suite and contains all Kubernetes resources needed to run it, grouped into a [Kustomize](https://kustomize.io/) bundle. See `manifests/ospool-ep/` as an example.

### `test/`

All Go test code lives here in a single `test` package. `test_utils.go` contains shared helpers for common polling and introspection patterns. Individual test suites each get their own `*_test.go` file.

---

## Running Tests Locally

Run all tests:

```sh
go test ./test/... -v
```

Run a specific test suite by name:

```sh
go test ./test/... -v -run TestOSPoolEP
```

---

## Adding a New Test Suite

1. **Add a manifest directory** under `manifests/my-service/` containing your Kubernetes resources and a `kustomization.yaml` that lists them. See `manifests/ospool-ep/` for an example.

2. **Add a test file** at `test/my_service_test.go`. See `test/ospool_ep_test.go` for the standard structure — namespace creation, deferred cleanup, kustomize apply, and sub-tests. Shared helpers in `test_utils.go` can be used directly; add new ones there if the pattern will be reused.

3. **Add a CI job** to `.github/workflows/run-tests.yaml` following the existing `test-ospool-ep` job as a template, updating the `run` step to target your new test function.