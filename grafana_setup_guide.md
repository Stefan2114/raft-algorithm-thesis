# Grafana Dashboard Configuration & Setup Guide

This guide describes exactly how to build and configure the dashboards for your Raft cluster thesis presentation. For each panel, it explains **what** to configure, **why** it matters, and **where** to find each option in the Grafana panel editor. It aligns with the real-time observability figures captured in the thesis paper and details the PromQL/LogQL queries, dashboard panels, and visualization properties.

---

## Observability Infrastructure Access

When you run the cluster using Docker Compose (`docker-compose up -d`), the telemetry stack boots automatically:
- **Grafana**: [http://localhost:3000](http://localhost:3000) (Default Login: `admin` / `admin`)
- **Prometheus**: [http://localhost:9090](http://localhost:9090)
- **Loki Log Engine**: Running on port `3100`

### Adding Data Sources in Grafana
Before building the panels, ensure your data sources are connected:
1. **Prometheus**:
   - Go to **Connections > Data sources > Add data source**
   - Select **Prometheus** and set the URL to `http://localhost:9090`.
   - Click "Save & Test".
2. **Loki**:
   - Go to **Connections > Data sources > Add data source**
   - Select **Loki** and set the URL to `http://localhost:3100`.
   - Click "Save & Test".

---

## 1. Consensus Layer (State & Terms)

This section monitors the cluster's high-level consensus state, visualizes node roles, and tracks terms during Chaos Monkey outages.

![Raft State Timeline and Terms](images/grafana_raft_state_timeline_and_terms.png)

### Panel A: Raft State Timeline

#### 1. Why it Matters
This panel shows the consensus state of all nodes (Leader, Candidate, or Follower) in real-time. During Chaos Monkey runs, you will see node colors change instantly as followers become candidates and leaders crash.

#### 2. Step-by-Step Configuration
* **Select the Visualization Type**:
  * **Where to find**: Top-right corner of the screen, click the visualization dropdown (it might default to "Time series").
  * **What to do**: Search for and select **State timeline**.
* **Configure the Data Query**:
  * **Where to find**: The **Queries** tab at the bottom-left of the screen.
  * **What to do**:
    1. Make sure your data source is set to **Prometheus**.
    2. Toggle the switch to **Code** mode (on the right of the query options bar) to show the raw query editor.
    3. Enter the query:
       ```promql
       raft_state
       ```
    4. In the query parameters directly below, look for **Legend**. Change it from `{{label_name}}` to:
       ```
       Node {{node_id}}
       ```
       *(This ensures the Y-axis lists neat node labels like "Node 0" instead of full metrics strings).*
* **Configure Panel Title**:
  * **Where to find**: Right-side settings column, expand **Panel options** (the very top section).
  * **What to do**: Under **Title**, enter `Raft State Timeline`.
* **Configure State Values & Colors**:
  * **Where to find**: Right-side settings column, scroll down to the **Value mappings** section.
  * **What to do**: Click **+ Add value mapping** and define these three mappings:
    1. Value: `0` $\rightarrow$ Display text: `Follower` (Color: Dark Grey/Blue)
    2. Value: `1` $\rightarrow$ Display text: `Candidate` (Color: Yellow/Orange)
    3. Value: `2` $\rightarrow$ Display text: `Leader` (Color: Bright Green)
* **Clean Up Values Display**:
  * **Where to find**: Right-side settings column, expand the **State timeline** options section.
  * **What to do**: Set **Show values** to **Never**. *(This hides the numeric digits `0`, `1`, `2` inside the colored blocks, keeping the layout clean).*

---

### Panel B: Current Term per Node

#### 1. Why it Matters
Raft terms are the logical clock of the cluster. When elections take place, terms increase monotonically. Stale terms indicate offline or partitioned nodes that haven't received heartbeat updates.

#### 2. Step-by-Step Configuration
* **Select the Visualization Type**:
  * **Where to find**: Top-right visualization dropdown.
  * **What to do**: Select **Time series**.
* **Configure the Data Query**:
  * **Where to find**: The **Queries** tab at the bottom-left.
  * **What to do**:
    1. Toggle to **Code** mode.
    2. Enter the query:
       ```promql
       raft_current_term
       ```
    3. Under **Legend**, enter:
       ```
       Node {{node_id}}
       ```
* **Configure Panel Title**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Panel options**.
  * **What to do**: Set **Title** to `Current Term per Node`.
* **Configure Y-Axis Formatting**:
  * **Where to find**: Right-side settings column $\rightarrow$ expand **Standard options**.
  * **What to do**: Set **Unit** to **Misc > short** or keep it blank, but under **Decimals**, set it to `0`. *(Terms are strictly whole integers).*

---

## 2. Throughput Layer (Client Operations)

Monitors read and write transactions submitted by the Go CLI or Java client.

![Client Operations Rate](images/grafana_client_operations_rate.png)

### Panel: Client Operations Rate

#### 1. Why it Matters
Visualizes incoming reads and writes. Demonstrates that client operations are continuously succeeding and hitting the leader during active load generation.

#### 2. Step-by-Step Configuration
* **Select the Visualization Type**:
  * **Where to find**: Top-right visualization dropdown.
  * **What to do**: Select **Time series**.
* **Configure the Data Query**:
  * **Where to find**: The **Queries** tab at the bottom-left.
  * **What to do**:
    1. Toggle to **Code** mode.
    2. Enter the query:
       ```promql
       sum(rate(raft_client_requests_total[1m])) by (operation)
       ```
    3. Under **Legend**, enter:
       ```
       {{operation}}
       ```
* **Configure Y-Axis**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Standard options**.
  * **What to do**: Set **Unit** to **Data rate > ops/sec (ops/s)**.
* **Configure Panel Title**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Panel options**.
  * **What to do**: Set **Title** to `Client Operations Rate`.

---

## 3. Replication & Log Telemetry Layer

Tracks log synchronization lag alongside real-time cluster logs.

![Commit Index vs. Last Applied and Loki Log Volume](images/grafana_applied_command_and_loki_logs_volume.png)

### Panel A: Commit Index vs. Last Applied (Replication Lag)

#### 1. Why it Matters
This panel monitors how quickly the state machine is catching up with Raft consensus. A healthy node shows the `CommitIndex` and `LastApplied` indices overlapping perfectly. A wide gap indicates slow disk write speeds or a node recovering from snapshot streams.

#### 2. Step-by-Step Configuration
* **Select the Visualization Type**:
  * **Where to find**: Top-right visualization dropdown.
  * **What to do**: Select **Time series**.
* **Configure Data Queries**:
  * **Where to find**: The **Queries** tab at the bottom-left.
  * **What to do**: Click **+ Add query** so you have two active queries running concurrently:
    1. **Query A**:
       * PromQL Query: `raft_commit_index`
       * Legend: `Node {{node_id}} (Commit)`
    2. **Query B**:
       * PromQL Query: `raft_last_applied`
       * Legend: `Node {{node_id}} (Applied)`
* **Customize Line Styles (Visual separation)**:
  * **Where to find**: Right-side settings column $\rightarrow$ scroll down and click **+ Add override** (under the **Overrides** tab).
  * **What to do**: 
    1. Choose **Fields with name** $\rightarrow$ select `*Applied*`.
    2. Click **+ Add override property** $\rightarrow$ select **Graph styles > Line style**.
    3. Choose **Dashed**.
    *(This renders all "Last Applied" lines as dashed and "Commit" lines as solid, making comparison easy).*
* **Configure Panel Title**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Panel options**.
  * **What to do**: Set **Title** to `Commit Index vs. Last Applied`.

---

### Panel B: Log Volume by Severity

#### 1. Why it Matters
Using LogQL, this counts logs matching severity levels. When chaos events occur, you will see a visual explosion of Warning (yellow) and Error (red) logs, giving immediate visual feedback for crashes.

#### 2. Step-by-Step Configuration
* **Select the Visualization Type**:
  * **Where to find**: Top-right visualization dropdown.
  * **What to do**: Select **Bar chart**.
* **Configure the Data Query**:
  * **Where to find**: The **Queries** tab at the bottom-left.
  * **What to do**:
    1. Change data source dropdown from *Prometheus* to **Loki**.
    2. Toggle to **Code** mode.
    3. Enter the query:
       ```logql
       sum by (level) (count_over_time({job="kvraft_logs"} |~ "(?i)debug|info|warn|error|panic|fatal" | regexp "(?i)(?P<level>debug|info|warn|error|panic|fatal)" [1m]))
       ```
    4. Under **Legend**, enter:
       ```
       {{level}}
       ```
* **Customize Color Mappings**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Overrides** tab.
  * **What to do**: Add overrides to match standard logging severity levels:
    1. **`INFO` / `info`** $\rightarrow$ Green (`#56A64B`)
    2. **`DEBUG` / `debug`** $\rightarrow$ Light Gray/Purple (`#8E8E8E`)
    3. **`WARN` / `warn`** $\rightarrow$ Yellow/Orange (`#FADE2A`)
    4. **`ERROR` / `error`** $\rightarrow$ Bright Red (`#E02F44`)
    5. **`FATAL` / `fatal`** / **`PANIC` / `panic`** $\rightarrow$ Dark Red (`#CF2F74`)
* **Configure Panel Title**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Panel options**.
  * **What to do**: Set **Title** to `Log Volume by Severity`.

---

### Panel C: Live Cluster Logs Stream

#### 1. Why it Matters
A scrolling console showing live cluster logs from all nodes simultaneously, allowing viewers to see nodes detecting leader loss and triggering elections in real time.

#### 2. Step-by-Step Configuration
* **Select the Visualization Type**:
  * **Where to find**: Top-right visualization dropdown.
  * **What to do**: Select **Logs**.
* **Configure the Data Query**:
  * **Where to find**: The **Queries** tab at the bottom-left.
  * **What to do**:
    1. Change data source dropdown from *Prometheus* to **Loki**.
    2. Toggle to **Code** mode.
    3. Enter the query:
       ```logql
       {job="kvraft_logs"}
       ```
* **Configure Panel Title**:
  * **Where to find**: Right-side settings column $\rightarrow$ **Panel options**.
  * **What to do**: Set **Title** to `Live Cluster Logs`.

---

## Global Time Range Guidelines

For the best visual impact during your thesis presentation, control the time range **globally** in the top-right corner of your dashboard:
- **During the Live Demo (5–15 min window)**: Set time range to `Last 15 minutes` or `Last 5 minutes` and auto-refresh to `5s`. This provides a high-resolution, scrolling view of the node role changes and throughput drops.
- **For Long-Term Stability (1–6 hour window)**: Set time range to `Last 1 hour` or `Last 6 hours` with auto-refresh turned off. This shows flat term lines and uniform client operations over long, stable periods.
- **How to scroll and view historical data**:
  * **Shift + Drag**: Hold the `Shift` key and click-and-drag your mouse left or right across the timeline to scroll/pan horizontally.
  * **Zoom**: Click and drag your mouse across a specific region of the timeline to zoom into a detailed window. Double-click the panel background to zoom back out.
  * **Pan buttons**: Use the left/right arrows (`<` and `>`) next to the time range selector at the top-right of the dashboard to step backward or forward in time.
