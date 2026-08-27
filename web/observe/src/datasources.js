// Same-origin Prometheus + Loki. Empty directUrl so fetch('/api/v1/...')
// and fetch('/loki/api/v1/...') hit this observe process.
const sameOrigin = "";

export const prometheusDatasource = {
  kind: "GlobalDatasource",
  metadata: { name: "prometheus" },
  spec: {
    default: true,
    plugin: {
      kind: "PrometheusDatasource",
      spec: {
        directUrl: sameOrigin,
      },
    },
  },
};

export const lokiDatasource = {
  kind: "GlobalDatasource",
  metadata: { name: "loki" },
  spec: {
    default: true,
    plugin: {
      kind: "LokiDatasource",
      spec: {
        directUrl: sameOrigin,
      },
    },
  },
};

import homelabDashboard from "../homelab-dashboard.json";

export const stubDashboard = {
  kind: "Dashboard",
  metadata: {
    name: "iugum-observe",
    project: "iugum",
    createdAt: "2026-08-24T00:00:00Z",
    updatedAt: "2026-08-24T00:00:00Z",
    version: 0,
  },
  spec: {
    display: { name: "iugum observe" },
    duration: "1h",
    refreshInterval: "0s",
    variables: [],
    panels: {},
    layouts: [],
  },
};

export function dashboardOrEmpty(raw) {
  if (
    raw &&
    raw.kind === "Dashboard" &&
    raw.spec &&
    raw.spec.panels &&
    typeof raw.spec.panels === "object" &&
    Object.keys(raw.spec.panels).length > 0
  ) {
    return raw;
  }
  return stubDashboard;
}

export const defaultDashboard = dashboardOrEmpty(homelabDashboard);

function pluginKind(selector, fallbackKind) {
  if (!selector) {
    return fallbackKind;
  }
  if (typeof selector === "string") {
    return selector;
  }
  return selector.kind || fallbackKind;
}

class DatasourceApiImpl {
  getDatasource() {
    return Promise.resolve(undefined);
  }

  getGlobalDatasource(selector) {
    const kind = pluginKind(selector, "PrometheusDatasource");
    const name = selector && selector.name;
    if (name === "loki" || kind === "LokiDatasource") {
      return Promise.resolve(lokiDatasource);
    }
    if (name === "prometheus" || kind === "PrometheusDatasource" || !kind) {
      return Promise.resolve(prometheusDatasource);
    }
    return Promise.resolve(undefined);
  }

  listDatasources() {
    return Promise.resolve([]);
  }

  listGlobalDatasources(pluginKindName) {
    const all = [prometheusDatasource, lokiDatasource];
    const kind = pluginKind(pluginKindName);
    if (!kind) {
      return Promise.resolve(all);
    }
    return Promise.resolve(all.filter((ds) => ds.spec.plugin.kind === kind));
  }

  buildProxyUrl() {
    return "";
  }
}

export const datasourceApi = new DatasourceApiImpl();
