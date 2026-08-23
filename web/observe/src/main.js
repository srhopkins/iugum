import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import "./style.css";

const TEMPS = ["junction_c", "edge_c", "mem_c", "cpu_c"];
const COLORS = {
  junction_c: "#e11",
  edge_c: "#e80",
  mem_c: "#08c",
  cpu_c: "#3a3",
  fan_pct: "#555",
};

function field(s, ...names) {
  for (const n of names) {
    if (s[n] !== undefined) return s[n];
  }
  return undefined;
}

function drawMarks(marks) {
  return [
    (u) => {
      const ctx = u.ctx;
      const { left, top, width, height } = u.bbox;
      for (const y of marks) {
        const py = u.valToPos(y, "y", true);
        if (py < top || py > top + height) continue;
        ctx.save();
        ctx.strokeStyle = y >= 105 ? "#c00" : y >= 100 ? "#e80" : "#6a6";
        ctx.setLineDash([4, 4]);
        ctx.beginPath();
        ctx.moveTo(left, py);
        ctx.lineTo(left + width, py);
        ctx.stroke();
        ctx.restore();
      }
    },
  ];
}

function seriesPoints(resp, name) {
  const list = field(resp, "series", "Series") || [];
  const hits = list.filter((s) => field(s, "name", "Name") === name);
  const pts = [];
  for (const s of hits) {
    const p = field(s, "points", "Points") || [];
    for (const row of p) pts.push(row);
  }
  pts.sort((a, b) => a[0] - b[0]);
  return pts;
}

function align(seriesMap, names) {
  const xs = new Set();
  for (const n of names) {
    for (const p of seriesMap[n] || []) xs.add(p[0]);
  }
  const times = [...xs].sort((a, b) => a - b);
  const cols = [times];
  for (const n of names) {
    const by = new Map((seriesMap[n] || []).map((p) => [p[0], p[1]]));
    cols.push(times.map((t) => (by.has(t) ? by.get(t) : null)));
  }
  return cols;
}

function chart(el, names, data, marks, yLabel) {
  el.replaceChildren();
  if (!data[0] || data[0].length === 0) {
    el.textContent = "no samples";
    return;
  }
  const series = [{ label: "t" }];
  for (const n of names) {
    series.push({
      label: n,
      stroke: COLORS[n] || "#333",
      width: 1.5,
    });
  }
  const width = Math.max(320, el.clientWidth || 720);
  return new uPlot(
    {
      width,
      height: 280,
      series,
      axes: [{}, { label: yLabel }],
      hooks: { draw: marks && marks.length ? drawMarks(marks) : [] },
    },
    data,
    el
  );
}

async function load() {
  const meta = await fetch("/meta.json").then((r) => r.json());
  const marks = meta.marks_c || [50, 100, 105];
  const resp = await fetch("/query/metrics?q=").then((r) => r.json());
  const by = {};
  for (const n of [...TEMPS, "fan_pct"]) {
    by[n] = seriesPoints(resp, n);
  }
  chart(document.getElementById("temps"), TEMPS, align(by, TEMPS), marks, "°C");
  chart(document.getElementById("fan"), ["fan_pct"], align(by, ["fan_pct"]), [], "%");

  const logs = await fetch('/query/logs?q={stream="homelab"}').then((r) => r.json());
  const rows = logs.logs || [];
  const pre = document.getElementById("logs");
  if (!rows.length) {
    pre.textContent = "no logs";
    return;
  }
  pre.textContent = rows
    .map((l) => {
      const msg = l.Message || l.message || "";
      const stream = l.Stream || l.stream || "";
      return `${stream}  ${msg}`;
    })
    .join("\n");
}

load().catch((err) => {
  document.getElementById("logs").textContent = String(err);
});
