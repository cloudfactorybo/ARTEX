"use client";

import * as React from "react";

import { LanguagesIcon } from "lucide-react";
import { toast } from "sonner";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { AgentLanguage } from "@/lib/types";

// Mirrors the codes accepted by agent.ParseAgentLanguage.
const languages: { value: AgentLanguage; label: string; hint: string }[] = [
  {
    value: "es",
    label: t("Spanish"),
    hint: t("Replies, findings, facts and reports are written in Spanish."),
  },
  {
    value: "en",
    label: t("English"),
    hint: t("Agents write in English; prompts are sent unchanged."),
  },
  {
    value: "auto",
    label: t("Same as the operator"),
    hint: t("Agents answer in the language of your latest message, or of the task description and goal."),
  },
];

// AgentLanguageCard sets the language agents write their human-facing text in.
export function AgentLanguageCard() {
  const [value, setValue] = React.useState<AgentLanguage | "">("");
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    api
      .settings()
      .then((s) => setValue(s.agent_language ?? "es"))
      .catch((e) => toast.error(t("Failed to load: {error}", { error: (e as Error).message })));
  }, []);

  const save = (next: AgentLanguage) => {
    setSaving(true);
    api
      .setSettings({ agent_language: next })
      .then(() => {
        setValue(next);
        toast.success(
          t("Agent language set to {language}", { language: languages.find((l) => l.value === next)?.label ?? next }),
        );
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSaving(false));
  };

  const current = languages.find((l) => l.value === value);
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <LanguagesIcon className="size-4" />
          {t("Agent language")}
        </CardTitle>
        <CardDescription>
          {t(
            "Language of everything the agents write for people: chat replies, plans, facts, findings and reports. Commands, payloads, URLs, tool output and identifiers stay as they are. You can write to the agents in Spanish or English either way. Applies from the next model call, no restart.",
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="agent-language" className="text-sm font-normal text-muted-foreground">
            {t("Language")}
          </Label>
          <Select value={value} disabled={!value || saving} onValueChange={(v) => save(v as AgentLanguage)}>
            <SelectTrigger id="agent-language" className="w-48 shrink-0">
              <SelectValue placeholder={t("Loading…")} />
            </SelectTrigger>
            <SelectContent>
              {languages.map((l) => (
                <SelectItem key={l.value} value={l.value}>
                  {l.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {current && <p className="text-muted-foreground text-xs">{current.hint}</p>}
      </CardContent>
    </Card>
  );
}
