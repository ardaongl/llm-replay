(() => {
  "use strict";

  const colors = ["#51e1c2", "#76a9ff", "#f7b955", "#c58cff", "#ff7ea2"];
  const embedded = document.getElementById("replay-data");
  const apiMode = !embedded;
  const state = { report: null, records: [], nextCursor: "", selected: null };
  const byId = (id) => document.getElementById(id);

  function element(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = String(text);
    return node;
  }

  async function requestJSON(path) {
    const response = await fetch(path, { headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error((await response.text()).trim() || `Request failed (${response.status})`);
    return response.json();
  }

  async function initialize() {
    try {
      if (apiMode) {
        state.report = await requestJSON("/api/v1/report");
        await loadServerRecords(true);
      } else {
        state.report = JSON.parse(embedded.textContent);
        state.records = state.report.records || [];
      }
      populateModels();
      renderReport();
      bindEvents();
      byId("mode-badge").textContent = apiMode ? "LOCAL UI" : "OFFLINE REPORT";
      setStatus(state.report.truncated ? `${state.report.included_records} of ${state.report.total_records} records included.` : "Report ready.");
    } catch (error) {
      setStatus(error.message || String(error), true);
    }
  }

  function setStatus(message, isError = false) {
    const node = byId("status");
    node.textContent = message;
    node.classList.toggle("error", isError);
  }

  function populateModels() {
    const select = byId("filter-model");
    for (const run of state.report.runs) {
      const option = element("option", "", run.name);
      option.value = run.name;
      select.append(option);
    }
  }

  function renderReport() {
    byId("dataset-hash").textContent = `dataset ${state.report.dataset_sha256.slice(0, 12)}`;
    byId("record-count").textContent = `${state.report.total_records.toLocaleString()} comparable records`;
    renderLeaders();
    renderCards();
    renderLatencyChart();
    renderRecords();
  }

  function renderLeaders() {
    const strip = byId("leader-strip");
    strip.replaceChildren();
    const priced = state.report.runs.filter((run) => run.summary.pricing_available);
    const definitions = [
      ["Highest reliability", state.report.runs, (run) => run.summary.success_rate, Math.max],
      ["Highest JSON validity", state.report.runs.filter((run) => run.summary.evaluations.json_valid_rate != null), (run) => run.summary.evaluations.json_valid_rate, Math.max],
      ["Highest schema adherence", state.report.runs.filter((run) => run.summary.evaluations.schema_adherence_rate != null), (run) => run.summary.evaluations.schema_adherence_rate, Math.max],
      ["Lowest average latency", state.report.runs, (run) => run.summary.latency_avg_ms, Math.min],
      ["Lowest P95", state.report.runs, (run) => run.summary.latency_p95_ms, Math.min],
      ["Lowest cost", priced, (run) => run.summary.estimated_cost_usd, Math.min],
    ];
    for (const [label, runs, value, choose] of definitions) {
      if (!runs.length) continue;
      const target = choose(...runs.map(value));
      const leaders = runs.filter((run) => value(run) === target).map((run) => run.name).join(", ");
      strip.append(element("span", "leader-chip", `${label}: ${leaders}`));
    }
  }

  function renderCards() {
    const grid = byId("model-cards");
    grid.replaceChildren();
    state.report.runs.forEach((run, index) => {
      const card = element("article", "model-card");
      card.style.setProperty("--model-color", colors[index % colors.length]);
      card.append(element("h3", "", run.name));
      const metrics = element("div", "metric-grid");
      metrics.append(metric(percent(run.summary.success_rate), "Success"));
      metrics.append(metric(optionalRate(run.summary.evaluations.json_valid_rate), "Valid JSON"));
      metrics.append(metric(optionalRate(run.summary.evaluations.schema_adherence_rate), "Schema"));
      metrics.append(metric(optionalRate(run.summary.evaluations.exact_match_rate), "Exact match"));
      metrics.append(metric(`${number(run.summary.latency_avg_ms)} ms`, "Average latency"));
      metrics.append(metric(`${number(run.summary.latency_p50_ms)} ms`, "P50 latency"));
      metrics.append(metric(`${number(run.summary.latency_p95_ms)} ms`, "P95 latency"));
      metrics.append(metric(number(run.summary.total_tokens), "Total tokens"));
      metrics.append(metric(run.summary.pricing_available ? money(run.summary.estimated_cost_usd) : "n/a", "Run cost"));
      metrics.append(metric(run.summary.pricing_available ? money(run.summary.average_cost_per_successful_request_usd) : "n/a", "Cost / success"));
      card.append(metrics);
      if (run.summary.pricing_available && run.summary.total_requests > 0) {
        const perRequest = run.summary.estimated_cost_usd / run.summary.total_requests;
        card.append(element("p", "cost-projection", `Linear estimate · 10k ≈ ${money(perRequest * 10000)} · 100k ≈ ${money(perRequest * 100000)} · 1m ≈ ${money(perRequest * 1000000)}`));
      }
      grid.append(card);
    });
  }

  function metric(value, label) {
    const node = element("div", "metric");
    node.append(element("strong", "", value), element("span", "", label));
    return node;
  }

  function renderLatencyChart() {
    const svg = byId("latency-chart");
    const legend = byId("chart-legend");
    svg.replaceChildren();
    legend.replaceChildren();
    const all = state.report.latency.series;
    const maximum = state.report.latency.max_ms;
    const bins = state.report.latency.bins;
    const counts = all.map((series) => series.counts);
    const maxCount = Math.max(1, ...counts.flat());
    const width = 880 / bins;
    const groupWidth = Math.max(3, (width - 8) / Math.max(1, all.length));
    all.forEach((series, seriesIndex) => {
      const item = element("span", "legend-item");
      const dot = element("span", "legend-dot");
      dot.style.background = colors[seriesIndex % colors.length];
      item.append(dot, document.createTextNode(series.name), element("span", "legend-stats", `P50 ${series.p50_ms} · P90 ${series.p90_ms} · P95 ${series.p95_ms} · P99 ${series.p99_ms} ms · ${series.timeout_count} timeout`));
      legend.append(item);
      counts[seriesIndex].forEach((count, bin) => {
        const bar = document.createElementNS("http://www.w3.org/2000/svg", "rect");
        const height = (count / maxCount) * 190;
        bar.setAttribute("x", String(52 + bin * width + 4 + seriesIndex * groupWidth));
        bar.setAttribute("y", String(218 - height));
        bar.setAttribute("width", String(Math.max(2, groupWidth - 2)));
        bar.setAttribute("height", String(height));
        bar.setAttribute("rx", "2");
        bar.setAttribute("fill", colors[seriesIndex % colors.length]);
        bar.setAttribute("opacity", ".84");
        svg.append(bar);
      });
      [["50", series.p50_ms], ["90", series.p90_ms], ["95", series.p95_ms], ["99", series.p99_ms]].forEach(([labelText, latency], markerIndex) => {
        const marker = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        marker.setAttribute("cx", String(52 + (latency / maximum) * 888));
        marker.setAttribute("cy", String(10 + seriesIndex * 18 + markerIndex * 3));
        marker.setAttribute("r", "3");
        marker.setAttribute("fill", colors[seriesIndex % colors.length]);
        marker.setAttribute("aria-label", `${series.name} P${labelText} ${latency} ms`);
        svg.append(marker);
      });
    });
    const axis = document.createElementNS("http://www.w3.org/2000/svg", "line");
    axis.setAttribute("x1", "52"); axis.setAttribute("x2", "940"); axis.setAttribute("y1", "218"); axis.setAttribute("y2", "218"); axis.setAttribute("stroke", "currentColor"); axis.setAttribute("opacity", ".28");
    svg.append(axis);
    for (let index = 0; index <= 4; index++) {
      const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
      label.setAttribute("x", String(52 + (888 * index / 4)));
      label.setAttribute("y", "244");
      label.setAttribute("fill", "currentColor"); label.setAttribute("opacity", ".6"); label.setAttribute("font-size", "12"); label.setAttribute("text-anchor", index === 0 ? "start" : index === 4 ? "end" : "middle");
      label.textContent = `${Math.round(maximum * index / 4)} ms`;
      svg.append(label);
    }
  }

  function renderRecords() {
    const body = byId("results-body");
    body.replaceChildren();
    const records = apiMode ? state.records : localFilteredRecords();
    for (const record of records) {
      const row = element("tr");
      row.tabIndex = 0;
      row.append(element("td", "mono", record.id));
      const models = element("td");
      const tags = element("div", "model-tags");
      record.models.forEach((model) => tags.append(element("span", "tag", labelModel(model.name))));
      models.append(tags); row.append(models);
      const statuses = [...new Set(record.models.map((model) => model.status))];
      const status = statuses.every((value) => value === "success") ? "success" : statuses.join(" · ");
      row.append(element("td", `status-${status === "success" ? "success" : "error"}`, status));
      row.append(element("td", "mono", record.models.map((model) => `${labelModel(model.name)} ${model.metrics.latency_ms}ms`).join(" · ")));
      row.append(element("td", "mono", record.models.map((model) => `${labelModel(model.name)} ${model.metrics.input_tokens}/${model.metrics.output_tokens}`).join(" · ")));
      const qualityCell = element("td");
      const quality = element("div", "quality-tags");
      for (const model of record.models) {
        const failed = (model.evaluations || []).filter((evaluation) => !evaluation.skipped && !evaluation.passed).length;
        quality.append(element("span", `tag ${failed ? "fail" : "pass"}`, `${labelModel(model.name)} ${failed ? `${failed} failed` : "passed"}`));
      }
      qualityCell.append(quality); row.append(qualityCell);
      row.addEventListener("click", () => openRecord(record));
      row.addEventListener("keydown", (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); openRecord(record); } });
      body.append(row);
    }
    byId("empty-state").hidden = records.length !== 0;
    byId("result-total").textContent = `${records.length.toLocaleString()} shown${apiMode ? "" : ` · ${state.report.included_records.toLocaleString()} available`}`;
    byId("load-more").hidden = !apiMode || !state.nextCursor;
  }

  function localFilteredRecords() {
    const query = currentFilters();
    return state.records.filter((record) => {
      if (query.q) {
        const text = [record.id, ...record.request.messages.map((message) => message.content)].join(" ").toLowerCase();
        if (!text.includes(query.q.toLowerCase())) return false;
      }
      return record.models.some((model) => {
        if (query.model && model.name !== query.model) return false;
        if (query.status && model.status !== query.status) return false;
        if (query.min_latency_ms && model.metrics.latency_ms < Number(query.min_latency_ms)) return false;
        if (query.evaluator && !(model.evaluations || []).some((evaluation) => evaluation.name === query.evaluator && !evaluation.passed)) return false;
        return true;
      });
    });
  }

  function currentFilters() {
    return {
      q: byId("filter-search").value.trim(), status: byId("filter-status").value,
      evaluator: byId("filter-evaluator").value, model: byId("filter-model").value,
      min_latency_ms: byId("filter-latency").value,
    };
  }

  async function loadServerRecords(reset) {
    const filters = currentFiltersSafe();
    const parameters = new URLSearchParams({ limit: "200" });
    for (const [key, value] of Object.entries(filters)) if (value && value !== "0") parameters.set(key, value);
    if (filters.evaluator) parameters.set("passed", "false");
    if (!reset && state.nextCursor) parameters.set("cursor", state.nextCursor);
    const page = await requestJSON(`/api/v1/results?${parameters}`);
    state.records = reset ? page.records : state.records.concat(page.records);
    state.nextCursor = page.next_cursor || "";
  }

  function currentFiltersSafe() {
    if (!byId("filter-search")) return {};
    return currentFilters();
  }

  async function refreshRecords() {
    try {
      if (apiMode) await loadServerRecords(true);
      renderLatencyChart();
      renderRecords();
    } catch (error) { setStatus(error.message || String(error), true); }
  }

  async function openRecord(record) {
    try {
      state.selected = apiMode ? await requestJSON(`/api/v1/results/${encodeURIComponent(record.id)}`) : record;
      const selected = state.selected;
      byId("detail-title").textContent = selected.id;
      const modelSelect = byId("detail-model"); modelSelect.replaceChildren();
      selected.models.forEach((model) => { const option = element("option", "", model.name); option.value = model.name; modelSelect.append(option); });
      const prompt = byId("prompt-content"); prompt.replaceChildren();
      selected.request.messages.forEach((message) => {
        const row = element("div", "prompt-message");
        row.append(element("span", "prompt-role", message.role), element("span", "", message.content));
        prompt.append(row);
      });
      renderDetailModel();
      byId("detail-dialog").showModal();
    } catch (error) { setStatus(error.message || String(error), true); }
  }

  function renderDetailModel() {
    if (!state.selected) return;
    const model = state.selected.models.find((item) => item.name === byId("detail-model").value) || state.selected.models[0];
    const baseline = formatContent(state.selected.baseline.content || "");
    const candidate = formatContent(model.candidate?.content || model.error?.message || "No candidate response");
    byId("baseline-content").textContent = baseline;
    byId("candidate-content").textContent = candidate;
    const status = byId("detail-status"); status.textContent = `${model.status} · ${model.metrics.latency_ms} ms`; status.className = `status-pill status-${model.status}`;
    renderDiff(baseline, candidate);
    const evaluations = byId("evaluation-content"); evaluations.replaceChildren();
    if (!(model.evaluations || []).length) evaluations.append(element("p", "muted", "No evaluations recorded."));
    for (const evaluation of model.evaluations || []) {
      const row = element("div", "evaluation-row");
      const outcome = evaluation.skipped ? "skipped" : evaluation.passed ? "passed" : "failed";
      row.append(element("strong", "", evaluation.name), element("span", outcome === "passed" ? "status-success" : outcome === "failed" ? "status-error" : "muted", outcome));
      if (evaluation.error) row.append(element("span", "status-error", evaluation.error));
      if (evaluation.score != null || evaluation.details) {
        const details = element("pre", "evaluation-details");
        details.textContent = JSON.stringify({ score: evaluation.score, details: evaluation.details }, null, 2);
        row.append(details);
      }
      evaluations.append(row);
    }
  }

  function formatContent(value) {
    try { return JSON.stringify(JSON.parse(value), null, 2); } catch (_) { return value; }
  }

  function renderDiff(left, right) {
    const output = byId("diff-content"); output.replaceChildren();
    const note = byId("diff-note"); note.textContent = "";
    if (left.length > 20000 || right.length > 20000) {
      note.textContent = "Diff disabled above 20,000 characters.";
      output.textContent = "Use the baseline and candidate panels above for the full text.";
      return;
    }
    const a = left.split(/(\s+)/); const b = right.split(/(\s+)/);
    if (a.length > 400 || b.length > 400) {
      note.textContent = "Diff simplified for long text.";
      appendSimpleDiff(output, left, right); return;
    }
    const matrix = Array.from({ length: a.length + 1 }, () => new Uint16Array(b.length + 1));
    for (let i = a.length - 1; i >= 0; i--) for (let j = b.length - 1; j >= 0; j--) matrix[i][j] = a[i] === b[j] ? matrix[i + 1][j + 1] + 1 : Math.max(matrix[i + 1][j], matrix[i][j + 1]);
    let i = 0; let j = 0;
    while (i < a.length || j < b.length) {
      if (i < a.length && j < b.length && a[i] === b[j]) { output.append(document.createTextNode(a[i])); i++; j++; }
      else if (j < b.length && (i === a.length || matrix[i][j + 1] >= matrix[i + 1][j])) { output.append(element("span", "diff-add", b[j++])); }
      else { output.append(element("span", "diff-remove", a[i++])); }
    }
  }

  function appendSimpleDiff(output, left, right) {
    let prefix = 0; while (prefix < left.length && prefix < right.length && left[prefix] === right[prefix]) prefix++;
    let suffix = 0; while (suffix < left.length - prefix && suffix < right.length - prefix && left[left.length - 1 - suffix] === right[right.length - 1 - suffix]) suffix++;
    output.append(document.createTextNode(left.slice(0, prefix)));
    output.append(element("span", "diff-remove", left.slice(prefix, left.length - suffix)));
    output.append(element("span", "diff-add", right.slice(prefix, right.length - suffix)));
    if (suffix) output.append(document.createTextNode(left.slice(left.length - suffix)));
  }

  function bindEvents() {
    let timer;
    byId("filter-search").addEventListener("input", () => { clearTimeout(timer); timer = setTimeout(refreshRecords, 180); });
    ["filter-status", "filter-evaluator", "filter-model", "filter-latency"].forEach((id) => byId(id).addEventListener("change", refreshRecords));
    byId("filters").addEventListener("submit", (event) => event.preventDefault());
    byId("load-more").addEventListener("click", async () => { await loadServerRecords(false); renderLatencyChart(); renderRecords(); });
    byId("close-detail").addEventListener("click", () => byId("detail-dialog").close());
    byId("detail-model").addEventListener("change", renderDetailModel);
    byId("detail-dialog").addEventListener("click", (event) => { if (event.target === byId("detail-dialog")) byId("detail-dialog").close(); });
  }

  function percent(value) { return `${(Number(value || 0) * 100).toFixed(1)}%`; }
  function optionalRate(value) { return value === undefined || value === null ? "n/a" : percent(value); }
  function number(value) { return Number(value || 0).toLocaleString(); }
  function money(value) { return `$${Number(value || 0).toFixed(6)}`; }
  function labelModel(value) { return value.replace("/", " · "); }

  initialize();
})();
