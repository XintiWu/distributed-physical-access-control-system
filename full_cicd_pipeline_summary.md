# 完整的 CI/CD 流水線架構與優化總結 (Comprehensive CI/CD Pipeline Summary)

這份文件完整總結了我們為這個分散式門禁系統建立的 CI/CD Pipeline (`.github/workflows/ci.yml`) 的所有技術細節與優化歷程。

---

## 第一階段：靜態代碼分析 (Code Linting)
在這個階段，我們確保所有推送到儲存庫的程式碼都符合 Go 語言的最佳實踐，並且沒有語法或記憶體層級的隱患。
1. **升級 Actions 解決 Node 20 棄用警告**：更新了 `actions/checkout@v4`, `actions/setup-go@v5`, 以及 `golangci-lint-action@v6` 等工具版本，確保 Actions 跑在 Node.js 24 之上，解決了 GitHub Actions 的棄用警告。
2. **多模組並行 Linting**：系統包含多個微服務，我們針對 `access-api`, `admin-api`, `report-api`, `aggregation-worker`, `cache-invalidation-worker` 分別執行 `golangci-lint`，及早捕捉空指標與排版問題。

---

## 第二階段：單元測試與 SonarQube 覆蓋率整合 (Unit Testing & Coverage)
這是我們解決「覆蓋率永遠異常」的關鍵優化區域。
1. **分散式測試與覆蓋率收集**：自動遍歷所有微服務與共用模組 (`pkg`) 執行 `go test -coverpkg=./...`，並產生獨立的 `coverage.out`。
2. **路徑修正 (Path Mapping) 魔法**：由於 Go Module 的路徑與 GitHub 目錄結構不完全一致，我們使用 `sed` 替換 `coverage.out` 中的路徑前綴 (例如將 `github.com/tsmc/access-api/` 替換為 `access-api/`)，這確保了 SonarCloud 能夠精準將覆蓋率對應回 GitHub 上的原始碼。
3. **精準的 SonarCloud 配置**：
    *   明確劃分 `sonar.sources` (產品業務邏輯) 與 `sonar.tests` (測試程式碼)，避免 SonarQube 將測試檔案算進覆蓋率分母。
    *   透過 `sonar.exclusions` 與 `sonar.coverage.exclusions` 排除 `cmd/`、`repository/`、`docs/` 以及 `docker-compose.yml` 等不需計算單元測試覆蓋率的入口與設定檔案。

---

## 第三階段：單元測試覆蓋率衝刺 (Coverage Optimization Phases)
為了解決過低的覆蓋率，我們制定了階段性的推進計畫，將核心業務覆蓋率推升至 **100%**。以下是我們具體「測了什麼」：
*   **Phase 1: `report-api` 報表系統**
    *   **PDF 視覺化渲染**：測試圖表 Fallback 機制、Site Overview 與 Sub-Unit 報表的表格繪製邊界條件 (`pdf_visual_test.go`)。
    *   **非同步匯出工作 (Export Jobs)**：測試 `BuildExportDocument` 與異步任務的狀態扭轉 (Pending -> Processing -> Completed/Failed)。
    *   **API 防呆與權限**：針對 Handler 注入 403 (權限不足)、500 (DB 連線異常) 與參數格式錯誤的模擬請求。
*   **Phase 2: `access-api` & `admin-api` (Queue / Outbox 機制)**
    *   **Kafka 容錯與重試**：測試當 Kafka 連線中斷時，Producer 的 `retryLoop` 是否能正確發揮作用。
    *   **Outbox Pattern 備援**：測試當 Kafka 徹底死線時，系統是否能將刷卡紀錄安全寫入本地硬碟 (`FileOutbox`)，並驗證 Outbox 的讀寫原子性，確保資料不遺失。
*   **Phase 3: `cache-invalidation-worker` (快取清除服務)**
    *   **Redis Cluster 容錯**：測試對 Redis 叢集連線的初始化與 Ping 檢測。
    *   **Kafka 消費者邊界**：測試當收到損壞的 JSON (Invalid Message)、或者寫入 Redis 失敗時，Worker 是否能正確放棄 Commit 並印出錯誤，確保 Offset 推進邏輯不會卡死。

---

## 第四階段：核心服務架設與動態健康檢查 (Service Provisioning & Health Checks)
這階段負責準備整合測試 (Integration Testing) 所需的乾淨環境。
1. **動態健康檢查迴圈**：使用 `docker compose up -d` 啟動 Redis, Kafka, ClickHouse 等核心中間件。並撰寫了 Bash 腳本 (`docker inspect`) 進行持續輪詢。
2. **解決 Flaky Tests**：腳本最高重試 30 次，每 5 秒檢查一次，確保所有中間件的狀態皆為 `healthy` 後才放行下一步。徹底解決了過去因為「資料庫還沒開好，測試就開始跑」而導致 CI 隨機失敗的痛點。

---

## 第五階段：資料初始化與微服務啟動 (Seeding & Microservices)
1. **自動化 Schema 佈署**：透過 Makefile 自動執行 Kafka Topics 建立、ClickHouse Schema 遷移與測試用假資料寫入 (`make seed`)。
2. **微服務聯調啟動**：將所有的微服務 API 與 Workers 透過 Docker Compose 帶起，並等待 20 秒確保應用程式啟動完畢且連線穩定。

---

## 第六階段：端到端整合測試與事件流驗證 (E2E & Pipeline Validation)
在這個階段，我們不再使用 Mock，而是對真實架起的 Docker 容器群發起測試：
1. **端到端測試 (E2E)**：
    *   透過執行 `make test-e2e-pipeline`，注入測試金鑰與資料庫連線字串，直接呼叫真實的 API Endpoints。
    *   **測了什麼**：驗證真實環境下的 DB 查詢是否正確、Middleware 攔截器是否正常工作、以及跨服務 API 的 HTTP 狀態碼回傳是否符合預期。
2. **事件流驗證 (`verify-pipeline.sh`)**：
    *   這是一支嚴格的非同步驗證腳本。
    *   **測了什麼**：
        1. 模擬員工刷卡進門 (`POST /access/swipe (IN)`)。
        2. 擷取 API 回傳的 `eventId` 與 `decision`。
        3. **輪詢 ClickHouse 資料庫**：腳本會每 2 秒去 ClickHouse 的 `inout_events` 表格撈資料，最多等待 30 秒。
        4. 嚴格比對落盤資料的 `id`、`employee_id`、`direction` 與 `status` 是否與第一步 API 回傳的完全一致。
    *   **意義**：證明了「API 寫入 Kafka」→「Aggregation Worker 消費 Kafka」→「批次寫入 ClickHouse」這整條分散式非同步流水線是完全暢通且資料無損的。

---

## 第七階段：效能 SLA 壓力測試 (Performance Validation)
這階段負責把關系統不會因為高併發而崩潰。
1. **自動化部署 k6**：在 CI 機器上即時下載並安裝 Grafana k6 負載測試工具。
2. **執行壓測 (`scripts/k6-load-test.js`)**：
    *   **測了什麼**：腳本會同時模擬兩條核心路徑的極限負載：
        *   **Fast Path (門禁刷卡)**：模擬 50 RPS (每秒 50 次) 的刷卡請求持續轟炸 `access-api`。
        *   **Slow Path (複雜報表)**：模擬 5 RPS 的跨月份部門匯總查詢轟炸 `report-api`。
    *   **SLA 閥值阻擋**：K6 設定了嚴格的閾值 (Thresholds)。如果刷卡的 **P99 延遲超過 50ms**，或是報表查詢的 **P95 延遲超過 200ms**，CI 流水線就會直接亮紅燈 (Fail fast)，拒絕此次不符效能標準的程式碼合併。

---

## 結論
透過上述這套完整的 CI/CD Pipeline，專案目前具備了：
1. **語法與風格的絕對一致性** (Linting)。
2. **極高的程式碼可靠度** (100% 單元測試覆蓋率)。
3. **無懈可擊的系統整合度** (End-to-End 流水線驗證)。
4. **符合企業級標準的效能保證** (k6 壓測把關)。


---
config:
  layout: fixed
---
flowchart TB
 subgraph P1["1. 程式碼品質與單元測試"]
        Lint["靜態代碼分析 (Linting)"]
        Unit["單元測試 (100% 覆蓋率)"]
        Detail1_2["【核心單測涵蓋細節】\n- report-api (報表):  \n * PDF 邊界渲染 &amp; Fallback 機制\n  * 異步任務狀態扭轉流程\n  * 注入 403 (無權限) / 500 (異常) 防呆\n \n- access-api (門禁):\n  * Kafka 中斷重試機制 (retryLoop)\n  * Outbox 本地備援與讀寫原子性\n \n- cache-worker (快取):\n  * Redis Cluster 連線與 Ping 檢測\n  * Kafka 損壞 JSON 與寫入失敗處理"]
  end
 subgraph P2["2. 測試環境建置與初始化"]
    direction TB
        Prov["中介軟體佈署 (Docker)"]
        Health["動態健康檢查"]
        Detail2["【環境建置與穩定性】\n- 解決 Flaky Tests 痛點:\n  * 持續輪詢 (最高 30 次 / 每 5 秒)\n  * 確保 DB 與 Kafka 完全啟動才放行\n \n- 自動化初始化 (Seed):\n  * 自動建立 Kafka Topics\n  * 自動遷移 ClickHouse Schema\n  * 寫入測試用假資料"]
  end
 subgraph P3["3. 端到端與事件流驗證"]
    direction TB
        E2E["E2E 整合測試"]
        Stream["非同步事件流驗證"]
        Detail3["【真實容器驗證細節】\n- 端到端測試 (E2E):\n  * 測試真實 DB 查詢、跨服務 API 串接\n  * 驗證中介層 (Middleware) 攔截器\n\n- 非同步流驗證 (verify-pipeline.sh):\n  * 流程: 刷卡 API -&gt; Kafka -&gt; Worker -&gt; DB\n  * 機制: 每 2 秒輪詢 ClickHouse (上限30秒)\n  * 嚴格比對落盤與 API 回傳資料一致性"]
  end
 subgraph P4["4. 效能 SLA 把關"]
    direction TB
        Perf["k6 壓力測試"]
        Detail4["【效能阻擋標準 (Fail-Fast)】\n- 快速路徑 (門禁刷卡):\n  * 模擬 50 RPS 持續負載壓測\n  * SLA 門檻：P99 延遲 &lt; 50ms\n \n - 慢速路徑 (複雜報表):\n  * 模擬 5 RPS 跨月查詢負載\n  * SLA 門檻：P95 延遲 &lt; 200ms\n\n -CI 管控機制:\n  * 任何指標未達標即直接亮紅燈拒絕合併"]
  end
    Lint --> Unit
    Prov --> Health
    Health --> Detail2
    E2E --> Stream
    Stream --> Detail3
    Perf --> Detail4
    P1 --> P2
    P2 --> P3
    P3 --> P4
    Unit --> Detail1_2

     Lint:::step
     Unit:::step
     Detail1_2:::detail
     Prov:::step
     Health:::step
     Detail2:::detail
     E2E:::step
     Stream:::step
     Detail3:::detail
     Perf:::step
     Detail4:::detail
    classDef phaseBox fill:#f8fafc,stroke:#cbd5e1,stroke-width:2px,color:#0f172a
    classDef step fill:#eff6ff,stroke:#3b82f6,stroke-width:2px,color:#1d4ed8,font-weight:bold
    classDef detail fill:#fffbeb,stroke:#f59e0b,stroke-width:1.5px,color:#92400e,text-align:left,font-size:13px