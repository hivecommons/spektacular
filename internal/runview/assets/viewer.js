// Spektacular run viewer. It draws the page data embedded in #run-data as a
// waterfall on one canvas, drawing only the rows in view. It never fetches
// anything, and every string from the data reaches the page only through
// fillText or textContent.
(function () {
  "use strict";

  var data = JSON.parse(document.getElementById("run-data").textContent);
  var S = data.strings;
  var R = data.rows;
  var G = data.gaps;
  var n = R.span_id.length;

  var ROW_H = 20;
  var AXIS_H = 28;
  var LABEL_W = 280;
  var INDENT = 12;
  var PAD_R = 16;
  var MIN_BAR = 2;

  var TYPE_LABEL = {
    "task": "Task",
    "step": "Step",
    "sub-agent": "Helper agent",
    "user-wait": "Wait on the user",
    "work": "Other work",
    "command": "Command"
  };
  var PROBLEM_LABEL = {
    "failed": "Failed",
    "interrupted": "Interrupted",
    "abandoned": "Abandoned",
    "open": "Never closed"
  };
  var GLYPH = { "failed": "✕", "interrupted": "‖", "abandoned": "∅", "open": "…" };
  var STATUS_LABEL = { "-1": "none (never closed)", "0": "ok", "1": "error" };

  var T = {};
  data.types.forEach(function (t, i) { T[t] = i; });
  var P = {};
  data.problems.forEach(function (p, i) { P[p] = i; });

  var wall = Math.max(1, data.totals.wall_ms);
  var runStart = Date.parse(data.run.started_at);

  // The page's state: the zoom window (ms from the run's start), the
  // selected row and the gap threshold.
  var state = { from: 0, to: wall, selected: -1, threshold: data.gap_threshold_ms };

  // visible is the list of row indexes drawn, top to bottom.
  var visible = [];
  for (var i = 0; i < n; i++) visible.push(i);
  // position maps a row index to its place in visible, or -1.
  var position = new Int32Array(n);
  function indexVisible() {
    position.fill(-1);
    for (var v = 0; v < visible.length; v++) position[visible[v]] = v;
  }
  indexVisible();

  var userWaits = [];
  for (var w = 0; w < n; w++) if (R.type[w] === T["user-wait"]) userWaits.push(w);

  function str(i) { return i < 0 || i == null ? "" : S[i]; }
  function typeName(row) { return data.types[R.type[row]]; }
  function problemName(row) { return data.problems[R.problem[row]]; }

  // ---- Formatting ------------------------------------------------------

  function pad2(v) { return v < 10 ? "0" + v : "" + v; }

  // fmtMS prints a duration as the run statistics report does: 12.3s,
  // 4m05s or 1h02m03s.
  function fmtMS(v) {
    var sign = "";
    if (v < 0) { sign = "-"; v = -v; }
    if (v < 60000) return sign + (v / 1000).toFixed(1) + "s";
    var h = Math.floor(v / 3600000);
    var m = Math.floor((v % 3600000) / 60000);
    var s = Math.floor((v % 60000) / 1000);
    if (h > 0) return sign + h + "h" + pad2(m) + "m" + pad2(s) + "s";
    return sign + m + "m" + pad2(s) + "s";
  }

  function fmtUTC(ms) {
    var iso = new Date(ms).toISOString();
    return iso.slice(0, 10) + " " + iso.slice(11, 23) + " UTC";
  }

  // tickLabel prints an axis offset, precise enough for the tick step.
  function tickLabel(ms, step) {
    if (ms === 0) return "0";
    if (step < 1000) return "+" + (ms / 1000).toFixed(step < 100 ? 2 : 1) + "s";
    if (ms < 60000) return "+" + Math.round(ms / 1000) + "s";
    var h = Math.floor(ms / 3600000);
    var m = Math.floor((ms % 3600000) / 60000);
    var s = Math.round((ms % 60000) / 1000);
    if (step < 60000) return "+" + (h > 0 ? h + "h" + pad2(m) + "m" : m + "m") + pad2(s) + "s";
    if (h > 0) return "+" + h + "h" + pad2(m) + "m";
    return "+" + m + "m";
  }

  var STEPS = [1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 15000, 30000,
    60000, 120000, 300000, 600000, 900000, 1800000, 3600000, 7200000, 14400000,
    21600000, 43200000, 86400000];

  function tickStep(span, width) {
    var raw = span / Math.max(1, width / 90);
    for (var i = 0; i < STEPS.length; i++) if (STEPS[i] >= raw) return STEPS[i];
    return STEPS[STEPS.length - 1] * Math.ceil(raw / STEPS[STEPS.length - 1]);
  }

  // ---- Header, legend and details ---------------------------------------

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function fillHeader() {
    var run = data.run;
    var title = run.name || "Spektacular run";
    document.title = title + " - Spektacular run";
    document.getElementById("run-title").textContent = title;
    var meta = [run.kind, "agent " + run.agent, "run " + run.run_id.slice(0, 8), run.status,
      "started " + fmtUTC(runStart)];
    document.getElementById("run-meta").textContent = meta.join(" · ");
    if (run.skipped_lines > 0) {
      var sk = document.getElementById("run-skipped");
      sk.textContent = "Skipped lines: " + run.skipped_lines;
      sk.hidden = false;
    }
    var totals = document.getElementById("totals");
    [["Wall time", data.totals.wall_ms], ["Working time", data.totals.working_ms],
      ["User wait", data.totals.user_wait_ms], ["Helper wait", data.totals.helper_wait_ms]]
      .forEach(function (t) {
        var d = el("div");
        d.appendChild(el("dt", null, t[0]));
        var dd = el("dd", null, fmtMS(t[1]));
        dd.title = t[1] + " ms";
        d.appendChild(dd);
        totals.appendChild(d);
      });
  }

  function legendItem(label, colour, cls, glyph) {
    var item = el("span", "legend-item");
    var sw = el("span", "swatch" + (cls ? " " + cls : ""));
    sw.style.setProperty("--sw", "var(" + colour + ")");
    item.appendChild(sw);
    if (glyph) {
      var g = el("span", "glyph", glyph);
      g.style.setProperty("--sw", "var(" + colour + ")");
      item.appendChild(g);
    }
    item.appendChild(el("span", null, label));
    return item;
  }

  function fillLegend() {
    var lg = document.getElementById("legend");
    lg.appendChild(legendItem("Task", "--c-task"));
    lg.appendChild(legendItem("Step", "--c-step"));
    lg.appendChild(legendItem("Helper agent", "--c-sub-agent"));
    lg.appendChild(legendItem("Wait on the user", "--c-user-wait", "hatched"));
    lg.appendChild(legendItem("Other work", "--c-work"));
    lg.appendChild(legendItem("Command", "--c-command", "thin"));
    lg.appendChild(legendItem("Failed", "--c-failed", "outline", GLYPH.failed));
    lg.appendChild(legendItem("Interrupted", "--c-interrupted", "outline dashed", GLYPH.interrupted));
    lg.appendChild(legendItem("Abandoned", "--c-abandoned", "outline dotted", GLYPH.abandoned));
    lg.appendChild(legendItem("Never closed", "--c-step", "fade", GLYPH.open));
    lg.appendChild(legendItem("Gap over threshold", "--c-gap", "hatched"));
  }

  function showDetails(row) {
    var list = document.getElementById("details-list");
    var empty = document.getElementById("details-empty");
    var note = document.getElementById("details-note");
    while (list.firstChild) list.removeChild(list.firstChild);
    if (row < 0) {
      list.hidden = true;
      note.hidden = true;
      empty.hidden = false;
      return;
    }
    var typ = typeName(row);
    var start = R.start_ms[row];
    var status = STATUS_LABEL[String(R.status[row])] || "";
    if (R.message[row] >= 0) status += ": " + str(R.message[row]);
    var fields = [
      ["Name", str(R.name[row])],
      ["Type", TYPE_LABEL[typ] || typ],
      ["Start", fmtUTC(runStart + start) + " (+" + fmtMS(start) + ")"],
      ["Duration", fmtMS(R.duration_ms[row]) + " (" + R.duration_ms[row] + " ms)"],
      ["Status", status],
      ["End reason", str(R.end_reason[row])],
      ["Agent", data.run.agent],
      ["Workflow", data.run.kind + " " + data.run.name],
      ["Step", str(R.step[row])],
      ["Task", str(R.task_id[row])],
      ["Span id", R.span_id[row]]
    ];
    var prob = problemName(row);
    if (prob) fields.push(["Problem", PROBLEM_LABEL[prob] || prob]);
    if (typ === "command") {
      fields.push(["Command line", str(R.args[row]), "code"]);
      fields.push(["Exit code", R.exit_code[row] == null ? "" : String(R.exit_code[row])]);
    }
    fields.forEach(function (f) {
      if (f[1] === "") return;
      list.appendChild(el("dt", null, f[0]));
      list.appendChild(el("dd", f[2] || null, f[1]));
    });
    list.hidden = false;
    empty.hidden = true;
    var incomplete = prob === "interrupted" || prob === "abandoned" || prob === "open";
    note.textContent = incomplete ? "This span did not complete: its bar is drawn to where it was cut off." : "";
    note.hidden = !incomplete;
  }

  // ---- Drawing ---------------------------------------------------------

  var wf = document.getElementById("waterfall");
  var canvas = document.getElementById("chart");
  var spacer = document.getElementById("spacer");
  var ctx = canvas.getContext("2d");
  var dpr = 1;
  var cw = 0;
  var ch = 0;
  var colours = {};
  var hatches = {};

  function readColours() {
    var cs = getComputedStyle(document.documentElement);
    ["bg", "bg-panel", "fg", "fg-muted", "line", "grid", "select", "c-task", "c-step",
      "c-sub-agent", "c-user-wait", "c-work", "c-command", "c-failed", "c-interrupted",
      "c-abandoned", "c-gap"].forEach(function (k) {
      colours[k] = cs.getPropertyValue("--" + k).trim();
    });
    hatches = {};
  }

  function typeColour(typ) { return colours["c-" + typ] || colours["c-work"]; }

  // hatch is a diagonal-stripe pattern in colour.
  function hatch(colour) {
    if (hatches[colour]) return hatches[colour];
    var c = document.createElement("canvas");
    var size = Math.round(6 * dpr);
    c.width = size;
    c.height = size;
    var g = c.getContext("2d");
    g.strokeStyle = colour;
    g.lineWidth = Math.max(1, dpr * 1.5);
    g.beginPath();
    g.moveTo(-1, size + 1);
    g.lineTo(size + 1, -1);
    g.stroke();
    var p = ctx.createPattern(c, "repeat");
    if (p && p.setTransform && typeof DOMMatrix !== "undefined") p.setTransform(new DOMMatrix().scale(1 / dpr));
    hatches[colour] = p;
    return p;
  }

  function contentHeight() { return AXIS_H + visible.length * ROW_H; }

  function resize() {
    dpr = window.devicePixelRatio || 1;
    cw = wf.clientWidth;
    ch = wf.clientHeight;
    canvas.style.height = ch + "px";
    canvas.width = Math.max(1, Math.round(cw * dpr));
    canvas.height = Math.max(1, Math.round(ch * dpr));
    spacer.style.height = Math.max(0, contentHeight() - ch) + "px";
    hatches = {};
  }

  function x0() { return LABEL_W; }
  function x1() { return Math.max(LABEL_W + 10, cw - PAD_R); }
  function xOf(ms) { return x0() + (ms - state.from) / (state.to - state.from) * (x1() - x0()); }

  var pending = false;
  function schedule() {
    if (pending) return;
    pending = true;
    window.requestAnimationFrame(function () { pending = false; draw(); });
  }

  function fitText(text, width) {
    if (ctx.measureText(text).width <= width) return text;
    var lo = 0, hi = text.length;
    while (lo < hi) {
      var mid = (lo + hi + 1) >> 1;
      if (ctx.measureText(text.slice(0, mid) + "…").width <= width) lo = mid; else hi = mid - 1;
    }
    return lo > 0 ? text.slice(0, lo) + "…" : "";
  }

  function drawBands(top, bottom) {
    var left = x0(), right = x1();
    // Waits on the user: a shaded band across the whole timeline, drawn
    // from every user-wait row whatever is shown.
    ctx.save();
    ctx.globalAlpha = 0.15;
    ctx.fillStyle = colours["c-user-wait"];
    for (var i = 0; i < userWaits.length; i++) {
      var r = userWaits[i];
      var s = R.start_ms[r], e = s + R.duration_ms[r];
      if (e < state.from || s > state.to) continue;
      var a = Math.max(left, xOf(s)), b = Math.min(right, xOf(e));
      ctx.fillRect(a, top, Math.max(1, b - a), bottom - top);
    }
    ctx.restore();

    // Gaps over the threshold: a hatched band labelled with its length.
    ctx.save();
    for (var g = 0; g < G.start_ms.length; g++) {
      var d = G.duration_ms[g];
      if (d <= state.threshold) continue;
      var gs = G.start_ms[g], ge = gs + d;
      if (ge < state.from || gs > state.to) continue;
      var ga = Math.max(left, xOf(gs)), gb = Math.min(right, xOf(ge));
      var gw = Math.max(1, gb - ga);
      ctx.globalAlpha = 0.5;
      ctx.fillStyle = hatch(colours["c-gap"]);
      ctx.fillRect(ga, top, gw, bottom - top);
      ctx.globalAlpha = 1;
    }
    ctx.restore();
  }

  // drawGapLabels labels each gap band over the threshold with its length,
  // at the foot of the view, over the bars.
  function drawGapLabels(bottom) {
    var left = x0(), right = x1();
    ctx.save();
    ctx.font = "11px system-ui, sans-serif";
    ctx.textBaseline = "middle";
    for (var g = 0; g < G.start_ms.length; g++) {
      var d = G.duration_ms[g];
      if (d <= state.threshold) continue;
      var gs = G.start_ms[g], ge = gs + d;
      if (ge < state.from || gs > state.to) continue;
      var ga = Math.max(left, xOf(gs)), gb = Math.min(right, xOf(ge));
      var label = "gap " + fmtMS(d);
      var tw = ctx.measureText(label).width + 10;
      if (gb - ga < tw) label = fmtMS(d), tw = ctx.measureText(label).width + 10;
      if (gb - ga < tw) continue;
      var lx = ga + (gb - ga - tw) / 2;
      ctx.fillStyle = colours["bg-panel"];
      ctx.strokeStyle = colours["c-gap"];
      ctx.lineWidth = 1;
      ctx.fillRect(lx, bottom - 22, tw, 18);
      ctx.strokeRect(lx + 0.5, bottom - 21.5, tw - 1, 17);
      ctx.fillStyle = colours["fg"];
      ctx.fillText(label, lx + 5, bottom - 13);
    }
    ctx.restore();
  }

  function drawBar(row, y) {
    var typ = typeName(row);
    var prob = problemName(row);
    var colour = typeColour(typ);
    var s = R.start_ms[row], e = s + R.duration_ms[row];
    if (e < state.from || s > state.to) return;
    var a = xOf(s), b = xOf(e);
    var w = Math.max(MIN_BAR, b - a);
    var top = y + 4, h = ROW_H - 8;
    if (typ === "command") { top = y + 7; h = ROW_H - 14; }

    if (typ === "user-wait") {
      ctx.fillStyle = hatch(colour);
      ctx.fillRect(a, top, w, h);
      ctx.strokeStyle = colour;
      ctx.lineWidth = 1;
      ctx.strokeRect(a + 0.5, top + 0.5, Math.max(1, w - 1), h - 1);
    } else if (prob === "open") {
      var grad = ctx.createLinearGradient(a, 0, a + w, 0);
      grad.addColorStop(0, colour);
      grad.addColorStop(0.6, colour);
      grad.addColorStop(1, "rgba(0,0,0,0)");
      ctx.fillStyle = grad;
      ctx.fillRect(a, top, w, h);
    } else {
      ctx.fillStyle = colour;
      ctx.fillRect(a, top, w, h);
    }

    var border = null, dash = [];
    if (prob === "failed") { border = colours["c-failed"]; }
    else if (prob === "interrupted") { border = colours["c-interrupted"]; dash = [4, 3]; }
    else if (prob === "abandoned") { border = colours["c-abandoned"]; dash = [1, 2]; }
    if (border) {
      ctx.save();
      ctx.strokeStyle = border;
      ctx.lineWidth = 2;
      ctx.setLineDash(dash);
      ctx.strokeRect(a - 1, top - 1, w + 2, h + 2);
      ctx.restore();
    }
    if (row === state.selected) {
      ctx.save();
      ctx.strokeStyle = colours["fg"];
      ctx.lineWidth = 1.5;
      ctx.strokeRect(a - 2.5, top - 2.5, w + 5, h + 5);
      ctx.restore();
    }
  }

  function drawLabel(row, y) {
    var x = 8 + R.depth[row] * INDENT;
    var prob = problemName(row);
    ctx.textBaseline = "middle";
    if (prob) {
      ctx.fillStyle = prob === "open" ? colours["fg-muted"] : colours["c-" + prob];
      ctx.font = "bold 12px system-ui, sans-serif";
      ctx.fillText(GLYPH[prob], x, y + ROW_H / 2);
      x += 14;
    }
    var typ = typeName(row);
    ctx.font = (typ === "task" || typ === "step" ? "600 " : "") + "12px system-ui, sans-serif";
    ctx.fillStyle = typ === "command" ? colours["fg-muted"] : colours["fg"];
    ctx.fillText(fitText(str(R.name[row]), LABEL_W - x - 8), x, y + ROW_H / 2);
  }

  function drawAxis() {
    var left = x0(), right = x1();
    ctx.fillStyle = colours["bg-panel"];
    ctx.fillRect(0, 0, cw, AXIS_H);
    var span = state.to - state.from;
    var step = tickStep(span, right - left);
    ctx.font = "11px system-ui, sans-serif";
    ctx.textBaseline = "middle";
    ctx.fillStyle = colours["fg-muted"];
    ctx.strokeStyle = colours["line"];
    ctx.lineWidth = 1;
    for (var t = Math.ceil(state.from / step) * step; t <= state.to; t += step) {
      var x = Math.round(xOf(t)) + 0.5;
      ctx.beginPath();
      ctx.moveTo(x, AXIS_H - 6);
      ctx.lineTo(x, AXIS_H);
      ctx.stroke();
      var label = tickLabel(t, step);
      var tw = ctx.measureText(label).width;
      ctx.fillText(label, Math.min(right - tw, Math.max(left, x - tw / 2)), AXIS_H / 2 - 2);
    }
    ctx.fillStyle = colours["fg-muted"];
    ctx.fillText("Span", 8, AXIS_H / 2 - 2);
    ctx.beginPath();
    ctx.moveTo(0, AXIS_H - 0.5);
    ctx.lineTo(cw, AXIS_H - 0.5);
    ctx.stroke();
    return step;
  }

  function drawGrid(step) {
    ctx.strokeStyle = colours["grid"];
    ctx.lineWidth = 1;
    for (var t = Math.ceil(state.from / step) * step; t <= state.to; t += step) {
      var x = Math.round(xOf(t)) + 0.5;
      ctx.beginPath();
      ctx.moveTo(x, AXIS_H);
      ctx.lineTo(x, ch);
      ctx.stroke();
    }
  }

  function draw() {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = colours["bg"];
    ctx.fillRect(0, 0, cw, ch);

    var step = tickStep(state.to - state.from, x1() - x0());
    drawGrid(step);
    drawBands(AXIS_H, ch);

    var scroll = wf.scrollTop;
    var first = Math.max(0, Math.floor(scroll / ROW_H));
    var last = Math.min(visible.length - 1, first + Math.ceil((ch - AXIS_H) / ROW_H) + 1);

    // Selected row highlight, across the label column and timeline.
    if (state.selected >= 0 && position[state.selected] >= first && position[state.selected] <= last) {
      ctx.fillStyle = colours["select"];
      ctx.fillRect(0, AXIS_H + position[state.selected] * ROW_H - scroll, cw, ROW_H);
    }

    // Bars, clipped to the timeline.
    ctx.save();
    ctx.beginPath();
    ctx.rect(x0(), AXIS_H, x1() - x0() + PAD_R, ch - AXIS_H);
    ctx.clip();
    for (var v = first; v <= last; v++) drawBar(visible[v], AXIS_H + v * ROW_H - scroll);
    ctx.restore();
    drawGapLabels(ch);

    // Labels, clipped to the label column.
    ctx.save();
    ctx.beginPath();
    ctx.rect(0, AXIS_H, LABEL_W, ch - AXIS_H);
    ctx.clip();
    for (var u = first; u <= last; u++) drawLabel(visible[u], AXIS_H + u * ROW_H - scroll);
    ctx.restore();

    ctx.strokeStyle = colours["line"];
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(LABEL_W - 0.5, 0);
    ctx.lineTo(LABEL_W - 0.5, ch);
    ctx.stroke();

    drawAxis();
  }

  // ---- Selection -------------------------------------------------------

  function select(row) {
    state.selected = row;
    showDetails(row);
    schedule();
  }

  // rowAt is the row under a point on the canvas, or -1.
  function rowAt(y) {
    if (y < AXIS_H) return -1;
    var v = Math.floor((y - AXIS_H + wf.scrollTop) / ROW_H);
    return v >= 0 && v < visible.length ? visible[v] : -1;
  }

  // barHit reports whether x is on row's bar, allowing a few pixels.
  function barHit(row, x) {
    var a = xOf(R.start_ms[row]);
    var b = Math.max(a + MIN_BAR, xOf(R.start_ms[row] + R.duration_ms[row]));
    return x >= a - 4 && x <= b + 4;
  }

  function point(e) {
    var r = canvas.getBoundingClientRect();
    return { x: e.clientX - r.left, y: e.clientY - r.top };
  }

  canvas.addEventListener("click", function (e) {
    var p = point(e);
    if (p.y < AXIS_H) return;
    var row = rowAt(p.y);
    if (row >= 0 && (p.x < LABEL_W || barHit(row, p.x))) select(row);
    else select(-1);
  });

  function scrollToRow(row) {
    var v = position[row];
    if (v < 0) return;
    var top = v * ROW_H, view = ch - AXIS_H;
    if (top < wf.scrollTop) wf.scrollTop = top;
    else if (top + ROW_H > wf.scrollTop + view) wf.scrollTop = top + ROW_H - view;
  }

  wf.addEventListener("keydown", function (e) {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    if (!visible.length) return;
    e.preventDefault();
    var v = state.selected >= 0 ? position[state.selected] : -1;
    v = e.key === "ArrowDown" ? Math.min(visible.length - 1, v + 1) : Math.max(0, v - 1);
    select(visible[v]);
    scrollToRow(visible[v]);
  });

  // ---- Start -----------------------------------------------------------

  wf.addEventListener("scroll", schedule, { passive: true });
  window.addEventListener("resize", function () { resize(); schedule(); });
  if (window.matchMedia) {
    var mq = window.matchMedia("(prefers-color-scheme: dark)");
    var onScheme = function () { readColours(); schedule(); };
    if (mq.addEventListener) mq.addEventListener("change", onScheme);
    else if (mq.addListener) mq.addListener(onScheme);
  }

  fillHeader();
  fillLegend();
  readColours();
  resize();
  draw();
})();
