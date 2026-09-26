"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import {
  Bell,
  BellOff,
  LogOut,
  MoreHorizontal,
  ShieldCheck,
  ShieldOff,
  UserMinus,
  UserPlus,
  Users,
  X,
} from "lucide-react";

import { setConversationMuted, type SearchUserItem } from "@/lib/api";
import {
  addCommunityMembers,
  leaveCommunity,
  listCommunityMembers,
  removeCommunityMember,
  setCommunityMemberRole,
  type ApiCommunity,
  type ApiCommunityMember,
  type CommunityRole,
} from "@/lib/community-api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { CommunityAvatar } from "./CommunityAvatar";
import { ConfirmDialog } from "./ConfirmDialog";
import { PeoplePicker } from "./PeoplePicker";

type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="36" height="36"><circle cx="18" cy="18" r="18" fill="#d4d4d8"/></svg>',
  );

export const ROLE_KEY: Record<CommunityRole, TranslationKey> = {
  owner: "community.roleOwner",
  admin: "community.roleAdmin",
  member: "community.roleMember",
};

/** Owner/admin get a visible pill; members a quiet label. */
export function RoleBadge({ role }: { role: CommunityRole }) {
  const { t } = useLanguage();
  if (role === "member") {
    return <span className="shrink-0 text-xs text-muted-soft">{t(ROLE_KEY[role])}</span>;
  }
  return (
    <span
      className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-semibold ${
        role === "owner" ? "bg-primary text-white" : "bg-primary-soft text-primary"
      }`}
    >
      {t(ROLE_KEY[role])}
    </span>
  );
}


/**
 * Details for a group or channel: identity, mute, leave, and — only where the
 * server grants the permission — member management. Rendered as a slide-over
 * on the thread pane (full width on mobile) so the thread never gets squeezed.
 */
export function CommunityDetails({
  community,
  currentUserId,
  onClose,
  onChange,
  onLeft,
}: {
  community: ApiCommunity;
  currentUserId?: string;
  onClose: () => void;
  onChange: (next: ApiCommunity) => void;
  onLeft: (id: string) => void;
}) {
  const { t } = useLanguage();
  const kind = community.type;
  const perms = community.permissions;

  const [muting, setMuting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [leaveError, setLeaveError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);

  const [members, setMembers] = useState<ApiCommunityMember[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      listCommunityMembers(kind, community.id, {}, signal)
        .then((page) => {
          setMembers(page.items);
          setNextCursor(page.nextCursor);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    },
    [community.id, kind],
  );

  useEffect(() => {
    if (!perms.canViewMembers) return;
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load, perms.canViewMembers]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    listCommunityMembers(kind, community.id, { cursor: nextCursor })
      .then((page) => {
        setMembers((prev) => {
          const seen = new Set(prev.map((m) => m.user.id));
          return [...prev, ...page.items.filter((m) => !seen.has(m.user.id))];
        });
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have; the button stays available to retry.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const toggleMute = async () => {
    if (muting) return;
    setMuting(true);
    setActionError(null);
    try {
      const result = await setConversationMuted(community.id, !community.muted);
      onChange({ ...community, muted: result.muted });
    } catch {
      setActionError(t("community.actionError"));
    } finally {
      setMuting(false);
    }
  };

  // Backend error bodies are untranslated English ("not allowed"), so failures
  // always show the localized actionError message.
  const leave = async () => {
    if (leaving) return;
    setLeaving(true);
    setLeaveError(null);
    try {
      await leaveCommunity(kind, community.id);
      onLeft(community.id);
    } catch {
      setLeaveError(t("community.actionError"));
      setLeaving(false);
    }
  };

  const bumpCount = (delta: number) => {
    if (community.memberCount != null) {
      onChange({ ...community, memberCount: Math.max(0, community.memberCount + delta) });
    }
  };

  const memberLabel = t(kind === "channel" ? "community.subscribers" : "community.members");

  return (
    <aside
      aria-label={t("community.info")}
      className="absolute inset-y-0 right-0 z-10 flex w-full max-w-sm flex-col border-border bg-surface shadow-xl sm:border-l"
    >
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">
          {t("community.info")}
        </h2>
        <button
          type="button"
          onClick={onClose}
          aria-label={t("profile.close")}
          className="flex h-8 w-8 items-center justify-center rounded-full text-muted transition-colors hover:bg-background"
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col items-center px-5 pb-4 pt-5 text-center">
          <CommunityAvatar kind={kind} name={community.name} avatarUrl={community.avatarUrl} size="lg" />
          <h3 className="mt-3 max-w-full break-words text-base font-semibold text-foreground">
            {community.name}
          </h3>
          <p className="mt-0.5 flex items-center gap-1.5 text-xs text-muted">
            <span>{t(kind === "channel" ? "community.kindChannel" : "community.kindGroup")}</span>
            {community.memberCount != null ? (
              <>
                <span aria-hidden>·</span>
                <Users className="h-3.5 w-3.5" aria-hidden />
                <span className="tabular-nums" aria-label={`${memberLabel}: ${community.memberCount}`}>
                  {community.memberCount}
                </span>
              </>
            ) : null}
            {community.role ? (
              <>
                <span aria-hidden>·</span>
                <RoleBadge role={community.role} />
              </>
            ) : null}
          </p>
          {community.description ? (
            <p className="mt-3 max-w-full whitespace-pre-wrap break-words text-sm text-foreground/90">
              {community.description}
            </p>
          ) : null}
        </div>

        <div className="flex flex-col gap-1 border-t border-border px-3 py-2">
          <button
            type="button"
            onClick={() => void toggleMute()}
            disabled={muting}
            className="flex items-center gap-3 rounded-xl px-2 py-2 text-sm text-foreground transition-colors hover:bg-background disabled:opacity-50"
          >
            {community.muted ? <Bell className="h-4 w-4 text-muted" /> : <BellOff className="h-4 w-4 text-muted" />}
            {community.muted ? t("community.unmute") : t("community.mute")}
          </button>
          {/* The backend rejects every owner leave (no ownership transfer in
              V1), so it is hidden for owners even though canLeave is true. */}
          {perms.canLeave && community.role !== "owner" ? (
            <button
              type="button"
              onClick={() => {
                setLeaveError(null);
                setConfirmLeave(true);
              }}
              className="flex items-center gap-3 rounded-xl px-2 py-2 text-sm text-red-600 transition-colors hover:bg-red-50"
            >
              <LogOut className="h-4 w-4" />
              {t(kind === "channel" ? "community.leaveChannel" : "community.leaveGroup")}
            </button>
          ) : null}
          {actionError ? (
            <p className="px-2 text-xs text-red-500" role="alert">
              {actionError}
            </p>
          ) : null}
        </div>

        {perms.canViewMembers ? (
          <section className="border-t border-border px-3 py-3">
            <div className="flex items-center justify-between px-2 pb-2">
              <h4 className="text-xs font-semibold uppercase tracking-wide text-muted">{memberLabel}</h4>
              {perms.canAddMembers && kind === "group" ? (
                <button
                  type="button"
                  onClick={() => setAdding(true)}
                  className="flex items-center gap-1 rounded-full px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary-soft"
                >
                  <UserPlus className="h-3.5 w-3.5" />
                  {t("community.add")}
                </button>
              ) : null}
            </div>

            {status === "loading" ? (
              <p className="py-6 text-center text-sm text-muted">{t("feed.loadingMore")}</p>
            ) : null}
            {status === "error" ? (
              <div className="py-6 text-center">
                <p className="text-sm text-muted">{t("community.membersError")}</p>
                <button type="button" onClick={() => load()} className="mt-1 text-sm text-primary hover:underline">
                  {t("search.tryAgain")}
                </button>
              </div>
            ) : null}
            {status === "ready" ? (
              <ul className="flex flex-col">
                {members.map((m) => (
                  <MemberRow
                    key={m.user.id}
                    community={community}
                    member={m}
                    isSelf={m.user.id === currentUserId}
                    onUpdated={(next) =>
                      setMembers((prev) => prev.map((x) => (x.user.id === next.user.id ? next : x)))
                    }
                    onRemoved={(userId) => {
                      setMembers((prev) => prev.filter((x) => x.user.id !== userId));
                      bumpCount(-1);
                    }}
                  />
                ))}
              </ul>
            ) : null}
            {status === "ready" && nextCursor ? (
              <button
                type="button"
                onClick={loadMore}
                disabled={loadingMore}
                className="mt-1 w-full rounded-xl px-2 py-2 text-center text-sm text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"
              >
                {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
              </button>
            ) : null}
          </section>
        ) : null}
      </div>

      {confirmLeave ? (
        <ConfirmDialog
          title={t(kind === "channel" ? "community.leaveChannel" : "community.leaveGroup")}
          body={t(kind === "channel" ? "community.leaveChannelBody" : "community.leaveGroupBody")}
          confirmLabel={t("community.leave")}
          pendingLabel={t("community.leaving")}
          pending={leaving}
          error={leaveError}
          onConfirm={() => void leave()}
          onCancel={() => setConfirmLeave(false)}
        />
      ) : null}

      {adding ? (
        <AddMembersDialog
          community={community}
          excludeIds={new Set([...members.map((m) => m.user.id), ...(currentUserId ? [currentUserId] : [])])}
          onClose={() => setAdding(false)}
          onAdded={(added) => {
            setMembers((prev) => {
              const seen = new Set(prev.map((m) => m.user.id));
              return [...prev, ...added.filter((m) => !seen.has(m.user.id))];
            });
            bumpCount(added.length);
            setAdding(false);
          }}
        />
      ) : null}
    </aside>
  );
}

function MemberRow({
  community,
  member,
  isSelf,
  onUpdated,
  onRemoved,
}: {
  community: ApiCommunity;
  member: ApiCommunityMember;
  isSelf: boolean;
  onUpdated: (next: ApiCommunityMember) => void;
  onRemoved: (userId: string) => void;
}) {
  const { t } = useLanguage();
  const perms = community.permissions;
  const [menuOpen, setMenuOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

  // Server permissions gate every action. The role checks below only hide
  // actions that can never apply (e.g. acting on the owner or on yourself);
  // the backend remains the final authority and its refusal is shown.
  const target = member.role;
  const canPromote = perms.canManageAdmins && !isSelf && target === "member";
  const canDemote = perms.canManageAdmins && !isSelf && target === "admin";
  const canRemove =
    perms.canRemoveMembers &&
    !isSelf &&
    target !== "owner" &&
    (target === "member" || community.role === "owner");
  const hasActions = canPromote || canDemote || canRemove;

  const changeRole = async (role: "admin" | "member") => {
    if (pending) return;
    setMenuOpen(false);
    setPending(true);
    setError(null);
    try {
      onUpdated(await setCommunityMemberRole(community.type, community.id, member.user.id, role));
    } catch {
      setError(t("community.actionError"));
    } finally {
      setPending(false);
    }
  };

  const remove = async () => {
    if (pending) return;
    setPending(true);
    setError(null);
    try {
      await removeCommunityMember(community.type, community.id, member.user.id);
      setConfirmRemove(false);
      onRemoved(member.user.id);
    } catch {
      setError(t("community.actionError"));
      setPending(false);
    }
  };

  return (
    <li className="rounded-xl">
      <div className="flex items-center gap-3 px-2 py-2">
        <Link
          href={`/u/${encodeURIComponent(member.user.username)}`}
          className="flex min-w-0 flex-1 items-center gap-3 rounded-lg hover:opacity-90"
        >
          <Image
            src={member.user.avatarUrl ?? FALLBACK_AVATAR}
            alt=""
            width={36}
            height={36}
            unoptimized={Boolean(member.user.avatarUrl)}
            className="h-9 w-9 shrink-0 rounded-full object-cover"
          />
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1.5">
              <span className="truncate text-sm font-medium text-foreground">{member.user.displayName}</span>
              {isSelf ? <span className="shrink-0 text-xs text-muted-soft">({t("community.you")})</span> : null}
            </span>
            <span className="block truncate text-xs text-muted">@{member.user.username}</span>
          </span>
        </Link>
        <RoleBadge role={member.role} />
        {hasActions ? (
          <div className="relative" ref={menuRef}>
            <button
              type="button"
              onClick={() => setMenuOpen((v) => !v)}
              disabled={pending}
              aria-haspopup="menu"
              aria-expanded={menuOpen}
              aria-label={t("community.memberActions", { name: member.user.displayName })}
              className="flex h-8 w-8 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"
            >
              <MoreHorizontal className="h-4 w-4" />
            </button>
            {menuOpen ? (
              <div
                role="menu"
                className="absolute right-0 top-full z-20 mt-1 w-44 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
              >
                {canPromote ? (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => void changeRole("admin")}
                    className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background"
                  >
                    <ShieldCheck className="h-4 w-4 text-muted" />
                    {t("community.makeAdmin")}
                  </button>
                ) : null}
                {canDemote ? (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => void changeRole("member")}
                    className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background"
                  >
                    <ShieldOff className="h-4 w-4 text-muted" />
                    {t("community.removeAdmin")}
                  </button>
                ) : null}
                {canRemove ? (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setMenuOpen(false);
                      setError(null);
                      setConfirmRemove(true);
                    }}
                    className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-red-600 transition-colors hover:bg-red-50"
                  >
                    <UserMinus className="h-4 w-4" />
                    {t("community.removeMember")}
                  </button>
                ) : null}
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
      {error && !confirmRemove ? (
        <p className="px-2 pb-1 text-xs text-red-500" role="alert">
          {error}
        </p>
      ) : null}
      {confirmRemove ? (
        <ConfirmDialog
          title={t("community.removeMemberTitle", { name: member.user.displayName })}
          body={t("community.removeMemberBody")}
          confirmLabel={t("community.removeMember")}
          pendingLabel={t("community.removing")}
          pending={pending}
          error={error}
          onConfirm={() => void remove()}
          onCancel={() => setConfirmRemove(false)}
        />
      ) : null}
    </li>
  );
}

function AddMembersDialog({
  community,
  excludeIds,
  onClose,
  onAdded,
}: {
  community: ApiCommunity;
  excludeIds: ReadonlySet<string>;
  onClose: () => void;
  onAdded: (added: ApiCommunityMember[]) => void;
}) {
  const { t } = useLanguage();
  const [selected, setSelected] = useState<SearchUserItem[]>([]);
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

  const submit = async () => {
    if (selected.length === 0 || submittingRef.current) return;
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const result = await addCommunityMembers(community.type, community.id, selected.map((u) => u.id));
      onAdded(result.items ?? []);
    } catch {
      setError(t("community.actionError"));
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-label={t("community.addMembers")}
      onMouseDown={() => {
        if (!submittingRef.current) onClose();
      }}
    >
      <div
        onMouseDown={(e) => e.stopPropagation()}
        className="flex max-h-[85dvh] w-full flex-col overflow-hidden rounded-t-2xl border border-border bg-surface shadow-xl sm:max-w-md sm:rounded-2xl"
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">
            {t("community.addMembers")}
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
        <div className="flex min-h-0 flex-1 flex-col px-4 py-4">
          <PeoplePicker selected={selected} onChange={setSelected} excludeIds={excludeIds} disabled={submitting} />
        </div>
        <div className="border-t border-border px-4 py-3">
          {error ? (
            <p className="mb-2 text-xs text-red-500" role="alert">
              {error}
            </p>
          ) : null}
          <div className="flex items-center justify-between gap-3">
            <span className="min-w-0 truncate text-xs text-muted">
              {t("community.selectedCount", { count: selected.length })}
            </span>
            <button
              type="button"
              onClick={() => void submit()}
              disabled={submitting || selected.length === 0}
              className="shrink-0 rounded-full bg-primary px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {submitting ? t("community.adding") : t("community.add")}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
