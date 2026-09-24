import Image from "next/image";
import { Megaphone, Users } from "lucide-react";

import type { CommunityKind } from "@/lib/community-api";

const SIZE = {
  sm: { px: 36, box: "h-9 w-9", icon: "h-4 w-4" },
  md: { px: 48, box: "h-12 w-12", icon: "h-5 w-5" },
  lg: { px: 72, box: "h-[72px] w-[72px]", icon: "h-7 w-7" },
} as const;

/**
 * Group/channel avatar. Uses the server-provided image when present; otherwise
 * a neutral kind glyph (never a fabricated picture). Rounded-square for groups
 * and channels so they read differently from people (circles).
 */
export function CommunityAvatar({
  kind,
  name,
  avatarUrl,
  size = "md",
}: {
  kind: CommunityKind;
  name: string;
  avatarUrl: string | null;
  size?: keyof typeof SIZE;
}) {
  const s = SIZE[size];
  if (avatarUrl) {
    return (
      <Image
        src={avatarUrl}
        alt={name}
        width={s.px}
        height={s.px}
        unoptimized
        className={`${s.box} shrink-0 rounded-2xl object-cover`}
      />
    );
  }
  const Icon = kind === "channel" ? Megaphone : Users;
  return (
    <span
      aria-hidden
      className={`${s.box} flex shrink-0 items-center justify-center rounded-2xl bg-primary-soft text-primary`}
    >
      <Icon className={s.icon} />
    </span>
  );
}
