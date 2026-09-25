# snip's architecture

Two diagrams in the [C4 model](https://c4model.com/)'s style: **context** (snip
and the people and systems around it) and **containers** (the separately
running pieces inside it). The *why* behind them is in [the ADRs](adr/).

## Level 1: system context

```mermaid
flowchart TB
  owner["<b>Link owner</b><br/>[person]<br/>Creates and manages short links<br/>with an API key"]
  visitor["<b>Visitor</b><br/>[person]<br/>Clicks a short link"]
  snip["<b>snip</b><br/>[software system]<br/>Shortens URLs, redirects visitors,<br/>counts clicks"]
  dest["<b>Destination websites</b><br/>[external systems]"]
  owner -- "creates, lists, deletes links<br/>(JSON over HTTPS)" --> snip
  visitor -- "GET /{slug}" --> snip
  snip -. "302 redirect: the visitor's browser<br/>goes to the destination" .-> dest
```

## Level 2: containers

```mermaid
flowchart TB
  owner["Link owner"] --> lb
  visitor["Visitor"] --> lb
  lb["<b>Load balancer / ingress</b><br/>[nginx, or a Kubernetes Gateway]"]
  api["<b>snip API</b><br/>[Go: snip serve]<br/>Management API and redirects"]
  worker["<b>Click worker</b><br/>[Go: snip-worker]<br/>Batches click events into totals"]
  redis[("<b>Redis</b><br/>Cache, click stream,<br/>rate-limit counters<br/>(soft dependency: ADR 6)")]
  pg[("<b>Postgres</b><br/>Links, API keys, click totals<br/>(source of truth: ADR 2)")]
  lb --> api
  api -- "slug → URL (cache)" --> redis
  api -- "on miss; all writes" --> pg
  api -- "XADD click event (ADR 5)" --> redis
  worker -- "read batches" --> redis
  worker -- "add totals" --> pg
```

Each snip API copy is stateless, so the API scales by running more copies
behind the load balancer. The worker runs separately so click processing can
fall behind, or be restarted, without affecting redirects.

(Levels 3 and 4 of C4, components and code, are the package layout under
`internal/` and the code itself; handbook chapter 5.4 walks through them.)
