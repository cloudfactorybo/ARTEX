"use client";

import * as React from "react";

import { ClipboardListIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { TaskSummary } from "@/lib/types";

// Operator-facing technical summary: what the task did, what it could not test,
// which credentials it used, coverage, findings and cost. Computed server-side.

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-md border px-3 py-2">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="text-lg font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function fmtDuration(sec: number): string {
  if (!sec || sec < 0) return "—";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export function SummaryTab({ taskId }: { taskId: string }) {
  const [data, setData] = React.useState<TaskSummary | null>(null);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    let active = true;
    setLoading(true);
    api
      .taskSummary(taskId)
      .then((d) => {
        if (active) setData(d);
      })
      .catch((e) => toast.error(t("Failed to load: {error}", { error: (e as Error).message })))
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [taskId]);

  if (loading) return <p className="p-6 text-sm text-muted-foreground">{t("Loading…")}</p>;
  if (!data) return <p className="p-6 text-sm text-muted-foreground">{t("No data")}</p>;

  const did = data.intents.filter((i) => i.state === "done");
  // What it could not test: stalled (exhausted / timed out) and blocked intents.
  const couldNot = data.intents.filter((i) => i.state === "exhausted" || i.state === "blocked");
  const negativeCoverage = (data.coverage?.entries ?? []).filter((e) => {
    const o = e.outcome.toLowerCase();
    return o.includes("negativ") || o.includes("not") || o.includes("fail") || o.includes("block");
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2.5">
        <ClipboardListIcon className="h-5 w-5 shrink-0" />
        <div>
          <h2 className="text-base font-semibold leading-tight">{t("Technical summary")}</h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            {t(
              "What the task did, what it could not test, which credentials it used and the coverage — for the team, not the client.",
            )}
          </p>
        </div>
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <Stat label={t("Status")} value={data.status || "—"} />
        <Stat label={t("Findings")} value={data.findings?.total ?? 0} />
        <Stat label={t("Facts")} value={data.fact_count} />
        <Stat label={t("Tool executions")} value={data.tool_calls} />
        <Stat label={t("Duration")} value={fmtDuration(data.wall_seconds)} />
        <Stat label={t("Tokens")} value={(data.tokens?.total ?? 0).toLocaleString()} />
      </div>

      {/* Findings by severity */}
      {Object.keys(data.findings_by_severity).length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">{t("Findings by severity")}:</span>
          {Object.entries(data.findings_by_severity).map(([sev, n]) => (
            <Badge key={sev} variant="outline">
              {sev}: {n}
            </Badge>
          ))}
        </div>
      )}

      {/* Credentials used */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("Credentials used")}</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {data.credentials.length === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">{t("No credentials were used.")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Domain")}</TableHead>
                  <TableHead>{t("Username")}</TableHead>
                  <TableHead>{t("Password")}</TableHead>
                  <TableHead>{t("Source")}</TableHead>
                  <TableHead>{t("Note")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.credentials.map((c, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-mono text-xs">{c.domain || "—"}</TableCell>
                    <TableCell className="font-mono text-xs">{c.username || "—"}</TableCell>
                    <TableCell>{c.has_password ? "••••••" : "—"}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{c.source === "agent" ? t("Agent-generated") : t("Operator")}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-xs">{c.note || "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* What it did */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">
            {t("What it did")} ({did.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {did.length === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">{t("No completed directions.")}</p>
          ) : (
            <ul className="divide-y">
              {did.map((i) => (
                <li key={i.id} className="px-4 py-2 text-sm">
                  {i.summary || t("(no summary)")}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {/* What it could not test */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">
            {t("What it could not test")} ({couldNot.length + negativeCoverage.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {couldNot.length === 0 && negativeCoverage.length === 0 && (data.coverage?.unexamined?.length ?? 0) === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">{t("Nothing was left untested.")}</p>
          ) : (
            <ul className="divide-y">
              {couldNot.map((i) => (
                <li key={i.id} className="flex items-start gap-2 px-4 py-2 text-sm">
                  <Badge
                    variant="outline"
                    className={
                      i.state === "blocked" ? "border-destructive text-destructive" : "border-amber-400 text-amber-600"
                    }
                  >
                    {i.state === "blocked" ? t("Blocked") : t("Incomplete")}
                  </Badge>
                  <span>
                    {i.summary || t("(no summary)")}
                    {i.blocked_reason ? ` — ${i.blocked_reason}` : ""}
                  </span>
                </li>
              ))}
              {negativeCoverage.map((e, i) => (
                <li key={`c${i}`} className="px-4 py-2 text-sm">
                  <span className="font-mono text-xs">{e.surface}</span> · {e.risk_area}: {e.outcome}
                  {e.reason ? ` — ${e.reason}` : ""}
                </li>
              ))}
              {(data.coverage?.unexamined ?? []).map((u, i) => (
                <li key={`u${i}`} className="px-4 py-2 text-sm text-muted-foreground">
                  {t("Not examined")}: {u}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {/* Coverage ledger */}
      {data.coverage_enabled && (data.coverage?.entries?.length ?? 0) > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t("Coverage")}</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Surface")}</TableHead>
                  <TableHead>{t("Risk area")}</TableHead>
                  <TableHead>{t("Outcome")}</TableHead>
                  <TableHead>{t("Evidence")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(data.coverage?.entries ?? []).map((e, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-mono text-xs">{e.surface}</TableCell>
                    <TableCell>{e.risk_area}</TableCell>
                    <TableCell>{e.outcome}</TableCell>
                    <TableCell className="text-muted-foreground text-xs">{e.evidence || "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
