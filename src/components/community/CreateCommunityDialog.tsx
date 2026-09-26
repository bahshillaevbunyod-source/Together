"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronLeft, Megaphone, Users, X } from "lucide-react";

import type { SearchUserItem } from "@/lib/api";
import {
  createCommunity,
  isCommunityApiUnavailable,
  type ApiCommunity,
  type CommunityKind,
} from "@/lib/community-api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { PeoplePicker } from "./PeoplePicker";

const NAME_MAX = 80;
const DESCRIPTION_MAX = 500;

const KINDS: {
  kind: CommunityKind;
  icon: typeof Users;
  labelKey: TranslationKey;
  hintKey: TranslationKey;
}[] = [
  { kind: "group", icon: Users, labelKey: "community.kindGroup", hintKey: "community.kindGroupHint" },
  { kind: "channel", icon: Megaphone, labelKey: "community.kindChannel", hintKey: "community.kindChannelHint" },
];

/**
 * Lightweight create flow: kind + basics on one step, then (groups only) pick
 * initial members and confirm. The dialog only closes on a real server
 * response; nothing is created locally.
 */
export function CreateCommunityDialog({
  initialKind,
  currentUserId,
  onClose,
  onCreated,
}: {
  initialKind: CommunityKind;
  currentUserId?: string;
  onClose: () => void;
  onCreated: (community: ApiCommunity) => void;
}) {
  const { t } = useLanguage();
  const [kind, setKind] = useState<CommunityKind>(initialKind);
  const [step, setStep] = useState<"basics" | "members">("basics");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [members, setMembers] = useState<SearchUserItem[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const submittingRef = useRef(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !submittingRef.current) onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const trimmedName = name.trim();
  const needsMembersStep = kind === "group";
  const onLastStep = !needsMembersStep || step === "members";

  const submit = async () => {
    if (!trimmedName || submittingRef.current) return;
    if (kind === "group" && members.length === 0) return;
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const created = await createCommunity(kind, {
        name: trimmedName,
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(kind === "group" ? { memberIds: members.map((m) => m.id) } : {}),
      });
      onCreated(created);
    } catch (err) {
      setError(
        isCommunityApiUnavailable(err)
          ? t(kind === "group" ? "groups.unavailable" : "channels.unavailable")
          : t("community.createError"),
      );
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };

  const onPrimary = (e: React.FormEvent) => {
    e.preventDefault();
    if (!trimmedName) {
      setError(t("community.nameRequired"));
      return;
    }
    if (!onLastStep) {
      setError(null);
      setStep("members");
      return;
    }
    void submit();
  };

  const exclude = new Set(currentUserId ? [currentUserId] : []);

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-label={t("community.create")}
      onMouseDown={() => {
        if (!submittingRef.current) onClose();
      }}
    >
      <form
        onSubmit={onPrimary}
        onMouseDown={(e) => e.stopPropagation()}
        className="flex max-h-[92dvh] w-full flex-col overflow-hidden rounded-t-2xl border border-border bg-surface shadow-xl sm:max-h-[85dvh] sm:max-w-md sm:rounded-2xl"
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          {step === "members" ? (
            <button
              type="button"
              onClick={() => setStep("basics")}
              disabled={submitting}
              aria-label={t("community.back")}
              className="flex h-8 w-8 items-center justify-center rounded-full text-muted transition-colors hover:bg-background disabled:opacity-50"
            >
              <ChevronLeft className="h-5 w-5" />
            </button>
          ) : null}
          <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">
            {step === "members" ? t("community.addMembers") : t("community.create")}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            aria-label={t("profile.close")}
            className="flex h-8 w-8 items-center justify-center rounded-full text-muted transition-colors hover:bg-background disabled:opacity-50"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
          {step === "basics" ? (
            <div className="flex flex-col gap-4">
              <div role="radiogroup" aria-label={t("community.create")} className="grid grid-cols-2 gap-2">
                {KINDS.map(({ kind: k, icon: Icon, labelKey, hintKey }) => {
                  const active = kind === k;
                  return (
                    <button
                      key={k}
                      type="button"
                      role="radio"
                      aria-checked={active}
                      onClick={() => setKind(k)}
                      disabled={submitting}
                      className={`flex flex-col items-start gap-1 rounded-xl border px-3 py-2.5 text-left transition-colors disabled:opacity-50 ${
                        active
                          ? "border-primary bg-primary-soft"
                          : "border-border hover:bg-background"
                      }`}
                    >
                      <span className={`flex items-center gap-1.5 text-sm font-semibold ${active ? "text-primary" : "text-foreground"}`}>
                        <Icon className="h-4 w-4" />
                        {t(labelKey)}
                      </span>
                      <span className="text-xs leading-snug text-muted">{t(hintKey)}</span>
                    </button>
                  );
                })}
              </div>

              <label className="block">
                <span className="mb-1 block text-xs font-medium text-muted">{t("community.name")}</span>
                <input
                  autoFocus
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  maxLength={NAME_MAX}
                  disabled={submitting}
                  className="h-10 w-full rounded-xl border border-border bg-surface px-3 text-sm text-foreground placeholder:text-muted-soft focus:border-primary disabled:opacity-50"
                />
              </label>

              <label className="block">
                <span className="mb-1 flex items-center justify-between text-xs font-medium text-muted">
                  <span>{t("community.description")}</span>
                  <span className="tabular-nums text-muted-soft">
                    {description.length}/{DESCRIPTION_MAX}
                  </span>
                </span>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  maxLength={DESCRIPTION_MAX}
                  rows={3}
                  disabled={submitting}
                  placeholder={t("community.descriptionPlaceholder")}
                  className="w-full resize-none rounded-xl border border-border bg-surface px-3 py-2 text-sm text-foreground placeholder:text-muted-soft focus:border-primary disabled:opacity-50"
                />
              </label>
            </div>
          ) : (
            <PeoplePicker
              selected={members}
              onChange={setMembers}
              excludeIds={exclude}
              disabled={submitting}
            />
          )}
        </div>

        <div className="border-t border-border px-4 py-3">
          {error ? (
            <p className="mb-2 text-xs text-red-500" role="alert">
              {error}
            </p>
          ) : null}
          <div className="flex items-center justify-between gap-3">
            <span className="min-w-0 truncate text-xs text-muted">
              {step === "members" ? t("community.selectedCount", { count: members.length }) : null}
            </span>
            <button
              type="submit"
              // A group needs at least one other member: the backend cannot
              // deliver messages in a group with only its owner.
              disabled={submitting || !trimmedName || (kind === "group" && step === "members" && members.length === 0)}
              className="shrink-0 rounded-full bg-primary px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {submitting
                ? t("community.creating")
                : onLastStep
                  ? t(kind === "group" ? "community.createGroup" : "community.createChannel")
                  : t("community.next")}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
