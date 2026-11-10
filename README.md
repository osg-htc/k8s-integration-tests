# k8s-integration-tests

Integration tests for Kubernetes-deployed services, written in Go using [Terratest](https://terratest.gruntwork.io/) and [testify](https://github.com/stretchr/testify).

Each test suite creates a randomly-named namespace, applies a Kustomize manifest bundle, runs assertions against the deployed resources, then tears everything down on exit.

---

## Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- A running [minikube](https://minikube.sigs.k8s.io/docs/start/)-based Kubernetes cluster accessible via `kubectl`
  - The test harness depends on several minikube-specific features, other single-node cluster tools like Kind are incompatible
- `kubectl` installed and configured (`~/.kube/config` pointing at your cluster)

---

## Repository Structure

### `manifests/`

Each subdirectory corresponds to a single test suite and contains all Kubernetes resources needed to run it, grouped into a [Kustomize](https://kustomize.io/) bundle. See `manifests/ospool-ep/` as an example. 

### `images/`

Dockerfiles for images built and used by a test suite, grouped by suite. Some suites need a purpose-built test-runner image rather than an existing published one.
See [Building Images Into Minikube](#building-images-into-minikube) for more information.

### `data/`

Static test data files that are mounted into the cluster during test runs. For example, `data/pelican/` contains files served by the Pelican origin pod.

### `test/`

All Go test code lives here in a single `test` package. `test_utils.go` contains shared helpers for common polling and introspection patterns. Individual test suites each get their own `*_test.go` file.

### `test-configs/`

Declarative, YAML-driven checks that don't need bespoke Go code — see [Scripted Tests](#scripted-tests) below.

---

## Test Environment Construction

Each test follows the same lifecycle, illustrated by `test/pelican_test.go`:

1. **Mount host data into minikube** — directories from `data/` on the host are bind-mounted into the minikube VM so that pods can access static test files.

1. **Build test-runner image** (Optional) — For images that depend on different client software per test environment, build the test runner pod's image directly into minikube.

1. **Generate credentials** — secrets for test services (TLS certificates, signing keys, passwords, etc.) are created programmatically via bespoke helper code and applied to the test namespace.

1. **Template and apply Kustomize manifests** — a Go template is applied to the relevant `manifests/` directory to produce a filled-in kustomize directory, which is then deployed with `kubectl apply -k`.

1. **Register teardown via `t.Cleanup`** — a cleanup function is registered immediately after setup. It dumps pod logs, deletes all created secrets and kustomized resources, removes the namespace, and cancels any background context (e.g. the bind mount).

1. **Run sub-tests** — the actual assertions are run as programmatically-generated `t.Run` sub-tests, driven by test scripts configured in `test-config/`. If a foundational sub-test (such as confirming all deployments become ready) fails, subsequent sub-tests are skipped early via an `if t.Failed()` guard.

---

## Scripted Tests

For pod-exec checks of the form "poll a pod until some command succeeds," it's often simpler to describe the check as data than to write bespoke Go. `test/test_config_parser.go` provides `TestHandle.RunTestConfigDir(configDir)`, which reads a `testConfig.yaml` file from `configDir` and runs each entry under its `tests:` list as a subtest.

Each entry supports:

- `name` — the subtest's name.
- `podSelector.labels` — a map of labels identifying the target pod (must match exactly one pod).
- `script` — a path, relative to `configDir`, to a script that is `kubectl cp`'d into the pod, `chmod +x`'d, then executed via `sh -c` until it exits `0`.
- `command` — a list of strings exec'd directly, with no shell and no file copy. Use this for containers (e.g. distroless images) that don't have a shell. `script` and `command` are mutually exclusive — specifying both fails the test.
- `container` — optional; targets a specific container in a multi-container pod for the copy, chmod, and exec steps.
- `retry.timeout` / `retry.wait` — how long (in seconds) to keep retrying, and how long to sleep between attempts.

Example, from `test-configs/ospool-ep/testConfig.yaml`:

```yaml
tests:
- name: Has Singularity
  podSelector:
    labels:
      app: test-cm
  script: has_singularity.sh
  retry:
    timeout: 120
    wait: 10
```

See `test-configs/ospool-ep/` and `test-configs/pelican/` for complete examples, and `test/ospool_ep_test.go` / `test/pelican_test.go` for how `RunTestConfigDir` is wired into a test suite.

---

## CI / Automated Testing

Tests are run automatically via GitHub Actions, defined in `.github/workflows/run-tests.yaml`. The workflow runs:

- **On a nightly schedule** — automatically triggered once per day.
- **On demand** — can be triggered manually via the `workflow_dispatch` event in the GitHub Actions UI.

Each test suite has its own job in the workflow. After each job completes (whether passing or failing), test logs and pod events are uploaded as **artifacts** via `actions/upload-artifact` and are accessible from the GitHub Actions run summary page. Artifacts are retained for 5 days.

---

## Running Tests Locally

Run all tests:

```sh
go test ./test -v
```

Run a specific test suite by name:

```sh
go test ./test -v -run TestOSPoolEP
```
```sh
go test ./test -v -run TestPelican
```
```sh
go test ./test -v -run TestAdstash
```

---

## Building Images Into Minikube

Some test suites need a purpose-built image rather than an existing published one — e.g. `images/adstash/`'s test-runner image, which bakes in a specific `elasticsearch-py`/`opensearch-py` combo per test run. Rather than publishing such images to a registry, a test suite can build them directly into minikube's own image store as part of its Go test setup, using `minikube image build`.

`TestHandle.buildMinikubeImage` in `test/test_runner_utils.go` wraps this: given a Dockerfile directory, an image tag, and a map of build args, it shells out to `minikube image build` and fails the test on error. See its use in `test/adstash_test.go`, which builds a different image per `RunnerTag`/`ESPyVersion`/`OSPyVersion` combination before applying the kustomize dir. This requires no local `docker` CLI or `minikube docker-env` juggling, and works the same way locally and in CI.

---

## Adding a New Test Suite

1. **Add a manifest directory** under `manifests/my-service/` containing your Kubernetes resources and a `kustomization.yaml` that lists them. See `manifests/ospool-ep/` for an example.

1. **Add a test file** at `test/my_service_test.go`. See `test/ospool_ep_test.go` for the standard structure — namespace creation, deferred cleanup, kustomize apply, and sub-tests. Shared helpers in `test_utils.go` can be used directly; add new ones there if the pattern will be reused. For pod-exec checks, prefer adding a `test-configs/my-service/testConfig.yaml` and calling `RunTestConfigDir` over writing a bespoke sub-test — see [Scripted Tests](#scripted-tests).

1. **Add a CI job** to `.github/workflows/run-tests.yaml` following the existing `test-ospool-ep` job as a template, updating the `run` step to target your new test function.
