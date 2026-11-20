"use client";

import * as React from "react";

import { KeyRoundIcon, PlusIcon, RefreshCwIcon, Trash2Icon, UsersIcon } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { intlLocale, t } from "@/i18n";
import { api } from "@/lib/api";
import type { SessionAccount, UserAccount, UserAuditEntry } from "@/lib/types";

const MIN_PASSWORD = 12;

// The bootstrap account carries zero timestamps; show a dash instead of year 1.
function when(ts?: string) {
  if (!ts) return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "—";
  return d.toLocaleString(intlLocale, { hour12: false });
}

function passwordProblem(password: string, confirm: string): string | null {
  if (password.length < MIN_PASSWORD) return t("Password must be at least 12 characters");
  if (password !== confirm) return t("The two passwords do not match");
  return null;
}

type PasswordDialogState = { mode: "create" } | { mode: "reset"; user: UserAccount } | null;

export default function UsersPage() {
  const [users, setUsers] = React.useState<UserAccount[]>([]);
  const [me, setMe] = React.useState<SessionAccount | null>(null);
  const [audit, setAudit] = React.useState<UserAuditEntry[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [auditLoading, setAuditLoading] = React.useState(false);
  const [busyId, setBusyId] = React.useState<number | null>(null);

  const [dialog, setDialog] = React.useState<PasswordDialogState>(null);
  const [formUser, setFormUser] = React.useState("");
  const [formPass, setFormPass] = React.useState("");
  const [formConfirm, setFormConfirm] = React.useState("");
  const [formError, setFormError] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const [toDelete, setToDelete] = React.useState<UserAccount | null>(null);

  const loadUsers = React.useCallback(() => {
    setLoading(true);
    Promise.all([api.users(), api.me()])
      .then(([list, current]) => {
        setUsers(list.users);
        setMe(current);
      })
      .catch((e) => toast.error(t("Failed to load: {error}", { error: (e as Error).message })))
      .finally(() => setLoading(false));
  }, []);

  const loadAudit = React.useCallback(() => {
    setAuditLoading(true);
    api
      .userAudit(100)
      .then((r) => setAudit(r.entries))
      .catch((e) => toast.error(t("Failed to load: {error}", { error: (e as Error).message })))
      .finally(() => setAuditLoading(false));
  }, []);

  React.useEffect(() => {
    loadUsers();
    loadAudit();
  }, [loadUsers, loadAudit]);

  const openDialog = (next: PasswordDialogState) => {
    setFormUser("");
    setFormPass("");
    setFormConfirm("");
    setFormError("");
    setDialog(next);
  };

  const submitDialog = async () => {
    if (!dialog) return;
    if (dialog.mode === "create" && formUser.trim().length < 3) {
      setFormError(
        t(
          "Username must be 3 to 32 characters: letters, digits, dot, dash or underscore, starting with a letter or digit",
        ),
      );
      return;
    }
    const problem = passwordProblem(formPass, formConfirm);
    if (problem) {
      setFormError(problem);
      return;
    }
    setSaving(true);
    setFormError("");
    try {
      if (dialog.mode === "create") {
        const created = await api.createUser(formUser.trim(), formPass);
        toast.success(t("User {name} created", { name: created.username }));
      } else {
        await api.updateUser(dialog.user.id, { password: formPass });
        toast.success(t("Password of {name} reset; their open sessions were closed", { name: dialog.user.username }));
      }
      setDialog(null);
      loadUsers();
      loadAudit();
    } catch (e) {
      setFormError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const toggleDisabled = async (user: UserAccount, enabled: boolean) => {
    setBusyId(user.id);
    try {
      await api.updateUser(user.id, { disabled: !enabled });
      toast.success(
        enabled
          ? t("User {name} enabled", { name: user.username })
          : t("User {name} disabled; their open sessions were closed", { name: user.username }),
      );
      loadUsers();
      loadAudit();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusyId(null);
    }
  };

  const confirmDelete = async () => {
    if (!toDelete) return;
    const user = toDelete;
    setToDelete(null);
    setBusyId(user.id);
    try {
      await api.deleteUser(user.id);
      toast.success(t("User {name} deleted", { name: user.username }));
      loadUsers();
      loadAudit();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusyId(null);
    }
  };

  const isMe = (u: UserAccount) => me !== null && (u.owner ? me.owner : !me.owner && me.id === u.id);

  return (
    <div className="flex flex-1 flex-col gap-5 p-6">
      <div className="flex items-center gap-2.5">
        <UsersIcon className="h-5 w-5 shrink-0" />
        <div>
          <h1 className="text-lg font-semibold leading-tight">{t("Users")}</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            {t(
              "Every account has the same permissions and works on the same tasks. Approvals and changes are recorded under the account that made them.",
            )}
          </p>
        </div>
      </div>

      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {t(
            "The initial ARTEX account cannot be disabled or deleted; change its password from your account menu, or recover it with reset-password.sh on the server.",
          )}
        </p>
        <Button onClick={() => openDialog({ mode: "create" })} size="sm" className="shrink-0">
          <PlusIcon className="h-4 w-4" />
          {t("Create user")}
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          {loading ? (
            <p className="p-6 text-sm text-muted-foreground">{t("Loading…")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Username")}</TableHead>
                  <TableHead>{t("Created by")}</TableHead>
                  <TableHead>{t("Created at")}</TableHead>
                  <TableHead>{t("Last sign-in")}</TableHead>
                  <TableHead>{t("Enabled")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((u) => (
                  <TableRow key={u.owner ? "owner" : u.id}>
                    <TableCell className="font-medium">
                      <div className="flex flex-wrap items-center gap-1.5">
                        {u.username}
                        {u.owner && <Badge variant="secondary">{t("Initial account")}</Badge>}
                        {isMe(u) && <Badge variant="outline">{t("You")}</Badge>}
                        {u.disabled && (
                          <Badge variant="outline" className="border-amber-400 text-amber-600">
                            {t("Disabled")}
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{u.created_by || "—"}</TableCell>
                    <TableCell className="text-muted-foreground">{when(u.created_at)}</TableCell>
                    <TableCell className="text-muted-foreground">{when(u.last_login_at)}</TableCell>
                    <TableCell>
                      <Switch
                        checked={!u.disabled}
                        disabled={u.owner || isMe(u) || busyId === u.id}
                        onCheckedChange={(v) => toggleDisabled(u, v)}
                        aria-label={t("Enabled")}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      {!u.owner && (
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={busyId === u.id}
                            onClick={() => openDialog({ mode: "reset", user: u })}
                          >
                            <KeyRoundIcon className="h-4 w-4" />
                            {t("Reset password")}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive"
                            disabled={isMe(u) || busyId === u.id}
                            onClick={() => setToDelete(u)}
                            aria-label={t("Delete user {name}", { name: u.username })}
                          >
                            <Trash2Icon className="h-4 w-4" />
                          </Button>
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle className="text-base">{t("Recent activity")}</CardTitle>
            <CardDescription>
              {t(
                "Sign-ins and every change made through the panel or the API, newest first. Only the method, path and result are kept, never the content.",
              )}
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={loadAudit} disabled={auditLoading}>
            <RefreshCwIcon className={auditLoading ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
            {t("Refresh")}
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          {audit.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">{t("No activity records")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Time")}</TableHead>
                  <TableHead>{t("User")}</TableHead>
                  <TableHead>{t("Action")}</TableHead>
                  <TableHead>{t("Path")}</TableHead>
                  <TableHead>{t("Status")}</TableHead>
                  <TableHead>{t("Address")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {audit.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="whitespace-nowrap text-muted-foreground">{when(e.at)}</TableCell>
                    <TableCell className="font-medium">{e.username}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{e.action === "LOGIN" ? t("Sign-in") : e.action}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{e.path}</TableCell>
                    <TableCell>
                      <span className={e.status >= 400 ? "text-destructive" : "text-muted-foreground"}>{e.status}</span>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{e.ip}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={dialog !== null} onOpenChange={(open) => !open && setDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {dialog?.mode === "reset"
                ? t("Reset password of {name}", { name: dialog.user.username })
                : t("Create user")}
            </DialogTitle>
            <DialogDescription>
              {dialog?.mode === "reset"
                ? t("The new password replaces the old one and closes every open session of this account.")
                : t(
                    "The new account gets the same permissions as everyone else. Share the password through a safe channel.",
                  )}
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              void submitDialog();
            }}
          >
            {dialog?.mode === "create" && (
              <div className="space-y-1.5">
                <Label htmlFor="new-username">{t("Username")}</Label>
                <Input
                  id="new-username"
                  value={formUser}
                  onChange={(e) => setFormUser(e.target.value)}
                  placeholder={t("e.g. jdoe")}
                  autoComplete="off"
                  autoCapitalize="none"
                  spellCheck={false}
                  autoFocus
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="new-password">{t("Password")}</Label>
              <Input
                id="new-password"
                type="password"
                value={formPass}
                onChange={(e) => setFormPass(e.target.value)}
                placeholder={t("At least 12 characters")}
                autoComplete="new-password"
                autoFocus={dialog?.mode === "reset"}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="new-password-confirm">{t("Confirm password")}</Label>
              <Input
                id="new-password-confirm"
                type="password"
                value={formConfirm}
                onChange={(e) => setFormConfirm(e.target.value)}
                placeholder={t("Enter the password again")}
                autoComplete="new-password"
              />
            </div>
            {formError && <p className="text-sm text-destructive">{formError}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialog(null)}>
                {t("Cancel")}
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? t("Saving…") : dialog?.mode === "reset" ? t("Reset password") : t("Create")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={toDelete !== null} onOpenChange={(open) => !open && setToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("Delete user {name}?", { name: toDelete?.username ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                "The account can no longer sign in and its open sessions end at once. Its past approvals and activity stay recorded under its name.",
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("Cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmDelete}>{t("Delete")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
