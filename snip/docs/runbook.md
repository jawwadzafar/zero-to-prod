# snip runbook

What to do when an alert fires (chapters 13.4–13.7). Each section matches an
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
   - snip's readiness: `/readyz` on an API copy reports each dependency.
     Postgres down → `"ready": false` (503). Redis down → `"degraded": true`
     but still 200: snip keeps redirecting from Postgres (chapter 13.5).
4. **What do the errors say?**
   `docker compose logs api --no-log-prefix --since 15m | jq -c 'select(.status >= 500)'`
   (`--no-log-prefix` drops Compose's `api-1 |` prefix, so each line is pure JSON.)
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

## redis-down

**No dedicated alert**: snip is built to *degrade* without Redis, so you'll see
it as `"degraded": true` on `/readyz`, a cache hit ratio near 0, a rising
`snip_click_record_errors_total`, and `click not recorded` warnings in logs.
**Means:** redirects still work (from Postgres, a little slower); **clicks made
during the outage are lost**, not delayed (they never reached the stream);
rate limiting fails open.

1. Confirm: `docker compose exec redis redis-cli ping`
   (k8s: `kubectl -n snip get pods -l app.kubernetes.io/name=redis`)
2. Restart it: `docker compose restart redis`
   (k8s: `kubectl -n snip rollout restart deploy/redis`).
3. Watch Postgres load while the cache is cold: every redirect is a query until
   the cache refills (chapter 8.6).
4. Record the start and end times: the lost clicks are a data gap to mention in
   the postmortem and to anyone reading click stats for that window.

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

## leaked-api-key

**No alert**: someone tells you. The key's owner, GitHub secret scanning (keys
start with `snip_`), or a stranger who found one in a paste or a public repo.
**Means:** whoever holds the key can create, list and delete its owner's
links. Treat it as a **security incident** (chapter 13.7): SEV1, private channel,
and don't discuss details in public until it's contained.

1. **Which owner?** Hash the reported key the way snip does (SHA-256, hex) and
   look it up. snip stores only the hash:
   `printf %s "$LEAKED" | sha256sum` (macOS: `shasum -a 256`), then
   `docker compose exec -T postgres psql -U snip -c "SELECT id, name, created_at, revoked_at FROM api_keys WHERE key_hash = '<hash>'"`
2. **Contain, now:** `docker compose exec api snip keys rotate <id>`. The old key
   fails from the next request; the owner keeps their links. Send the new key to
   the owner over a channel the attacker can't read. (`snip keys revoke <id>`
   instead, if the key shouldn't exist at all.)
3. **Preserve evidence before anything restarts:** container logs disappear
   with their containers.
   `docker compose logs api --no-log-prefix > api.jsonl`,
   `docker compose logs nginx --no-log-prefix > nginx.log`, then
   `sha256sum api.jsonl nginx.log > SHA256SUMS`. Note who saved them, when, and where.
4. **What did the key do?** Every authenticated request's log line carries
   `owner_id`: `jq -c 'select(.msg == "request" and .owner_id == <id>)' api.jsonl`.
   Changes are the `POST` and `DELETE` lines. Client addresses are in `nginx.log`
   (match by time, method and path).
5. **Undo the damage:** delete links the attacker created (check where they
   point), restore deleted ones from the owner or a backup.
6. **Rotate whatever leaked alongside the key** (a `.env` file often holds the
   database URL too), and watch for `401`s on `/api/` from the old key:
   `jq -c 'select(.status == 401)' api.jsonl`.
7. Write the timeline while it's fresh, and decide with whoever owns legal and
   customer communication whether anyone must be notified (chapter 13.7).

---

## After any page

- Keep a timeline in the incident channel as you go (what you saw, what you did, when).
- Once stable: schedule a blameless postmortem (chapter 13.5) and fix this
  runbook if a step was wrong, missing or unclear.
