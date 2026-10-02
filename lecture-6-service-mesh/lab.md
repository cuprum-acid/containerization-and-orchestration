# Lab 6 — Autopilot release: ships the good, rolls back the bad

## Situation

A service mesh promises encryption, reliability, and smart traffic management with no code changes. In this lab you put all of that to the test on `api` from the `shop` chart: first prove that mesh gives you encryption and reliability for free, then build a self-driving canary release on top of it — a new version gets a small slice of traffic, a gate script watches its health and automatically promotes it on success or rolls it back on rising errors, no human involved. At the end, honestly compute the cost — how much latency and resources this magic costs — and decide whether mesh is worth it here at all.

Scaled down for a modest laptop: a single-node cluster, a lightweight mesh, mesh only around `api`. Everything is deployed through the `shop` Helm chart from Lab 3 — versions, the traffic split, the mesh objects — all chart templates and values, not standalone manifests.

## Part 0 — Services

Extend `api` (fully AI-generated, implementation isn't graded):

- `GET /version` — returns `v1` or `v2`;
- v2 gets a `FAIL=true` switch that makes `POST /order` return a 500 (you'll need it for the rollback);
- v1 gets a separate `FLAKY=true` switch that makes a fraction of requests genuinely fail at random (you'll need it for the reliability part);
- the usual `GET /health`, `POST /order`, `GET /orders`.

Also generate a small gate script: every ~15 seconds it reads v2's error rate and latency from Prometheus and adjusts the traffic split by a simple rule.

## Part 1 — Pick a mesh

Pick one lightweight mesh and justify it in the README:

- **Linkerd** — very lightweight, simple to install;
- **Istio ambient** — Istio's sidecarless mode.

## Part 2 — Measure the baseline

Deploy `api` v1 and `postgres` through the chart, no mesh. Run load against the order path, record the baseline latency and resource use of `api` — your reference point for Part 8.

## Part 3 — Enable mesh and mTLS

Enroll `api` in the mesh through the chart (the required labels/annotations are `values.yaml` values too). Turn on mTLS and prove that traffic between `api` pods is now encrypted, and identity is confirmed by certificate, not by IP.

## Part 4 — Reliability for free

Turn on `FLAKY=true` on v1. Prove with a single graph: the backend genuinely returns an error for a noticeable fraction of requests, while the client barely notices. In the README, work out which mesh mechanism (retries, circuit breaking/outlier detection, or both) was responsible, and how you determined that from the mesh's own metrics.

## Part 5 — Deploy v1/v2 and split traffic

Deploy v1 and v2 simultaneously through the chart, configure a traffic split as a mesh object (also a chart template). Start at 90% on v1, 10% on v2. Check via `GET /version` that requests really do split in that ratio.

## Part 6 — Build the autopilot

Run the gate script. Give it a simple but meaningful rule (e.g.: if v2's error rate stays below a threshold, bump its share up a step; if above, roll it back to 0). In the README, describe the gate's logic in words: which metrics it watches and how it decides.

## Part 7 — Test both scenarios

1. **Bad release:** ship v2 with `FAIL=true`. Show the gate rolling traffic back to v1 on its own.
2. **Good release:** ship a healthy v2 (`FAIL=false`). Show the gate bringing its share up to 100% on its own.

Graphs for both scenarios — how v2's traffic share and error rate changed over time.

## Part 8 — Compute the cost and reach a verdict

Re-measure `api`'s latency and resource use — now with mesh — and compare against the baseline from Part 2. Compute how much latency and resources the proxy added, and reach a verdict: is mesh justified for a system of this size. Back the answer with numbers.

## Part 9 — Monitoring

Add `ServiceMonitor`/`PrometheusRule` templates to the chart, reusing the stack from Lab 2. The mesh itself gives you rich traffic metrics — use them for a dashboard (error rate and latency per version, the traffic split). Bake in 3 alerts, justify each in the README.

## Result

- a baseline (latency, resources) is recorded before mesh;
- mTLS is on, encryption and identity are confirmed cryptographically;
- proved with a graph: the backend genuinely fails a fraction of requests, but the client barely notices — the mesh's retries/circuit breaking work with zero code changes to `api`;
- traffic really splits 90/10 between v1 and v2, confirmed via `/version`;
- the autopilot rolls back a bad release and promotes a good one to 100% on its own — both scenarios backed by graphs;
- the cost of mesh is quantified against the baseline, with a reasoned verdict;
- a dashboard and 3 alerts on top of the Lab 2 stack, baked into the chart.

## What to submit

- The `api` code (v1/v2) and the gate script with a Dockerfile (any implementation, doesn't affect the grade; what's graded is the gate's logic, described in words).
- The updated `shop` Helm chart: `api` versions, the traffic-split object, mesh annotations, `ServiceMonitor`/`PrometheusRule`, the final `values.yaml`.
- `README.md`: the mesh choice and why (Part 1); the baseline (Part 2); proof of mTLS (Part 3); proof of retries/circuit breaking and which mechanism it was (Part 4); the traffic split (Part 5); the gate's logic (Part 6); the results of both scenarios with graphs (Part 7); the cost of mesh and the verdict (Part 8); the 3 alerts and their rationale (Part 9).
- Screenshots: proof of encryption; the reliability graph under `FLAKY=true`; the 90/10 traffic split by `/version`; the automatic rollback of a bad v2; the automatic promotion of a healthy v2; baseline vs. with-mesh comparison; the dashboard.

## How to start

Open the repository with your AI assistant and ask for help with Lab 6. Generate `api` v1/v2 and the gate script (Part 0), then go in order from Part 1. The assistant works step by step and checks your understanding; it won't hand you finished rules or chart templates.

> Using AI? Make sure the assistant follows the rules in [`AGENTS.md`](../AGENTS.md). Most tools pick it up automatically; if not, point it at the file.
