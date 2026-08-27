import React, { useEffect, useMemo, useState } from "react";
import { Box, Button, Stack, Typography } from "@mui/material";
import { ThemeProvider } from "@mui/material/styles";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  ChartsProvider,
  generateChartsTheme,
  getTheme,
  SnackbarProvider,
} from "@perses-dev/components";
import {
  dynamicImportPluginLoader,
  PluginRegistry,
  TimeRangeControls,
  TimeRangeProvider,
} from "@perses-dev/plugin-system";
import {
  Dashboard,
  DashboardProvider,
  DatasourceStoreProvider,
  PanelFocusProvider,
  VariableProvider,
} from "@perses-dev/dashboards";
import * as prometheusPlugin from "@perses-dev/prometheus-plugin";
import * as timeseriesChartPlugin from "@perses-dev/timeseries-chart-plugin";
import * as lokiPlugin from "@perses-dev/loki-plugin";
import {
  dashboardOrEmpty,
  datasourceApi,
  stubDashboard,
} from "./datasources.js";
import * as logsPanel from "./logs-panel.jsx";
import { loadPlugin } from "./plugins.js";
import { DashboardJsonEdit } from "./dashboard-edit.jsx";
import { Home } from "./home.jsx";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 0,
    },
  },
});

const pluginLoader = dynamicImportPluginLoader([
  loadPlugin(prometheusPlugin),
  loadPlugin(timeseriesChartPlugin),
  loadPlugin(lokiPlugin),
  loadPlugin(logsPanel),
]);

function parseHash() {
  const raw = (window.location.hash || "#/").replace(/^#/, "") || "/";
  if (raw === "/logs" || raw === "/explore" || raw === "/explore/logql") {
    return { page: "home", tab: "explore", lang: "logql", file: "" };
  }
  if (raw === "/explore/promql") {
    return { page: "home", tab: "explore", lang: "promql", file: "" };
  }
  const m = raw.match(/^\/d\/([^/]+)$/);
  if (m) {
    return { page: "dash", tab: "dashboards", lang: "promql", file: decodeURIComponent(m[1]) };
  }
  return { page: "home", tab: "dashboards", lang: "promql", file: "" };
}

export default function App() {
  const [route, setRoute] = useState(parseHash);
  const [timeRange, setTimeRange] = useState({ pastDuration: "1h" });
  const [refreshInterval, setRefreshInterval] = useState("0s");
  const [timeZone, setTimeZone] = useState("browser");
  const [dashboard, setDashboard] = useState(stubDashboard);
  const [dashRev, setDashRev] = useState(0);
  const [loadErr, setLoadErr] = useState("");
  const muiTheme = useMemo(() => getTheme("light"), []);
  const chartsTheme = useMemo(() => generateChartsTheme(muiTheme, {}), [muiTheme]);

  useEffect(() => {
    const onHash = () => setRoute(parseHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    if (route.page !== "dash" || !route.file) {
      return undefined;
    }
    let cancelled = false;
    fetch(`/dashboards/${encodeURIComponent(route.file)}`)
      .then((res) => {
        if (res.status === 404) {
          throw new Error("dashboard not found");
        }
        if (!res.ok) {
          throw new Error("dashboard fetch failed");
        }
        return res.json();
      })
      .then((data) => {
        if (!cancelled) {
          setDashboard(dashboardOrEmpty(data));
          setDashRev((n) => n + 1);
          setLoadErr("");
        }
      })
      .catch((e) => {
        if (!cancelled) {
          setDashboard(stubDashboard);
          setLoadErr(e.message || "load failed");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [route.page, route.file]);

  function goHome(tab, lang) {
    if (tab === "explore" || tab === "logs") {
      const which = lang || route.lang || "promql";
      window.location.hash = which === "logql" ? "#/explore/logql" : "#/explore/promql";
      return;
    }
    window.location.hash = "#/";
  }

  function openDash(file) {
    window.location.hash = `#/d/${encodeURIComponent(file)}`;
  }

  const display = (dashboard.spec && dashboard.spec.display) || {};
  const title = display.name || route.file || "iugum observe";
  const subtitle = display.description || "";

  return (
    <ThemeProvider theme={muiTheme}>
      <ChartsProvider chartsTheme={chartsTheme}>
        <SnackbarProvider
          anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
          variant="default"
          content=""
        >
          <PluginRegistry
            pluginLoader={pluginLoader}
            defaultPluginKinds={{
              Panel: "TimeSeriesChart",
              TimeSeriesQuery: "PrometheusTimeSeriesQuery",
              LogQuery: "LokiLogQuery",
            }}
          >
            <QueryClientProvider client={queryClient}>
              <TimeRangeProvider
                timeRange={timeRange}
                refreshInterval={refreshInterval}
                setTimeRange={setTimeRange}
                setRefreshInterval={setRefreshInterval}
              >
                <Box sx={{ p: 2, minHeight: "100vh" }}>
                  <Stack
                    direction="row"
                    justifyContent="space-between"
                    alignItems="center"
                    sx={{ mb: 2 }}
                  >
                    <div>
                      <Typography variant="h5" component="h1">
                        {route.page === "dash" ? title : "iugum observe"}
                      </Typography>
                      {route.page === "dash" ? (
                        subtitle ? (
                          <Typography variant="body2" color="text.secondary">
                            {subtitle}
                          </Typography>
                        ) : null
                      ) : (
                        <Typography variant="body2" color="text.secondary">
                          Pick a dashboard or run a log query.
                        </Typography>
                      )}
                    </div>
                    {route.page === "dash" ? (
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Button size="small" onClick={() => goHome("dashboards")}>
                          Home
                        </Button>
                        <DashboardJsonEdit
                          dashboard={dashboard}
                          path={`/dashboards/${encodeURIComponent(route.file)}`}
                          onSaved={(next) => {
                            setDashboard(dashboardOrEmpty(next));
                            setDashRev((n) => n + 1);
                          }}
                        />
                        <TimeRangeControls
                          timeZone={timeZone}
                          onTimeZoneChange={setTimeZone}
                        />
                      </Stack>
                    ) : null}
                  </Stack>
                  {route.page === "home" ? (
                    <Home
                      tab={route.tab}
                      onTab={(tab) => goHome(tab)}
                      exploreLang={route.lang || "promql"}
                      onExploreLang={(lang) => goHome("explore", lang)}
                      onOpen={openDash}
                    />
                  ) : (
                    <VariableProvider>
                      <DatasourceStoreProvider
                        dashboardResource={dashboard}
                        datasourceApi={datasourceApi}
                      >
                        <DashboardProvider
                          key={`${(dashboard.metadata && dashboard.metadata.name) || "dash"}-${dashRev}`}
                          initialState={{
                            dashboardResource: dashboard,
                            isEditMode: false,
                          }}
                        >
                          {loadErr ? (
                            <Typography color="error">{loadErr}</Typography>
                          ) : (
                            <PanelFocusProvider>
                              <Dashboard
                                emptyDashboardProps={{
                                  title: "Empty dashboard",
                                  description:
                                    "No panels yet. Prometheus and Loki stay wired to this origin.",
                                  actions: false,
                                }}
                              />
                            </PanelFocusProvider>
                          )}
                        </DashboardProvider>
                      </DatasourceStoreProvider>
                    </VariableProvider>
                  )}
                </Box>
              </TimeRangeProvider>
            </QueryClientProvider>
          </PluginRegistry>
        </SnackbarProvider>
      </ChartsProvider>
    </ThemeProvider>
  );
}
