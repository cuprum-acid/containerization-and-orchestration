# Lab 4 — SLA of the critical path under overload

## Situation

`shop` has a critical path — placing an order (`api → postgres`) — and secondary background load, like nightly analytics, that can be sacrificed under pressure. All configuration goes through the `shop` Helm chart from Lab 3: affinity, priorities, resources, PodDisruptionBudget — chart values and templates, not one-off kubectl patches. By hand you only observe and trigger load.

You don't guess the requests/limits for the critical path — `krr` (Kubernetes Resource Recommender) gives them to you based on real usage from Prometheus.

## Part 0 — The batch service

Generate `batch` — a container that burns CPU and memory in an infinite loop, doing nothing useful. Stand-in for "nightly analytics." Implementation isn't graded.

## Part 1 — Pick a spreading mechanism

Add a choice parameter to `values.yaml`: `podAntiAffinity` or `topologySpreadConstraints`. Justify in the README how they differ and why you picked this one.

## Part 2 — A tight cluster

Bring up a kind/k3d cluster with **strictly 1-2 nodes** — small on purpose, so scarcity is easy to trigger. Add `batch` as a new template in the `shop` chart, with sensible requests/limits — like any pod, the Lab 3 guardrails apply with no exceptions — and deploy via `helm upgrade`.

## Part 3 — Real numbers instead of guessing

Run `k6`/`hey` against the order path, let Prometheus collect some data. Run `krr` against your namespace — it analyzes real usage of `api`, `worker`, `postgres` and recommends requests/limits. Put the recommended values into `values.yaml`, deploy via `helm upgrade`. In the README, compare your original guess with what `krr` recommended and explain the gap.

## Part 4 — Spread the critical service

Apply the mechanism from Part 1 to `api`. Prove (`kubectl get pods -o wide`) the replicas landed on different nodes. Explain in the README how this improves fault tolerance.

## Part 5 — Set priorities and guarantees

Add `priorityClass` and `PodDisruptionBudget` templates to the chart, values driven by `values.yaml`:

- `api`, `postgres` — Guaranteed QoS on the numbers from Part 3 (requests = limits), a high `priorityClass`, a PodDisruptionBudget;
- `batch` — Burstable QoS (requests well below limits), a low `priorityClass`. It still has limits, same as the guardrails require for everyone — it's just less protected by priority and QoS class.

Explain in the README what each setting buys you under pressure, and how Burstable differs from Guaranteed in eviction order.

## Part 6 — Create scarcity, catch preemption

Raise `batch.replicas` in `values.yaml`, run `helm upgrade`, until the cluster stops fitting everything. Show and explain:

- pods stuck `Pending` — who didn't fit and why (the scheduler counts reserved requests, not real usage);
- preemption — how the higher-priority `api`/`postgres` evict `batch` to get room.

## Part 7 — Memory pressure, the eviction order

Raise `batch`'s memory use via `values.yaml` until you get real memory pressure on a node. Prove eviction takes `batch` first — the less-protected Burstable, low-priority pod — while the critical path (Guaranteed) survives. Explain in the README how this differs from `OOMKilled` by a pod's own limit — two different diagnoses with the same symptom.

## Part 8 — Prove the SLA under load

While Parts 6–7 are underway, run `k6`/`hey` against the order path and capture latency and error graphs. State your own SLA (e.g. "95% of orders under 300ms, even under overload") and show on the graphs that it holds — while `batch` is what degrades.

## Part 9 — Monitoring

Add `ServiceMonitor`/`PrometheusRule` templates to the chart, reusing the stack from Lab 2. Bake in 3 alerts, justify each in the README. Trigger at least one and show it firing.

## Result

- `api` replicas provably spread across nodes, even on a 1-2 node cluster;
- the critical path's requests/limits come from `krr`'s recommendations, not guesswork;
- every pod, including `batch`, has limits — Lab 3's guardrails apply with no exceptions;
- `Pending` and preemption under scarcity — observed and explained;
- eviction under memory pressure sacrifices `batch` first, by QoS/priority, not by missing limits;
- an SLA on the order path that holds under overload, backed by load-test graphs;
- a dashboard and 3 alerts on top of the Lab 2 stack, baked into the chart.

## What to submit

- The `batch` code with a Dockerfile (any implementation, doesn't affect the grade).
- The updated `shop` Helm chart: the `batch` template, `priorityClass`, `PodDisruptionBudget`, `ServiceMonitor`/`PrometheusRule`, the final `values.yaml`.
- The `krr` report (recommendations) and a comparison against your original guess.
- `README.md`: the spreading mechanism and why (Part 1); `krr`'s recommendations vs. your guess (Part 3); proof of spread (Part 4); the priority/QoS/PDB setup and the Burstable vs Guaranteed difference (Part 5); what `Pending`/preemption showed (Part 6); what eviction showed (Part 7); your SLA and the load-test graphs (Part 8); the 3 alerts and their rationale (Part 9).
- Screenshots: replicas on different nodes; the `krr` report; pods `Pending`; a preemption event; an eviction event; the order-path latency/error graphs under load; the dashboard.

## How to start

Open the repository with your AI assistant and ask for help with Lab 4. Generate `batch` (Part 0), then go in order from Part 1. The assistant works step by step and checks your understanding; it won't hand you finished policies, chart templates, or recommendations.

> Using AI? Make sure the assistant follows the rules in [`AGENTS.md`](../AGENTS.md). Most tools pick it up automatically; if not, point it at the file.
