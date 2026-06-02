# Distributed Physical Access Control System

A cloud-native physical access control platform for TSMC Cloud Native 2026. It separates **real-time door decisions** (Fast Path, Redis-backed, sub-50ms target) from **analytics and reporting** (Slow Path, ClickHouse-backed, sub-200ms render target).

No extra demo CSV or handover documents are required—everything needed to run the stack is in this repository (`clickhouse/seed.sql`, Makefile targets, and scripts).

---

## Table of contents

1. [Features](#features)
2. [Architecture](#architecture)
3. [Prerequisites](#prerequisites)
4. [Quick start](#quick-start)
5. [Services and ports](#services-and-ports)
6. [Makefile reference](#makefile-reference)
7. [Demo identities (seed data)](#demo-identities-seed-data)
8. [Access API](#access-api-port-8080)
9. [Admin API](#admin-api-port-8081)
10. [Report API](#report-api-port-8082)
11. [Report UI and org tree](#report-ui-and-org-tree)
12. [Permission model](#permission-model)
13. [Observability](#observability)
14. [Repository layout](#repository-layout)
15. [Testing](#testing)
16. [Troubleshooting](#troubleshooting)
17. [Git hooks](#git-hooks)
18. [Local-only files](#local-only-files)

---

## Features

| Area | Capability |
|------|------------|
| **Access control** | Badge swipe → ALLOW/DENY with card validation, permission check, anti-passback |
| **Admin** | Ban/unban employees; Kafka-driven Redis cache invalidation |
| **Ingestion** | Kafka `inout-events` → aggregation worker → ClickHouse `inout_events` |
| **Pre-aggregation** | Materialized views for daily entry/exit counts per org unit |
| **Reporting** | Personal, department, and audit reports; CSV/PDF export |
| **Analytics** | Door traffic heatmap (org subtree), attendance trends (daily/weekly/monthly/quarterly/yearly), workforce utilization |
| **Visual PDF** | Metadata header, KPI summary, detail table, embedded charts |
| **Web UI** | Chart.js dashboard at `/ui/` with role-based tabs |
| **Observability** | Prometheus metrics, Grafana dashboards, anti-passback alerting |

---

## Architecture

### Fast Path (door decision)

```
Badge reader / curl / badge-reader-sim
        ↓
   access-api :8080  ←→ Redis (permissions, anti-passback, deny cache)
        ↓ publish (async)
   Kafka topic: inout-events
        ↓
   aggregation-worker → INSERT inout_events (ClickHouse)
```

### Slow Path (reports)

```
report-api :8082
    → ClickHouse (events, org_unit, employee, pre_aggregated_reports MVs)
    → Redis (5-minute report cache)
    → JSON / CSV / visual PDF
```

### Admin / ban flow

```
admin-api :8081
    → ClickHouse employee (ReplacingMergeTree)
    → Kafka topic: permission-events
    → cache-invalidation-worker → Redis perm:denied keys
```

### Data store

**ClickHouse only** for persistence: raw events, org tree (`materialized_path`), employees (`report_role`), doors, and pre-aggregated report MVs.

---

## Prerequisites

| Requirement | Notes |
|-------------|--------|
| **Docker Desktop** | Must be running; `docker ps` should succeed |
| **Shell** | Run commands from the **repository root** |
| **jq** (recommended) | Pretty-print JSON from health checks and scripts |
| **Go 1.24+** (optional) | Only if you run `badge-reader-sim` or tests outside Docker |

If you see `Cannot connect to the Docker daemon`, start Docker Desktop and retry.

---

## Quick start

```bash
# Clone and enter the project
cd "Distributed Physical Access Control System"

# Start the full stack (~1–2 min): compose, Kafka topics, ClickHouse schema, seed, Redis cards
make up

# Health checks
curl -s http://localhost:8080/health | jq .
curl -s http://localhost:8081/health | jq .
curl -s http://localhost:8082/health | jq .

# Simulate one badge swipe (ALLOW + door opens on success path)
make swipe

# Open the reporting UI in a browser
open http://localhost:8082/ui/    # macOS
# xdg-open http://localhost:8082/ui/   # Linux
```

### Fresh environment (wipe all data)

```bash
make down -v    # removes volumes including ClickHouse data
make up
```

### Daily restart (keep data)

```bash
docker compose up -d --build
# Re-apply schema/seed only if tables or demo data are missing:
make schema-ch schema-ch-migrate seed-ch seed
```

### Generate sample traffic and reports

```bash
make demo-full
# Writes local files (gitignored): report_demo.csv, report_demo.pdf
```

---

## Services and ports

| Service | Local Dev URL | GCP Production URL (GKE) | Description / Notes |
|---------|---------------|--------------------------|---------------------|
| **access-api** | `http://localhost:8080` | `https://api.8.233.250.204.nip.io/access` | Door swipe & decision path |
| **admin-api** | `http://localhost:8081` | `https://api.8.233.250.204.nip.io/admin` | Ban/unban management |
| **report-api** | `http://localhost:8082` | `https://api.8.233.250.204.nip.io/reports` | REST Reporting APIs |
| **Report charts UI** | `http://localhost:8082/ui/` | `https://api.8.233.250.204.nip.io/ui/` | Role-based Dashboard UI |
| **Grafana** | `http://localhost:3001` | `https://8.233.250.204.nip.io/` | Managed Service for Grafana |
| **Prometheus** | `http://localhost:9090` | *Internal (GCP GMP / Monarch)* | Metric storage engine |
| **ClickHouse HTTP** | `http://localhost:8123` | `https://xxxxxx.gcp.clickhouse.cloud` | ClickHouse Cloud HTTP endpoint |
| **ClickHouse native** | `localhost:9000` | *GCP ClickHouse Cloud native* | ClickHouse Cloud TCP port |
| **Redis** | `localhost:6379` | *Internal GKE service* | Fast cache & state store |
| **Kafka** | `localhost:9092` | *Internal GKE service* | Async event buffer queue |

---

## Makefile reference

| Target | Description |
|--------|-------------|
| `make up` | `docker compose up -d --build`, init Kafka topics, apply ClickHouse schema + migration, seed CH + Redis |
| `make down` | Stop stack and **delete volumes** (`-v`) |
| `make build` | Build compose images only |
| `make schema-ch` | Apply `clickhouse/init.sql` |
| `make schema-ch-migrate` | Apply `clickhouse/migrate-analytics.sql` (door table, MVs, analytics) |
| `make seed-ch` | Load demo org/employees/doors from `clickhouse/seed.sql` |
| `make seed` | Redis card UID mappings via `scripts/utils/seed-redis.sh` |
| `make swipe` | One simulated IN swipe (`DIRECTION=OUT` supported) |
| `make demo` | Anti-passback demo script |
| `make demo-ban` | Ban → swipe DENY → unban flow |
| `make demo-full` | Bulk swipes → department CSV/PDF export smoke test |
| `make demo-report` | Roles, analytics APIs, audit `sourceIp`, export smoke test |
| `make demo-passback-alert` | Spike ANTI_PASSBACK denials for Grafana/Prometheus alert |
| `make ban USER=<uuid>` | Ban employee via admin-api |
| `make unban USER=<uuid>` | Unban employee |
| `make verify-pipeline` | Confirm swipe event lands in ClickHouse |
| `make load-demo` | Traffic spike (`LOAD_COUNT`, `LOAD_INTERVAL`) |
| `make load-shift-change` | Shift-change simulation (`SHIFT_COUNT` default 90000, `SHIFT_WORKERS` default 200) for Grafana |
| `make report-export-pdf` | Quick PDF to `report_export.pdf` (override `REPORT_USER`, `ORG_UNIT`) |
| `make logs` | Follow compose logs for core services |
| `make test-unit` | Go unit tests for all modules |
| `make test-integration` | access-api integration tests (testcontainers) |
| `make test-e2e-pipeline` | Full pipeline test (requires `make up`) |
---

## Demo identities (seed data)

After `make up` or `make seed-ch`, ClickHouse contains the following. **No CSV download is required.**

### People and roles

| Role | UUID | `report_role` |
|------|------|---------------|
| CEO | `aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa` | `CEO` |
| VP Engineering | `bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb` | `VP` |
| Team-A Manager | `cccccccc-cccc-cccc-cccc-cccccccccccc` | `TEAM_MANAGER` |
| Demo employee | `22222222-2222-2222-2222-222222222222` | `EMPLOYEE` |
| Banned seed user | `00000000-0000-0000-0000-000000000099` | `EMPLOYEE` (`is_active=0`) |

### Org units

| Unit | UUID | `materialized_path` (example) |
|------|------|-------------------------------|
| TSMC Corp (root) | `a0000000-0000-0000-0000-000000000001` | `/a0000000-...-0001/` |
| Engineering | `a0000000-0000-0000-0000-000000000002` | `.../0002/` |
| Team-A | `a0000000-0000-0000-0000-000000000003` | `.../0003/` |

### Doors and cards

| Resource | UUID / value |
|----------|----------------|
| Fab-12 Main Gate | `11111111-1111-1111-1111-111111111111` |
| Fab-12 East Wing | `22222222-2222-2222-2222-222222222221` |
| R&D Lobby | `33333333-3333-3333-3333-333333333331` |
| Card `CARD001` | Demo employee |
| Card `CARDMGR` | Team-A Manager |
| Card `CARDCEO` | CEO |

Ban flow does **not** pre-seed `perm:denied` in Redis—use `make ban` or admin-api.

---

## Access API (port 8080)

### Swipe example

```bash
curl -X POST http://localhost:8080/access/swipe \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dev-api-key-2026" \
  -d '{
    "userId": "22222222-2222-2222-2222-222222222222",
    "doorId": "11111111-1111-1111-1111-111111111111",
    "direction": "IN",
    "cardUid": "CARD001",
    "timestamp": "2026-05-19T08:00:00Z"
  }'

```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/access/swipe` | Process badge swipe; target &lt;50ms p99 via Redis |
| `GET` | `/access/employee/{userId}/state` | Anti-passback state (`IN` / `OUT`) |
| `GET` | `/access/door/{doorId}/status` | Door ONLINE/OFFLINE |
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Prometheus metrics (`access_api_*`) |

### Badge reader simulator

```bash
make swipe
make swipe DIRECTION=OUT

cd badge-reader-sim && go run ./cmd/sim --direction IN
cd badge-reader-sim && go run ./cmd/sim --count 100 --interval 50ms
```

---

## Admin API (port 8081)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/admin/employees/{userId}/ban` | Deactivate employee + publish BAN event |
| `POST` | `/admin/employees/{userId}/unban` | Reactivate + publish UNBAN event |
| `GET` | `/health` | Health check |

```bash
make ban USER=22222222-2222-2222-2222-222222222222
make unban USER=22222222-2222-2222-2222-222222222222
```

---

## Report API (port 8082)

All endpoints require header **`X-User-ID`** (employee UUID). Authorization uses:

- `org_unit.materialized_path` subtree checks
- `employee.report_role` (`CEO`, `CFO`, `VP`, `DIRECTOR`, `TEAM_MANAGER`, `EMPLOYEE`)

### Charts UI (recommended)

Open **http://localhost:8082/ui/** — see [Report UI and org tree](#report-ui-and-org-tree) for hierarchy and locked org dropdown behavior.

- Switch persona (CEO, manager, employee); org list shows **only units in your subtree**
- Department summary, sub-unit comparison (when children exist), door heatmap, attendance trends
- Download CSV / PDF links

### REST endpoints

| Method | Path | Query / body | Description |
|--------|------|--------------|-------------|
| `GET` | `/reports/personal` | `startDate`, `endDate` | Personal attendance (all roles) |
| `GET` | `/reports/department` | `orgUnitId`, dates, `granularity` | Department summary + periods + sub-units |
| `GET` | `/reports/audit` | dates, optional filters | Paginated audit log (`sourceIp` in response) |
| `GET` | `/reports/analytics/door-heatmap` | `orgUnitId`, `minutes` (default 60) | Door swipe ranking (subtree-scoped) |
| `GET` | `/reports/analytics/attendance-trends` | `orgUnitId`, dates, `granularity` | Avg hours + late rate series |
| `GET` | `/reports/analytics/workforce-utilization` | `orgUnitId`, dates | Headcount utilization + on-site rate |
| `GET` | `/reports/export` | `format=csv\|pdf`, `type=events\|personal\|department` | Synchronous export |
| `POST` | `/reports/export/jobs` | JSON body | Async export job |
| `GET` | `/reports/export/jobs/:jobId` | — | Poll / download async export |
| `GET` | `/health` | — | Health check |
| `GET` | `/metrics` | — | Prometheus metrics |
| `GET` | `/ui/` | — | Static Chart.js UI |

`granularity`: `daily` | `weekly` | `monthly` | `quarterly` | `yearly` (default `daily`).

**Workforce utilization:** `uniqueEmployees / headcount` in range; `onSiteRate` uses latest ALLOW direction per employee.

### curl examples

```bash
MANAGER=cccccccc-cccc-cccc-cccc-cccccccccccc
TEAM=a0000000-0000-0000-0000-000000000003
CEO=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
CORP=a0000000-0000-0000-0000-000000000001
TODAY=$(date +%Y-%m-%d)
MONTH=$(date +%Y-%m-01)

# Department report (JSON)
curl -s -H "X-User-ID: $MANAGER" -H "X-API-Key: dev-api-key-2026" \
  "http://localhost:8082/reports/department?orgUnitId=$TEAM&startDate=$MONTH&endDate=$TODAY&granularity=daily" | jq .

# Door heatmap (last 60 minutes)
curl -s -H "X-User-ID: $MANAGER" -H "X-API-Key: dev-api-key-2026" \
  "http://localhost:8082/reports/analytics/door-heatmap?minutes=60" | jq .

# Attendance trends
curl -s -H "X-User-ID: $MANAGER" -H "X-API-Key: dev-api-key-2026" \
  "http://localhost:8082/reports/analytics/attendance-trends?orgUnitId=$TEAM&startDate=$MONTH&endDate=$TODAY&granularity=daily" | jq .

# Visual department PDF (metadata, KPIs, detail table, charts)
curl -s -H "X-User-ID: $MANAGER" -H "X-API-Key: dev-api-key-2026" \
  "http://localhost:8082/reports/export?orgUnitId=$TEAM&startDate=$TODAY&endDate=$TODAY&format=pdf&type=department" \
  -o my_report.pdf

# CEO full-site PDF (executive overview when exporting corp root)
curl -s -H "X-User-ID: $CEO" -H "X-API-Key: dev-api-key-2026" \
  "http://localhost:8082/reports/export?orgUnitId=$CORP&startDate=$TODAY&endDate=$TODAY&format=pdf&type=department" \
  -o ceo_report.pdf

# CSV event export for org subtree
curl -s -H "X-User-ID: $MANAGER" -H "X-API-Key: dev-api-key-2026" \
  "http://localhost:8082/reports/export?orgUnitId=$TEAM&startDate=$TODAY&endDate=$TODAY&format=csv&type=events" \
  -o events.csv

```

### Metrics and rules

- **Entry/exit counts**: `pre_aggregated_reports` materialized view (fast path into aggregates).
- **Avg hours / late rate**: Computed from `inout_events` (ALLOW events, IN/OUT pairs per day).
- **Late rate rule**: An employee-day is *late* if the first `ALLOW` `IN` is **after 09:00:00 UTC** that calendar day.
- **Security counters in PDF**: `ANTI_PASSBACK` and `PERMISSION_DENIED` deny counts in range.

**Performance (local reference):** Department JSON often &lt;100ms with Redis cache warm; visual PDF export typically 100–200ms depending on data volume. Design target for report **data** delivery is &lt;200ms (see course spec).

**Demo note:** `make demo-full` uses **current UTC time** for swipe timestamps. If you run it after 09:00 UTC, late rate may show **~100%** even when avg hours are low—that is expected under the UTC rule, not a PDF bug.

### Visual PDF layout (page 1)

1. **Report metadata** — title, statistical window (`00:00:00`–`23:59:59`), generator identity, org `materialized_path`
2. **Executive summary** — total IN/OUT, avg on-site hours, unique employees, late rate, anti-passback denies, blacklist/banned swipe attempts
3. **Detailed breakdown** — sub-units (if any) or per-employee rows with swipe count, hours, anomaly notes
4. **Period breakdown** table (daily/weekly/monthly rows)
5. Subsequent pages: charts (traffic, hours, late rate, heatmap, attendance)

---

## Report UI and org tree

**Recommended entry for Slow Path demos:** [http://localhost:8082/ui/](http://localhost:8082/ui/) (after `make up`).

### Demo organization tree

```text
TSMC Corp（全公司 root）
└── Engineering（處）
    └── Team-A（課）
```

Each row in `org_unit` has a `materialized_path`. A manager may only query org units **inside their own path prefix** (self + descendants). Querying a parent org (e.g. Team-A manager selecting “全公司”) returns **HTTP 403** from the API.

### How to use the UI

1. Open **http://localhost:8082/ui/**
2. **登入身份** — sets `X-User-ID` and role (CEO / VP / 課長 / 員工).
3. **組織單位** — dropdown lists **only org units you are allowed to see** (locked units are hidden; a blue hint shows what is locked).
4. Click **重新載入圖表** — department summary, sub-unit chart (if any children), heatmap, attendance trends.
5. **CEO** at **TSMC Corp** shows the executive overview block and sub-unit comparison (Engineering vs Team-A).

| Demo login (toolbar) | Default org | Can select in dropdown |
|----------------------|-------------|----------------------|
| CEO — 全公司 | TSMC Corp | Corp, Engineering, Team-A |
| VP — Engineering | Engineering | Engineering, Team-A only |
| 課長 — Team-A | Team-A | **Team-A only** |
| 員工 | — | Org hidden; personal tab only |

### Verify enforcement (optional)

```bash
# Team manager → Team-A: OK
curl -s -o /dev/null -w "%{http_code}\n" -H "X-User-ID: cccccccc-cccc-cccc-cccc-cccccccccccc" \
  "http://localhost:8082/reports/department?orgUnitId=a0000000-0000-0000-0000-000000000003&startDate=2026-05-01&endDate=2026-05-22"

# Same manager → Corp root: 403
curl -s -H "X-User-ID: cccccccc-cccc-cccc-cccc-cccccccccccc" \
  "http://localhost:8082/reports/department?orgUnitId=a0000000-0000-0000-0000-000000000001&startDate=2026-05-01&endDate=2026-05-22"
```

---

## Permission model

| Role | Report scope |
|------|----------------|
| `CEO`, `CFO` | Entire company subtree; corp-level PDF executive overview |
| `VP`, `DIRECTOR` | Own division and all descendant org units |
| `TEAM_MANAGER` | Own team subtree |
| `EMPLOYEE` | Personal report and own audit rows only |

Rules enforced in report-api:

- `orgUnitId` in requests must be inside the requester’s subtree (`IsInSubtree` on `materialized_path`).
- Otherwise HTTP **403**.
- Employees do not see department / heatmap / trend tabs in `/ui/`.
- The `/ui/` org dropdown mirrors the same rules (only permitted units are listed).

---

## Observability

Starts with `make up` under `monitoring/`.

| Component | Local Dev URL | GCP Production URL (GKE) |
|-----------|---------------|--------------------------|
| **Grafana** | `http://localhost:3001` | `https://8.233.250.204.nip.io/` |
| **Prometheus** | `http://localhost:9090` | *Internal (GCP GMP / Monarch)* |
| **access-api metrics** | `http://localhost:8080/metrics` | *Internal GKE endpoint (port 8080)* |
| **report-api metrics** | `http://localhost:8082/metrics` | *Internal GKE endpoint (port 8082)* |

**Grafana folder:** `Access Control`

| Dashboard | Data source | Content |
|-----------|-------------|---------|
| Shift Change Monitor | Prometheus | Swipe QPS, p99 latency, ALLOW/DENY |
| Access Analytics | ClickHouse + Prometheus | Door heatmap, monthly hours/late rate, passback spike |

**Alerting:** Prometheus / Grafana alert when `report_passback_deny_1m_max >= 50`. Configure Slack under Grafana → Alerting → Contact points (`security-slack` in provisioning).

```bash
make demo-passback-alert
make load-demo LOAD_COUNT=200 LOAD_INTERVAL=20ms
# Shift-change spike (open Grafana Shift Change Monitor first)
make load-shift-change
# Quick local smoke: SHIFT_COUNT=2000 SHIFT_WORKERS=50 make load-shift-change

### Simulating 90,000 different people

The load tool assigns **one synthetic UUID per request index** (`00000000-0000-4000-a000-{index}`), default **direction=IN** (everyone entering at shift change). No `cardUid` is sent, so master data is not required for the Fast Path.

```bash
# Before first 90k run (clears prior passback keys in Redis)
make shift-change-prep
make load-shift-change

# Softer ramp (often lowers p99): spread 90k over 60s
SHIFT_RAMP=60s make load-shift-change

# Optional: also insert 90k rows into ClickHouse for reporting demos
make seed-load-users LOAD_USER_COUNT=90000
```

**Improving p99 &lt; 50ms locally:** use `--unique-users` + `IN` only (avoids anti-passback storms), `SHIFT_WORKERS=100–150`, optional `SHIFT_RAMP=30s`, rebuild access-api after pulling (Redis pool + no gin access log on `/access/swipe`).
```

---

## Security & Middleware

To protect the APIs and ensure system reliability, the following middleware components are applied to `access-api`, `admin-api`, and `report-api`:

### 1. API Key Authentication
All external endpoints (except `/health`, `/metrics`, and `/ui/` dashboard assets) require a valid API Key passed via the `X-API-Key` HTTP header. 
* **Header Format:** `X-API-Key: <your-api-key>`
* **Configuration:** Controlled by the `API_KEY` environment variable (defaults to `dev-api-key-2026` for development).
* **Unauthorized Access:** Requests without a valid key return `401 Unauthorized`.

### 2. Per-IP Rate Limiting
A local in-memory token bucket rate-limiter prevents brute-force attempts and controls load spike limits per client IP.
* **Configuration:** Configured via `RATE_LIMIT_RPS` env var.
* **Default Limits:**
  * `access-api`: 100 RPS
  * `report-api`: 50 RPS
  * `admin-api`: 20 RPS
* **Rate Limit Exceeded:** Returns `429 Too Many Requests`.

---

## GCP & Kubernetes (GKE) Deployment

The microservices are fully cloud-ready and deployed to **Google Cloud Platform (GCP) / GKE Autopilot**.

### 1. Build & Push Container Images

Build and push the microservice images to Google Artifact Registry for `linux/amd64` platform:

```bash
# Configure docker authentication for Artifact Registry
gcloud auth configure-docker asia-east1-docker.pkg.dev

# Build and push services (replace :v1 with your target tag)
REGISTRY="asia-east1-docker.pkg.dev/access-api-497314/access-api-repo"
SERVICES=("access-api" "admin-api" "report-api" "aggregation-worker" "cache-invalidation-worker")

for service in "${SERVICES[@]}"; do
  docker buildx build --platform linux/amd64 -t "${REGISTRY}/${service}:v1" "./${service}" --push
done
```

### 2. GKE Autopilot Cluster Setup & Secrets Configuration

1. **Namespace Setup:**
   ```bash
   kubectl apply -f k8s/01-namespace.yaml
   ```

2. **Secret Management via GCP Secret Manager:**
   We leverage Kubernetes `ExternalSecret` and Workload Identity Service Accounts (`secret-reader-sa`) to securely synchronize credentials:
   - Create a Secret named `CLICKHOUSE_PASSWORD` in GCP Secret Manager containing your ClickHouse Cloud password.
   - Configure the GCP IAM Role binding for the Kubernetes Service Account (`secret-reader-sa`) to allow it access to Secret Manager.
   - Apply the Secret Store and External Secret:
     ```bash
     kubectl apply -f k8s/secrets/secret-store.yaml
     kubectl apply -f k8s/secrets/external-secret.yaml
     ```
     This automatically syncs and creates the native `app-secrets` Secret in the `access-control` namespace.

3. **Application Configuration:**
   - Update `k8s/02-config.yaml` with your actual ClickHouse Cloud endpoint (`CLICKHOUSE_ADDR`) and Redis internal address (`REDIS_ADDR`).
   - Apply the Configuration:
     ```bash
     kubectl apply -f k8s/02-config.yaml
     ```

### 3. Deploy Applications & Ingress

1. **Deploy Microservices:**
   ```bash
   kubectl apply -f k8s/apps/
   ```
   *Note: Deployments are configured with a `Recreate` deployment strategy in GKE Autopilot to coordinate clean updates.*

2. **Setup Ingress & HTTPS (Cert-Manager):**
   ```bash
   kubectl apply -f k8s/ingress/cert-manager-issuer.yaml
   kubectl apply -f k8s/ingress/api-ingress.yaml
   ```

### 4. Database Cloud Migration & Redis Seeding

Since the cloud environment connects to ClickHouse Cloud and a cloud Redis instance, you can use the Makefile targets with a `.env` file to apply schemas and seed database credentials from your local machine:

1. **Configure your `.env` file:**
   ```env
   API_URL="https://api.8.233.250.204.nip.io/access"
   ADMIN_URL="https://api.8.233.250.204.nip.io/admin"
   REPORT_URL="https://api.8.233.250.204.nip.io/reports"
   REDIS_ADDR="10.140.0.3:6379"  # Use port forwarding if seeding from external local
   CLICKHOUSE_ADDR="xxxxxx.gcp.clickhouse.cloud:8443"
   CLICKHOUSE_USER="default"
   CLICKHOUSE_PASSWORD="your-cloud-password"
   API_KEY="your-prod-api-key"
   ```

2. **Apply ClickHouse Cloud Schema & Migrations:**
   ```bash
   make schema-ch-cloud
   ```

3. **Seed Cloud Redis Card UIDs:**
   ```bash
   # Ensure you have set up a port-forward to your GKE Redis service first (e.g. port 6379)
   kubectl port-forward svc/redis -n access-control 6379:6379 &
   make seed
   ```

### 5. Managed Observability (GMP & Grafana)

The GKE cluster runs Google Cloud Managed Service for Prometheus (GMP) for scraping metrics, and hosts a Grafana instance for visual dashboarding.

1. **Apply Scrapers & Rules:**
   ```bash
   kubectl apply -f k8s/monitoring/access-control-scraper.yaml
   kubectl apply -f k8s/monitoring/prometheus-rules-k8s.yaml
   ```

2. **Deploy Grafana & Ingress:**
   ```bash
   kubectl apply -f k8s/monitoring/grafana-managed-cert.yaml
   kubectl apply -f k8s/monitoring/grafana-oss.yaml
   kubectl apply -f k8s/monitoring/grafana-ingress.yaml
   ```

---

---

## Repository layout

```
access-api/              # Fast path: swipe decision, Redis, Kafka publish
admin-api/               # Ban/unban, ClickHouse employee writes
aggregation-worker/      # Kafka consumer → ClickHouse events
cache-invalidation-worker/
report-api/              # Reports, analytics, PDF, /ui/
badge-reader-sim/        # CLI swipe simulator
clickhouse/
  init.sql               # Core schema + MVs
  migrate-analytics.sql  # Door table, traffic MV, analytics extras
  seed.sql               # Demo org, employees, doors (single-line INSERTs)
docs/                    # Technical spec, SLO targets, operations runbook
monitoring/              # Prometheus, Grafana, dashboards, alerts
scripts/                 # demo/ and utils/ subdirectories containing shell scripts
docker-compose.yml
Makefile

```

---

## Testing

```bash
make test-unit           # All Go modules
make test-integration    # Redis testcontainers (no full stack required)
make test-e2e-pipeline   # Kafka → ClickHouse path (requires make up)
make verify-pipeline     # Shell script: swipe + poll ClickHouse
```

---

## Troubleshooting

| Symptom | What to do |
|---------|------------|
| Docker daemon error | Start Docker Desktop |
| Reports show all zeros | Run `make swipe` or `make demo-full`; wait a few seconds for ingestion |
| `seed-ch` syntax error | Ensure `clickhouse/seed.sql` uses single-line `INSERT` statements |
| HTTP 403 on reports | Check `X-User-ID` and that `orgUnitId` is in the user’s subtree |
| Late rate 100% in demo | Swipes used current UTC time after 09:00 UTC; see [Metrics and rules](#metrics-and-rules) |
| PDF layout overflow | Update to latest `report-api` (wrapped metadata path and detail table) |
| Ban does not DENY | Use `make ban`; confirm cache-invalidation-worker is running |

---

## Course context

TSMC Cloud Native 2026 — distributed physical access control with Fast Path / Slow Path separation, hierarchical reporting, and observability.

**Remote repository:** GitHub may redirect to  
https://github.com/XintiWu/TSM-Access-Control-System.git
