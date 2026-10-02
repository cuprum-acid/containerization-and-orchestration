# Lab 5 — Zero-trust network with proof

## Situation

By default, the Kubernetes network is flat: any pod can reach any pod. Your mission here is twofold. First, prove that the Service abstraction keeps `shop` running even while pods underneath are constantly dying and being recreated with new IPs. Second, bring `shop` to zero trust — "deny by default, open only what's needed" — and prove it with an access matrix, then, as the finale, find a hidden connectivity break through network observability. Everything goes through the `shop` Helm chart from Lab 3: NetworkPolicy and monitoring are chart templates too, not standalone manifests.

No new service needed here — you use `api`/`worker`/`postgres` from the chart.

## Part 0 — Pick a CNI

Network policies are enforced by the CNI plugin, and not every plugin can do it — the default `kindnet` silently ignores them: the rule gets accepted but never enforced. Pick one and justify it in the README:

- **Cilium** — eBPF-based, also gives you a traffic map (Hubble) and policies down to the application level (L7);
- **Calico** — the classic choice, reliably enforces policies.

## Part 1 — Deploy shop and explore pod networking

Bring up a cluster (kind/k3d) with your chosen CNI, deploy the `shop` chart. Look under the hood of the flat network: the pod's IP and its `eth0` from inside, the paired `veth` end on the node, and that pods on different nodes see each other directly by IP.

## Part 2 — Prove continuity through pod chaos

Expose `api` via `NodePort` — that's your client entry point. Explain in the README how `NodePort` differs from `LoadBalancer` and a headless service.

Run a script that, in the background, kills a random `api` or `worker` pod every few seconds — continuous chaos. In parallel, run steady client load against the service by name (via DNS, not a pod IP). Prove with a single graph: not one client request failed, while underneath, pods kept dying and getting recreated with new IPs the whole time.

In the README, explain why this worked at all — which part of the network stack (`Service`, `EndpointSlice`, `kube-proxy`, DNS) was responsible for what. While you're at it, check which mode your `kube-proxy` runs in (`iptables`/`IPVS`), and explain when the difference actually matters.

## Part 3 — Design the target access matrix

Before restricting anything, design on paper (in the README) a matrix: who SHOULD have access to whom and who SHOULDN'T. As a guide:

- client → `api` (allowed);
- `api` → PostgreSQL (allowed);
- `worker` → PostgreSQL (allowed);
- everything else, including "a stray pod → PostgreSQL" — denied.

Don't forget DNS (services can't find each other by name without it) and the egress you actually need.

## Part 4 — Turn on zero trust

Implement the matrix as NetworkPolicy templates in the `shop` chart, values (which labels, which ports are allowed) driven by `values.yaml`:

1. default-deny — deny all traffic to pods in the namespace by default;
2. open exactly the routes from the matrix, by pod labels, with ports specified;
3. separately allow DNS and the egress you need.

## Part 5 — Prove the matrix in practice

Walk the matrix live: from each pod, try to reach each destination and put the "expected / actual" result in a table. The goal is a fully green matrix: what's allowed works, what's denied doesn't. In particular, show that a stray pod can no longer connect to PostgreSQL, while `api` and `worker` still can.

## Part 6 — Find the hidden connectivity break

Prepare 3-4 candidate faults (e.g.: an overly broad deny rule, wrong labels in an allow rule, a missing DNS/egress allowance, a port in a rule that doesn't match the service's port). Write a script that picks one at random and applies it via `values.yaml` — or ask your AI assistant to introduce a fault without telling you which one.

Detect the symptom (orders stop going through), use the traffic map (Hubble on Cilium, or Calico's logs/tools) to find exactly which connection is being blocked and why, fix it, and confirm via the matrix that everything is green again. In the README, describe your diagnostic path — how you actually found the cause, not guessed it.

## Part 7 — Monitoring

Add `ServiceMonitor`/`PrometheusRule` templates to the chart, reusing the stack from Lab 2 — don't stand up a new one. Metrics: connections, traffic denied by policy, DNS health. Bake in 3 alerts, justify each in the README, trigger at least one.

## Result

- proved with a graph: the client didn't notice a single one of the continuously killed pods;
- NetworkPolicy are chart templates, default-deny plus targeted allows, including DNS/egress;
- the access matrix is fully green: what's allowed works, what's denied doesn't;
- the hidden connectivity break was found via the traffic map, not by guessing, and fixed;
- a dashboard and 3 alerts on top of the Lab 2 stack, baked into the chart.

## What to submit

- The updated `shop` Helm chart: NetworkPolicy templates, `NodePort` for `api`, `ServiceMonitor`/`PrometheusRule`, the final `values.yaml`.
- The pod-chaos script and the hidden-fault script.
- `README.md`: the CNI choice and why (Part 0); how a pod gets its network (Part 1); the continuity graph under pod chaos and why it worked, including the kube-proxy mode (Part 2); the target access matrix (Part 3); the final "expected/actual" matrix — green (Part 5); the hidden-break diagnosis — how you found and fixed it (Part 6); the 3 alerts and their rationale (Part 7).
- Screenshots: pod networking (IP, veth); the "0 failed requests" graph against killed pods; the green access matrix; the traffic map with the found block; the dashboard.

## How to start

Open the repository with your AI assistant and ask for help with Lab 5. The assistant works step by step and checks your understanding; it won't hand you finished policies or chart templates.

> Using AI? Make sure the assistant follows the rules in [`AGENTS.md`](../AGENTS.md). Most tools pick it up automatically; if not, point it at the file.
