const state = {
  issues: [],
  runs: [],
  workOrders: [],
  pipelineBoard: [],
  pipelines: [],
  testingPolicies: [],
  stageCatalog: [],
  selectedRunId: null,
  selectedStage: null,
  activeTab: "operations",
  lastDetail: null,
  lastMermaidSignature: null,
};

const els = {
  issuesList: document.getElementById("issues-list"),
  runsList: document.getElementById("runs-list"),
  issuesCount: document.getElementById("issues-count"),
  runsCount: document.getElementById("runs-count"),
  workordersCount: document.getElementById("workorders-count"),
  pipelineBoardCount: document.getElementById("pipeline-board-count"),
  pipelinesCount: document.getElementById("pipelines-count"),
  stagesCount: document.getElementById("stages-count"),
  importIssue: document.getElementById("import-issue"),
  issueJson: document.getElementById("issue-json"),
  importStatus: document.getElementById("import-status"),
  refreshIssues: document.getElementById("refresh-issues"),
  reconcileAll: document.getElementById("reconcile-all"),
  recoverRuns: document.getElementById("recover-runs"),
  detailTitle: document.getElementById("detail-title"),
  detailStatus: document.getElementById("detail-status"),
  detailEmpty: document.getElementById("detail-empty"),
  detailContent: document.getElementById("detail-content"),
  detailRunId: document.getElementById("detail-run-id"),
  detailIssueId: document.getElementById("detail-issue-id"),
  detailUpdated: document.getElementById("detail-updated"),
  detailPipelineFamily: document.getElementById("detail-pipeline-family"),
  detailPipelineSelection: document.getElementById("detail-pipeline-selection"),
  detailIssueType: document.getElementById("detail-issue-type"),
  stageGrid: document.getElementById("stage-grid"),
  pipelineMermaid: document.getElementById("pipeline-mermaid"),
  pipelineShape: document.getElementById("pipeline-shape"),
  policyEvaluation: document.getElementById("policy-evaluation"),
  runIndex: document.getElementById("run-index"),
  stageReport: document.getElementById("stage-report"),
  runStats: document.getElementById("run-stats"),
  stageStats: document.getElementById("stage-stats"),
  workordersList: document.getElementById("workorders-list"),
  pipelineBoard: document.getElementById("pipeline-board"),
  pipelineList: document.getElementById("pipeline-list"),
  stageCatalogList: document.getElementById("stage-catalog-list"),
  projectionGraph: document.getElementById("projection-graph"),
  tabs: Array.from(document.querySelectorAll(".tab")),
  tabPanels: Array.from(document.querySelectorAll(".tab-panel")),
};

async function request(path, options = {}) {
  const response = await fetch(path, {
    headers: {"Content-Type": "application/json"},
    ...options,
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `${response.status} ${response.statusText}`);
  }
  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    return response.json();
  }
  return null;
}

function statusClass(status) {
  return ["pending", "active", "completed", "failed", "awaiting_approval"].includes(status) ? status : "neutral";
}

function formatDate(value) {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

function chooseRun() {
  if (!state.selectedRunId && state.runs.length) {
    state.selectedRunId = state.runs[0].id;
  }
  return state.runs.find((run) => run.id === state.selectedRunId) || null;
}

function renderIssues() {
  els.issuesCount.textContent = String(state.issues.length);
  els.issuesList.innerHTML = "";
  for (const issue of state.issues) {
    const card = document.createElement("article");
    card.className = "card";
    const latest = issue.latest_run;
    card.innerHTML = `
      <div class="title">${issue.title || issue.id}</div>
      <div class="meta">
        <span>${issue.id}</span>
        <span>${issue.issue_type || "unknown"}</span>
        <span>${formatDate(issue.updated_at)}</span>
      </div>
      <div class="meta">${(issue.labels || []).join(" · ") || "no labels"}</div>
      <div class="actions">
        <button class="mini materialize">Materialize</button>
        <button class="mini reconcile">Reconcile</button>
        ${latest ? `<button class="mini select-run">Open Run</button>` : ""}
      </div>
    `;
    card.querySelector(".materialize").onclick = async () => {
      await request(`/api/v1/issues/${encodeURIComponent(issue.id)}/materialize`, {method: "POST"});
      await refresh();
    };
    card.querySelector(".reconcile").onclick = async () => {
      await request(`/api/v1/issues/${encodeURIComponent(issue.id)}/reconcile`, {method: "POST"});
      await refresh();
    };
    if (latest) {
      card.querySelector(".select-run").onclick = () => {
        state.selectedRunId = latest.id;
        state.selectedStage = null;
        state.activeTab = "run-detail";
        render();
        loadRunDetail();
      };
    }
    els.issuesList.appendChild(card);
  }
}

function renderRuns() {
  els.runsCount.textContent = String(state.runs.length);
  els.runsList.innerHTML = "";
  for (const run of state.runs) {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="title">${run.id}</div>
      <div class="meta">
        <span>${run.issue_id}</span>
        <span>${run.issue_type || "unknown"}</span>
      </div>
      <div class="meta">
        <span class="pill ${statusClass(run.status)}">${run.status}</span>
        <span>${formatDate(run.updated_at)}</span>
      </div>
    `;
    card.onclick = () => {
      state.selectedRunId = run.id;
      state.selectedStage = null;
      state.activeTab = "run-detail";
      render();
      loadRunDetail();
    };
    els.runsList.appendChild(card);
  }
}

function renderWorkOrders() {
  els.workordersCount.textContent = String(state.workOrders.length);
  els.workordersList.innerHTML = "";
  for (const workOrder of state.workOrders) {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="title">${workOrder.id}</div>
      <div class="meta">
        <span>${workOrder.issue_type || "unknown"}</span>
        <span>${workOrder.pipeline_family || "unassigned"}</span>
      </div>
      <div class="meta">${(workOrder.run_ids || []).join(" · ") || "no runs"}</div>
      <div class="muted">${workOrder.requested_outcome || "No requested outcome."}</div>
    `;
    els.workordersList.appendChild(card);
  }
}

function renderPipelineBoard() {
  els.pipelineBoardCount.textContent = String(state.pipelineBoard.length);
  els.pipelineBoard.innerHTML = "";
  for (const group of state.pipelineBoard) {
    const block = document.createElement("section");
    block.className = "board-family";
    block.innerHTML = `
      <div class="board-family-head">
        <h3>${group.pipeline_family}</h3>
        <span class="count">${group.active_runs}</span>
      </div>
    `;
    const lane = document.createElement("div");
    lane.className = "board-lane";
    for (const run of group.runs || []) {
      const chip = document.createElement("button");
      chip.className = `board-chip ${statusClass(run.status)}`;
      chip.textContent = `${run.run_id} · ${run.current_stage || "pending"}`;
      chip.onclick = () => {
        state.selectedRunId = run.run_id;
        state.selectedStage = null;
        state.activeTab = "run-detail";
        render();
        loadRunDetail();
      };
      lane.appendChild(chip);
    }
    block.appendChild(lane);
    els.pipelineBoard.appendChild(block);
  }
}

function renderPipelines() {
  els.pipelinesCount.textContent = String(state.pipelines.length);
  els.pipelineList.innerHTML = "";
  for (const pipeline of state.pipelines) {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="title">${pipeline.name}</div>
      <div class="meta">
        <span>${pipeline.family || "unknown"}</span>
        <span>${pipeline.testing_policy || "no testing policy"}</span>
      </div>
      <div class="meta">${(pipeline.accepted_issue_types || []).join(" · ") || "no issue types"}</div>
      <div class="meta">${(pipeline.optimization_goals || []).join(" · ") || "no optimization goals"}</div>
      <div class="muted">${pipeline.summary || "No summary."}</div>
    `;
    els.pipelineList.appendChild(card);
  }
}

function renderStageCatalog() {
  els.stagesCount.textContent = String(state.stageCatalog.length);
  els.stageCatalogList.innerHTML = "";
  for (const spec of state.stageCatalog) {
    const card = document.createElement("article");
    card.className = "card";
    card.innerHTML = `
      <div class="title">${spec.name}</div>
      <div class="meta">
        <span>${spec.queue_mode || "queue unset"}</span>
        <span>${spec.environment || "no env"}</span>
      </div>
      <div class="meta">
        <span>run_as: ${spec.run_as || "unset"}</span>
        <span>write_as: ${spec.write_as || "unset"}</span>
      </div>
      <div class="meta">deps: ${(spec.dependencies || []).join(", ") || "none"}</div>
      <div class="meta">surfaces: ${(spec.materialize || []).join(", ") || "none"}</div>
    `;
    els.stageCatalogList.appendChild(card);
  }
}

async function loadRunDetail() {
  const run = chooseRun();
  if (!run) {
    if (state.lastDetail) {
      renderDetail(
        state.lastDetail.run,
        state.lastDetail.index,
        state.lastDetail.report,
        state.lastDetail.executionPlan,
        state.lastDetail.policyEvaluation,
      );
    } else {
      renderDetail(null, null, null);
    }
    return;
  }
  const index = await request(`/api/v1/runs/${encodeURIComponent(run.id)}/index`);
  let report = null;
  const stageStates = ((run.metadata || {}).current_stage_states) || {};
  const stageNames = Object.keys(stageStates);
  if (!state.selectedStage && stageNames.length) {
    state.selectedStage = stageNames[0];
  }
  let executionPlan = null;
  let policyEvaluation = null;
  try {
    executionPlan = await request(`/api/v1/runs/${encodeURIComponent(run.id)}/pipeline/pipeline_execution_plan`);
  } catch {
    executionPlan = null;
  }
  try {
    policyEvaluation = await request(`/api/v1/runs/${encodeURIComponent(run.id)}/pipeline/policy_evaluation`);
  } catch {
    policyEvaluation = null;
  }
  if (state.selectedStage) {
    try {
      report = await request(`/api/v1/runs/${encodeURIComponent(run.id)}/stages/${encodeURIComponent(state.selectedStage)}/report`);
    } catch {
      report = null;
    }
  }
  state.lastDetail = { run, index, report, executionPlan, policyEvaluation };
  renderDetail(run, index, report, executionPlan, policyEvaluation);
}

function renderDetail(run, index, report, executionPlan, policyEvaluation) {
  if (!run) {
    els.detailEmpty.hidden = false;
    els.detailEmpty.style.display = "grid";
    els.detailContent.hidden = true;
    els.detailContent.style.display = "none";
    return;
  }
  els.detailEmpty.hidden = true;
  els.detailEmpty.style.display = "none";
  els.detailContent.hidden = false;
  els.detailContent.style.display = "block";
  els.detailTitle.textContent = run.id;
  els.detailStatus.className = `pill ${statusClass(run.status)}`;
  els.detailStatus.textContent = run.status;
  els.detailRunId.textContent = run.id;
  els.detailIssueId.textContent = run.issue_id;
  els.detailUpdated.textContent = formatDate(run.updated_at);
  els.detailPipelineFamily.textContent = run.pipeline_family || "unknown";
  els.detailPipelineSelection.textContent = (((executionPlan || {}).pipeline_selection) || "unknown");
  els.detailIssueType.textContent = run.issue_type || "unknown";

  const stageStates = ((run.metadata || {}).current_stage_states) || {};
  const stageEntries = Object.entries(stageStates).sort(([a], [b]) => a.localeCompare(b));
  els.stageGrid.innerHTML = "";
  for (const [stage, payload] of stageEntries) {
    const runtimeSummary = payload.runtime_summary || payload.summary || "No summary yet";
    const status = payload.status || "pending";
    const card = document.createElement("article");
    card.className = "stage-card";
    card.innerHTML = `
      <h4>${stage}</h4>
      <p>${runtimeSummary}</p>
      <button class="mini ${statusClass(status)}">${status}</button>
    `;
    card.onclick = async () => {
      state.selectedStage = stage;
      const nextReport = await request(`/api/v1/runs/${encodeURIComponent(run.id)}/stages/${encodeURIComponent(stage)}/report`);
      renderDetail(run, index, nextReport, executionPlan, policyEvaluation);
    };
    els.stageGrid.appendChild(card);
  }
  els.runStats.textContent = JSON.stringify(run.stats || {}, null, 2);
  els.stageStats.textContent = JSON.stringify((report || {}).stats || {}, null, 2);
  renderPipelineShape(executionPlan, stageStates);
  renderMermaidDiagram(executionPlan, stageStates);
  els.policyEvaluation.textContent = JSON.stringify(policyEvaluation || {}, null, 2);
  els.runIndex.textContent = JSON.stringify(index || {}, null, 2);
  els.stageReport.textContent = JSON.stringify(report || {}, null, 2);
}

function renderPipelineShape(executionPlan, stageStates) {
  const stages = (executionPlan && executionPlan.stages) || [];
  if (!stages.length) {
    els.pipelineShape.innerHTML = `<div class="muted">No materialized pipeline execution plan found for this run.</div>`;
    return;
  }
  const failed = Object.entries(stageStates || {}).find(([, payload]) => payload.status === "failed");
  if (failed) {
    const [stage, payload] = failed;
    els.pipelineShape.innerHTML = `<div class="muted">Hover a stage for details. Current failure: <strong>${stage}</strong> — ${payload.summary || "No summary available."}</div>`;
    return;
  }
  els.pipelineShape.innerHTML = `<div class="muted">Hover a stage node for execution details. Green stages succeeded, red stages failed, teal stages are active, and muted stages have not executed yet.</div>`;
}

function render() { renderProjection();
  renderTabs();
  renderIssues();
  renderRuns();
  renderWorkOrders();
  renderPipelineBoard();
  renderPipelines();
  renderStageCatalog();
}

function renderTabs() {
  for (const tab of els.tabs) {
    tab.classList.toggle("active", tab.dataset.tab === state.activeTab);
  }
  for (const panel of els.tabPanels) {
    panel.classList.toggle("active", panel.dataset.panel === state.activeTab);
  }
}

async function refresh() {
  const [payload, pipelineCatalog, stageCatalog] = await Promise.all([
    request("/api/v1/overview"),
    request("/api/v1/pipelines"),
    request("/api/v1/stages"),
  ]);
  state.issues = payload.issues || [];
  state.runs = payload.runs || [];
  state.workOrders = payload.work_orders || [];
  state.pipelineBoard = payload.pipeline_board || [];
  state.pipelines = pipelineCatalog.pipelines || [];
  state.testingPolicies = pipelineCatalog.testing || [];
  state.stageCatalog = stageCatalog.stages || [];
  render();
  await loadRunDetail();
}

let mermaidModulePromise = null;

async function mermaidModule() {
  if (!mermaidModulePromise) {
    mermaidModulePromise = import("https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs")
      .then((mod) => {
        mod.default.initialize({
          startOnLoad: false,
          securityLevel: "loose",
          theme: "neutral",
          themeVariables: {
            fontSize: "18px",
            primaryColor: "#fbf8f1",
            primaryBorderColor: "#0d7c66",
            lineColor: "#11231c",
            tertiaryColor: "#eef5f2",
          },
          flowchart: {
            nodeSpacing: 80,
            rankSpacing: 120,
            curve: "basis",
          },
        });
        return mod.default;
      })
      .catch(() => null);
  }
  return mermaidModulePromise;
}

function stageStatusMap(executionPlan, stageStates) {
  const statuses = {};
  for (const stage of (executionPlan?.stages || [])) {
    statuses[stage.name] = "pending";
  }
  for (const [name, payload] of Object.entries(stageStates || {})) {
    statuses[name] = payload.status || "pending";
  }
  return statuses;
}

function mermaidSourceForPlan(executionPlan, stageStates) {
  const stages = (executionPlan && executionPlan.stages) || [];
  const stageMap = new Map(stages.map((stage) => [stage.name, stage]));
  const statuses = stageStatusMap(executionPlan, stageStates);
  const lines = [
    "flowchart LR",
    "classDef status-succeeded fill:#eef8f2,stroke:#1e8f52,stroke-width:3px,color:#11231c;",
    "classDef status-failed fill:#fff1f1,stroke:#b33a3a,stroke-width:3px,color:#11231c;",
    "classDef status-active fill:#eef5f2,stroke:#0d7c66,stroke-width:3px,color:#11231c;",
    "classDef status-awaiting_approval fill:#fff6e8,stroke:#c2841a,stroke-width:3px,color:#11231c;",
    "classDef status-pending fill:#fbf8f1,stroke:#8ba39a,stroke-width:2px,color:#11231c;",
    "classDef status-neutral fill:#fbf8f1,stroke:#8ba39a,stroke-width:2px,color:#11231c;",
  ];
  for (const stage of stages) {
    const label = `${stage.name} | ${stage.environment || "global"} | ${stage.queue_mode || "auto"}`;
    lines.push(`  ${stage.name}["${label}"]`);
    lines.push(`  class ${stage.name} status-${statusClass(statuses[stage.name])};`);
  }

  const primaryEdges = primaryExecutionEdges(stages);
  const primaryEdgeSet = new Set(primaryEdges.map(([from, to]) => `${from}->${to}`));
  const edges = [];

  for (const [from, to] of primaryEdges) {
    edges.push({ from, to, primary: true });
  }

  for (const stage of stages) {
    for (const dep of stage.dependencies || []) {
      if (!stageMap.has(dep)) continue;
      const key = `${dep}->${stage.name}`;
      if (primaryEdgeSet.has(key)) continue;
      edges.push({ from: dep, to: stage.name, primary: false });
    }
  }

  edges.forEach((edge, index) => {
    const arrow = edge.primary ? "-->" : "-.->";
    lines.push(`  ${edge.from} ${arrow} ${edge.to}`);
    if (!edge.primary) {
      lines.push(`  linkStyle ${index} stroke:#8ba39a,stroke-width:2px;`);
      return;
    }
    const targetStatus = statuses[edge.to];
    const style = targetStatus === "failed"
      ? "stroke:#b33a3a,stroke-width:4px;"
      : "stroke:#1e8f52,stroke-width:4px;";
    lines.push(`  linkStyle ${index} ${style}`);
  });
  return lines.join("\n");
}

function primaryExecutionEdges(stages) {
  if (!stages.length) {
    return [];
  }

  const children = new Map();
  const indegree = new Map();
  for (const stage of stages) {
    children.set(stage.name, []);
    indegree.set(stage.name, (stage.dependencies || []).length);
  }
  for (const stage of stages) {
    for (const dep of stage.dependencies || []) {
      if (!children.has(dep)) continue;
      children.get(dep).push(stage.name);
    }
  }

  const roots = stages
    .filter((stage) => (stage.dependencies || []).length === 0)
    .map((stage) => stage.name)
    .sort();
  const queue = [...roots];
  const topo = [];
  while (queue.length) {
    queue.sort();
    const name = queue.shift();
    topo.push(name);
    for (const child of children.get(name) || []) {
      indegree.set(child, indegree.get(child) - 1);
      if (indegree.get(child) === 0) {
        queue.push(child);
      }
    }
  }

  const stageMap = new Map(stages.map((stage) => [stage.name, stage]));
  const bestDistance = new Map();
  const bestPrev = new Map();
  for (const name of topo) {
    const stage = stageMap.get(name);
    const deps = (stage.dependencies || []).filter((dep) => stageMap.has(dep));
    if (!deps.length) {
      bestDistance.set(name, 1);
      bestPrev.set(name, null);
      continue;
    }
    const ranked = deps
      .map((dep) => [dep, bestDistance.get(dep) || 0])
      .sort((a, b) => {
        if (b[1] !== a[1]) return b[1] - a[1];
        return a[0].localeCompare(b[0]);
      });
    bestPrev.set(name, ranked[0][0]);
    bestDistance.set(name, ranked[0][1] + 1);
  }

  const sink = [...bestDistance.entries()]
    .sort((a, b) => {
      if (b[1] !== a[1]) return b[1] - a[1];
      return a[0].localeCompare(b[0]);
    })[0]?.[0];
  if (!sink) {
    return [];
  }

  const path = [];
  let current = sink;
  while (current) {
    path.push(current);
    current = bestPrev.get(current);
  }
  path.reverse();

  const edges = [];
  for (let i = 0; i < path.length - 1; i += 1) {
    edges.push([path[i], path[i + 1]]);
  }
  return edges;
}

function buildStageTooltip(stage, payload = {}) {
  const deps = (stage.dependencies || []).join(", ") || "none";
  const reports = (stage.report_stages || []).join(", ") || "none";
  const outputs = ((stage.success_criteria || {}).required_outputs || []).join(", ") || "none";
  const image = stage.image_ref
    ? `${stage.image_ref}@${stage.image_digest || "unresolved"}`
    : (stage.image_digest || "unresolved");
  return [
    stage.name,
    `status: ${payload.status || "pending"}`,
    `summary: ${payload.summary || "No summary yet"}`,
    `env: ${stage.environment || "global"}`,
    `run_as: ${stage.run_as || "unset"}`,
    `write_as: ${stage.write_as || "unset"}`,
    `depends on: ${deps}`,
    `report inputs: ${reports}`,
    `required outputs: ${outputs}`,
    `image: ${image}`,
  ].join("\n");
}

function attachStageTooltips(executionPlan, stageStates) {
  const svg = els.pipelineMermaid.querySelector("svg");
  if (!svg) return;
  const stages = executionPlan?.stages || [];
  const stageStateMap = stageStates || {};
  const nodeGroups = Array.from(svg.querySelectorAll("g.node"));
  for (const stage of stages) {
    const node = nodeGroups.find((group) => {
      const label = (group.textContent || "").trim().toLowerCase();
      return label.startsWith(stage.name.toLowerCase());
    });
    if (!node) continue;
    const title = document.createElementNS("http://www.w3.org/2000/svg", "title");
    title.textContent = buildStageTooltip(stage, stageStateMap[stage.name]);
    node.appendChild(title);
  }
}

async function renderMermaidDiagram(executionPlan, stageStates) {
  const source = mermaidSourceForPlan(executionPlan, stageStates);
  const signature = JSON.stringify({
    source,
    statuses: stageStatusMap(executionPlan, stageStates),
  });
  if (!source || !executionPlan || !(executionPlan.stages || []).length) {
    state.lastMermaidSignature = null;
    els.pipelineMermaid.classList.remove("is-loading");
    els.pipelineMermaid.innerHTML = `<div class="muted">No diagram available.</div>`;
    return;
  }
  if (state.lastMermaidSignature === signature && els.pipelineMermaid.innerHTML.trim()) {
    attachStageTooltips(executionPlan, stageStates);
    return;
  }
  els.pipelineMermaid.classList.add("is-loading");
  const mermaid = await mermaidModule();
  if (!mermaid) {
    state.lastMermaidSignature = signature;
    els.pipelineMermaid.classList.remove("is-loading");
    els.pipelineMermaid.innerHTML = `<pre class="code">${source}</pre>`;
    return;
  }
  try {
    const id = `mermaid-${Date.now()}`;
    const { svg } = await mermaid.render(id, source);
    if (!svg || !svg.trim()) {
      state.lastMermaidSignature = signature;
      els.pipelineMermaid.classList.remove("is-loading");
      els.pipelineMermaid.innerHTML = `<pre class="code">${source}</pre>`;
      return;
    }
    state.lastMermaidSignature = signature;
    els.pipelineMermaid.classList.remove("is-loading");
    els.pipelineMermaid.innerHTML = svg;
    attachStageTooltips(executionPlan, stageStates);
  } catch (error) {
    state.lastMermaidSignature = signature;
    els.pipelineMermaid.classList.remove("is-loading");
    const message = (error && error.message) ? error.message : String(error);
    els.pipelineMermaid.innerHTML = `
      <div class="muted">Mermaid render failed; showing raw source instead.</div>
      <pre class="code">${message}\n\n${source}</pre>
    `;
  }
}

els.refreshIssues.onclick = async () => {
  await request("/api/enqueue", {method: "POST"});
  await refresh();
};

els.reconcileAll.onclick = async () => {
  await request("/api/reconcile", {method: "POST"});
  await refresh();
};

els.recoverRuns.onclick = async () => {
  await request("/api/recover", {method: "POST"});
  await refresh();
};

els.importIssue.onclick = async () => {
  els.importStatus.textContent = "Importing…";
  try {
    const payload = JSON.parse(els.issueJson.value);
    await request("/api/v1/issues", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    els.importStatus.textContent = "Imported";
    await refresh();
  } catch (error) {
    els.importStatus.textContent = error.message;
  }
};

for (const tab of els.tabs) {
  tab.onclick = () => {
    state.activeTab = tab.dataset.tab;
    renderTabs();
  };
}

refresh();
setInterval(refresh, 2000);

async function renderProjection() {
  if (state.activeTab !== "projection") return;
  try {
    const projection = await request("/api/v1/projection?goal=package");
    els.projectionGraph.innerHTML = `<pre class="code">${JSON.stringify(projection, null, 2)}</pre>`;
  } catch (error) {
    els.projectionGraph.innerHTML = `<div class="error">Failed to load graph: ${error.message}</div>`;
  }
}
