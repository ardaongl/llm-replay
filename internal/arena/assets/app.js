"use strict";

const state = { config: null, comparison: null, cards: [] };
const csrf = document.querySelector('meta[name="csrf-token"]').content;
const form = document.querySelector("#arena-form");
const compareButton = document.querySelector("#compare-button");
const formError = document.querySelector("#form-error");
const runStatus = document.querySelector("#run-status");
const modelGrid = document.querySelector("#model-grid");
const summaryPanel = document.querySelector("#comparison-summary");
const savePanel = document.querySelector("#save-panel");
const saveDialog = document.querySelector("#save-dialog");
const modelLeft = document.querySelector("#model-left");
const modelRight = document.querySelector("#model-right");

function textNode(tag, className, value) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  node.textContent = value;
  return node;
}

function catalogModel(id) {
  return state.config?.models.find((model) => model.id === id);
}

function selectionReady() {
  const left = catalogModel(modelLeft.value);
  const right = catalogModel(modelRight.value);
  return Boolean(left?.available && right?.available && left.id !== right.id);
}

function syncCards() {
  [modelLeft.value, modelRight.value].forEach((id, index) => {
    const model = catalogModel(id);
    const card = state.cards[index];
    if (!model || !card) return;
    card.querySelector(".provider-name").textContent = model.provider;
    card.querySelector(".model-name").textContent = model.id;
    const availability = card.querySelector(".availability");
    availability.textContent = model.available ? "READY" : "KEY MISSING";
    availability.className = `availability ${model.available ? "ok" : "missing"}`;
    card.querySelector(".response-copy").textContent = "Awaiting a prompt.";
    card.querySelector(".response-copy").className = "response-copy";
    card.querySelector(".evaluation-list").replaceChildren();
    card.querySelectorAll("[data-metric]").forEach((metric) => { metric.textContent = "—"; });
  });
  compareButton.disabled = !selectionReady();
  if (modelLeft.value && modelLeft.value === modelRight.value) {
    formError.textContent = "Choose two different models to compare.";
    formError.hidden = false;
  }
}

function renderModels(models) {
  const template = document.querySelector("#model-template");
  const options = models.map((model) => {
    const option = document.createElement("option");
    option.value = model.id;
    option.textContent = `${model.provider.toUpperCase()} · ${model.id}${model.available ? "" : " · key missing"}`;
    option.disabled = !model.available;
    return option;
  });
  modelLeft.replaceChildren(...options.map((option) => option.cloneNode(true)));
  modelRight.replaceChildren(...options.map((option) => option.cloneNode(true)));
  const available = models.filter((model) => model.available);
  const left = available[0] || models[0];
  const right = available.find((model) => model.provider !== left?.provider) || available[1] || models[1];
  if (left) modelLeft.value = left.id;
  if (right) modelRight.value = right.id;

  state.cards = [0, 1].map(() => {
    const fragment = template.content.cloneNode(true);
    const card = fragment.querySelector(".model-card");
    modelGrid.append(fragment);
    return modelGrid.lastElementChild;
  });
  [modelLeft, modelRight].forEach((select) => select.addEventListener("change", () => {
    state.comparison = null;
    summaryPanel.hidden = true;
    savePanel.hidden = true;
    formError.hidden = true;
    syncCards();
  }));
  syncCards();
}

function setLoading(loading) {
  compareButton.disabled = loading || !selectionReady();
  if (loading) {
    runStatus.textContent = "Running both models…";
    runStatus.className = "run-status busy";
    state.cards.forEach((card) => {
      const copy = card.querySelector(".response-copy");
      copy.textContent = "Waiting for provider response…";
      copy.className = "response-copy loading";
      card.querySelector(".evaluation-list").replaceChildren();
      card.querySelectorAll("[data-metric]").forEach((metric) => { metric.textContent = "—"; });
      const availability = card.querySelector(".availability");
      availability.textContent = "RUNNING";
      availability.className = "availability";
    });
  }
}

function formatCost(value) {
  return typeof value === "number" ? `$${value.toFixed(value < 0.01 ? 6 : 4)}` : "unknown";
}

function prettyContent(value) {
  try { return JSON.stringify(JSON.parse(value), null, 2); } catch { return value; }
}

function renderResult(result, card) {
  const response = card.querySelector(".response-copy");
  if (result.status === "success" && result.response) {
    response.textContent = prettyContent(result.response.content) || "(empty response)";
    response.className = "response-copy";
  } else {
    response.textContent = result.error?.message || `Request ended with status: ${result.status}`;
    response.className = "response-copy error";
  }
  card.querySelector('[data-metric="latency"]').textContent = `${result.metrics?.latency_ms ?? 0} ms`;
  const inputTokens = result.metrics?.input_tokens ?? 0;
  const outputTokens = result.metrics?.output_tokens ?? 0;
  card.querySelector('[data-metric="tokens"]').textContent = inputTokens || outputTokens ? `${inputTokens} in · ${outputTokens} out` : "—";
  card.querySelector('[data-metric="cost"]').textContent = formatCost(result.estimated_cost_usd);
  const evaluationList = card.querySelector(".evaluation-list");
  const chips = (result.evaluations || []).map((evaluation) => {
    const passed = evaluation.passed === true;
    return textNode("span", `eval-chip ${passed ? "pass" : "fail"}`, `${passed ? "✓" : "×"} ${evaluation.name}`);
  });
  evaluationList.replaceChildren(...chips);
  const availability = card.querySelector(".availability");
  availability.textContent = result.status === "success" ? "COMPLETE" : result.status.toUpperCase();
  availability.className = `availability ${result.status === "success" ? "ok" : "error"}`;
}

function renderSummary(results) {
  const metrics = document.querySelector("#summary-metrics");
  const successful = results.filter((result) => result.status === "success");
  const values = [];
  if (successful.length === 2) {
    const [left, right] = successful;
    const winner = left.metrics.latency_ms <= right.metrics.latency_ms ? left : right;
    const slower = winner === left ? right : left;
    const delta = slower.metrics.latency_ms > 0 ? Math.round((1 - winner.metrics.latency_ms / slower.metrics.latency_ms) * 100) : 0;
    if (slower.metrics.latency_ms > 0) values.push(["Faster response", winner.model], ["Latency advantage", `${delta}%`]);
    else values.push(["Successful responses", "2 / 2"]);
  } else {
    values.push(["Successful responses", `${successful.length} / 2`]);
  }
  values.push(["Arena result", state.comparison.arena_result_id]);
  metrics.replaceChildren(...values.map(([label, value]) => {
    const box = document.createElement("div");
    box.append(textNode("span", "", label), textNode("strong", "", value));
    return box;
  }));
  renderDiff(successful);
  summaryPanel.hidden = false;
}

function renderDiff(successful) {
  const output = document.querySelector("#diff-output");
  output.replaceChildren();
  if (successful.length !== 2) {
    output.textContent = "A diff requires two successful responses.";
    return;
  }
  const left = successful[0].response.content;
  const right = successful[1].response.content;
  if (left.length + right.length > 20000) {
    output.textContent = "Diff omitted because the combined response is larger than 20,000 characters.";
    return;
  }
  const a = left.split(/(\s+)/).slice(0, 400);
  const b = right.split(/(\s+)/).slice(0, 400);
  const rows = Array.from({ length: a.length + 1 }, () => new Uint16Array(b.length + 1));
  for (let i = 1; i <= a.length; i += 1) {
    for (let j = 1; j <= b.length; j += 1) rows[i][j] = a[i - 1] === b[j - 1] ? rows[i - 1][j - 1] + 1 : Math.max(rows[i - 1][j], rows[i][j - 1]);
  }
  const parts = [];
  let i = a.length, j = b.length;
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && a[i - 1] === b[j - 1]) { parts.push(["", a[--i]]); j -= 1; }
    else if (j > 0 && (i === 0 || rows[i][j - 1] >= rows[i - 1][j])) parts.push(["diff-add", b[--j]]);
    else parts.push(["diff-remove", a[--i]]);
  }
  output.replaceChildren(...parts.reverse().map(([className, value]) => textNode("span", className, value)));
}

function renderSave(results) {
  if (!state.config.save_enabled) return;
  const successful = results.filter((result) => result.status === "success");
  const select = document.querySelector("#baseline-model");
  select.replaceChildren(...successful.map((result) => {
    const option = document.createElement("option");
    option.value = result.model;
    option.textContent = result.model;
    return option;
  }));
  savePanel.hidden = successful.length === 0;
}

async function requestJSON(path, options = {}) {
  const response = await fetch(path, options);
  let payload;
  try { payload = await response.json(); } catch { payload = {}; }
  if (!response.ok) throw new Error(payload.error || `Request failed (${response.status})`);
  return payload;
}

function comparePayload() {
  const user = document.querySelector("#user-prompt").value;
  const system = document.querySelector("#system-prompt").value;
  if (!user.trim()) throw new Error("Enter a user message before comparing.");
  const messages = [];
  if (system.trim()) messages.push({ role: "system", content: system });
  messages.push({ role: "user", content: user });
  const schemaText = document.querySelector("#json-schema").value.trim();
  let schema;
  if (schemaText) {
    try { schema = JSON.parse(schemaText); } catch { throw new Error("JSON Schema must contain valid JSON."); }
    if (!schema || Array.isArray(schema) || typeof schema !== "object") throw new Error("JSON Schema must be a JSON object.");
  }
  return {
    models: [modelLeft.value, modelRight.value],
    messages,
    temperature: Number(document.querySelector("#temperature").value),
    max_tokens: Number(document.querySelector("#max-tokens").value),
    ...(schema ? { schema } : {})
  };
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  state.comparison = null;
  formError.hidden = true;
  summaryPanel.hidden = true;
  savePanel.hidden = true;
  try {
    const body = comparePayload();
    setLoading(true);
    state.comparison = await requestJSON("/api/v1/arena/compare", {
      method: "POST", headers: { "Content-Type": "application/json", "X-LLM-Replay-CSRF": csrf }, body: JSON.stringify(body)
    });
    state.comparison.results.forEach((result, index) => renderResult(result, state.cards[index]));
    renderSummary(state.comparison.results);
    renderSave(state.comparison.results);
    runStatus.textContent = "Comparison complete";
    runStatus.className = "run-status done";
  } catch (error) {
    formError.textContent = error.message;
    formError.hidden = false;
    runStatus.textContent = "Could not compare";
    runStatus.className = "run-status";
    state.cards.forEach((card) => {
      const copy = card.querySelector(".response-copy");
      if (copy.classList.contains("loading")) {
        copy.textContent = "Comparison did not complete. No provider result is available.";
        copy.className = "response-copy error";
      }
    });
  } finally { setLoading(false); if (state.comparison) { runStatus.textContent = "Comparison complete"; runStatus.className = "run-status done"; } }
});

document.addEventListener("keydown", (event) => {
  if ((event.ctrlKey || event.metaKey) && event.key === "Enter") form.requestSubmit();
});

document.querySelector("#review-save").addEventListener("click", () => {
  const model = document.querySelector("#baseline-model").value;
  document.querySelector("#save-summary").textContent = `${model} will be appended as one new record. This writes only to the dataset chosen in the arena command.`;
  saveDialog.showModal();
});

document.querySelector("#confirm-save").addEventListener("click", async (event) => {
  event.preventDefault();
  const status = document.querySelector("#save-status");
  try {
    const result = await requestJSON("/api/v1/arena/save", {
      method: "POST", headers: { "Content-Type": "application/json", "X-LLM-Replay-CSRF": csrf },
      body: JSON.stringify({
        arena_result_id: state.comparison.arena_result_id,
        baseline_model: document.querySelector("#baseline-model").value,
        use_response_as_expected: document.querySelector("#use-expected").checked,
        include_schema: document.querySelector("#include-schema").checked
      })
    });
    status.textContent = `Saved ${result.record_id}. Dataset now has ${result.dataset_record_count} records.`;
    saveDialog.close();
  } catch (error) { status.textContent = error.message; saveDialog.close(); }
});

(async function initialize() {
  try {
    state.config = await requestJSON("/api/v1/arena/config");
    renderModels(state.config.models);
    const available = state.config.models.filter((model) => model.available);
    compareButton.disabled = !selectionReady();
    if (available.length < 2) {
      formError.textContent = "At least two catalog models need provider credentials. Set an API key and restart Arena.";
      formError.hidden = false;
    }
  } catch (error) {
    formError.textContent = `Arena configuration could not be loaded: ${error.message}`;
    formError.hidden = false;
    compareButton.disabled = true;
  }
}());
