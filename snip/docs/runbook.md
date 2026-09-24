# snip runbook

What to do when an alert fires (chapters 13.4–13.6). Each section matches an
alert's `Runbook:` link. Commands assume the Compose stack (chapter 9.3); the
Kubernetes equivalents are noted.

**First, always:** acknowledge the alert, open the snip overview dashboard
(Grafana → snip → snip overview), note the start time, and say in the team
channel that you're on it. If users are affected for more than a few minutes,
declare an incident (chapter 13.5).

---

## high-error-rate

**Alerts:** `SnipErrorBudgetFastBurn` (page), `SnipErrorBudgetSlowBurn` (ticket).
**Means:** too many requests are failing with 5xx.

1. **Which route?** Prometheus:
   `sum by (route, code) (rate(snip_http_requests_total{code=~"5.."}[5m]))`
2. **Did something just change?** A deploy, a config change, a migration?
   (`git log`, the CI/CD history, `kubectl rollout history deploy/api`.)
   If yes and the timing matches: **roll back first, investigate after**
   (`kubectl rollout undo deploy/api`, `helm rollback`, or revert in Git for GitOps).
3. **Are dependencies healthy?**
   - Postgres: `docker compose exec postgres pg_isready -U snip`
     (k8s: `kubectl -n snip get pods -l app.kubernetes.io/name=postgres`)
   - Redis: `docker compose exec redis redis-cli ping`
   - snip's readiness: `/readyz` on an API copy reports which dependency failed.
4. **What do the errors say?**
   `docker compose logs api --since 15m | jq -c 'select(.status >= 500)'`
   Take a `trace_id` from a failing line and open it in Jaeger (chapter 13.3).
5. If the database is overloaded: check slow queries and connections (chapter 6.4);
   consider scaling snip *down* if connections are exhausted (chapter 8.6).

## slow-redirects

**Alert:** `SnipRedirectLatencyBudgetBurn` (page).
**Means:** more than 1% of redirects take over 250 ms.

1. **Cache hit ratio** (dashboard panel): a drop means Redis is down, flushed or
   evicting, and every redirect goes to Postgres.
2. **Traces:** in Jaeger, find `GET /{slug}` traces over 250 ms and see which span
   is long: `cache.GetURL`, `store.Get` or `links.RecordClick`.
3. **Saturation:** CPU and memory of API copies; database CPU and connections;
   is the HPA at its maximum? (`kubectl -n snip get hpa`)
4. **Mitigate:** scale out the API if it's CPU-bound; restore Redis if the cache
   is gone; if one link is extremely hot, see chapter 8.6's hot-key answer.

## click-backlog

**Alert:** `SnipClickBacklogGrowing` (ticket).
**Means:** the worker isn't keeping up; clicks are delayed, not lost.

1. Is the worker running? `docker compose ps worker`
   (k8s: `kubectl -n snip get pods -l app.kubernetes.io/name=worker`)
2. Is it erroring? `docker compose logs worker --since 15m | grep -i error`
   Repeated `processing clicks failed` usually means Postgres is unreachable.
3. Throughput: `rate(snip_worker_clicks_processed_total[5m])` vs incoming
   redirects. If traffic simply grew, **add workers**: the consumer group
   shares the stream (`docker compose up -d --scale worker=3`, or
   `kubectl -n snip scale deploy/worker --replicas=3`).

## target-down

**Alert:** `SnipTargetDown` (page).
**Means:** Prometheus can't scrape a snip process.

1. Is the process running? `docker compose ps` / `kubectl -n snip get pods`
   (look for `CrashLoopBackOff`, `OOMKilled`: chapter 10.5's field guide).
2. If it's crash-looping: `kubectl logs --previous` shows why.
3. If it's running but unscrapable: network policy or port changes
   (chapter 10.3), or the metrics endpoint itself failing.

---

## After any page

- Keep a timeline in the incident channel as you go (what you saw, what you did, when).
- Once stable: schedule a blameless postmortem (chapter 13.5) and fix this
  runbook if a step was wrong, missing or unclear.
