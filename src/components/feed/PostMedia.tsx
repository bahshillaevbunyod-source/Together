"use client";

import { useRef, useState } from "react";
import Image from "next/image";

import type { PostMedia as PostMediaItem } from "@/types/post";

// Real rendered media width (measured): ~640px on desktop, ~full width on
// mobile. A little headroom keeps DPR 2/3 screens sharp.
const SIZES = "(max-width: 1024px) 92vw, 680px";
const QUALITY = 85;

// Bounded aspect range: portrait cap 4:5, landscape cap 16:9. Images inside this
// range render at their true ratio; extremes are clamped (letterboxed) so a card
// never becomes absurdly tall or wide.
const MIN_RATIO = 4 / 5; // 0.8
const MAX_RATIO = 16 / 9; // ~1.778

function clampRatio(r: number): number {
  if (!isFinite(r) || r <= 0) return 1;
  return Math.min(MAX_RATIO, Math.max(MIN_RATIO, r));
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/** Media block for a post: a single natural-ratio image, or a carousel for 2+. */
export function PostMedia({ media }: { media: PostMediaItem[] }) {
  if (media.length === 0) return null;
  if (media.length === 1) return <SingleImage item={media[0]} />;
  return <Carousel media={media} />;
}

// SingleImage renders one image full-bleed at its natural aspect ratio, bounded
// to the [4:5, 16:9] range. It always uses object-cover: in-range images fill
// exactly (no crop); extremes take a small intentional crop rather than showing
// letterbox gutters. The original R2 file is never modified.
function SingleImage({ item }: { item: PostMediaItem }) {
  const [ratio, setRatio] = useState<number | null>(null);

  const onLoad = (e: React.SyntheticEvent<HTMLImageElement>) => {
    const img = e.currentTarget;
    const raw = img.naturalWidth / img.naturalHeight;
    if (!isFinite(raw) || raw <= 0) return;
    setRatio(clampRatio(raw));
  };

  return (
    <div
      className="relative mt-3 w-full overflow-hidden rounded-xl border border-border bg-background"
      style={{ aspectRatio: ratio ?? 1 }}
    >
      <Image
        src={item.src}
        alt={item.alt}
        fill
        sizes={SIZES}
        quality={QUALITY}
        onLoad={onLoad}
        className="object-cover"
      />
    </div>
  );
}

// Carousel renders 2–8 images in a native scroll-snap track with arrows, an
// index pill, and dot/progress indicators. All slides share one frame aspect
// (from the first image, clamped) so the track height is stable.
function Carousel({ media }: { media: PostMediaItem[] }) {
  const n = media.length;
  const trackRef = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(0);
  const [frameRatio, setFrameRatio] = useState<number | null>(null);

  const onFirstLoad = (e: React.SyntheticEvent<HTMLImageElement>) => {
    const img = e.currentTarget;
    const raw = img.naturalWidth / img.naturalHeight;
    if (isFinite(raw) && raw > 0) setFrameRatio(clampRatio(raw));
  };

  // Derive the active index from the real scroll position, so it stays correct
  // whether the user swipes, trackpad-scrolls, or clicks an arrow/dot.
  const onScroll = () => {
    const el = trackRef.current;
    if (!el) return;
    const idx = Math.round(el.scrollLeft / el.clientWidth);
    setActive(Math.max(0, Math.min(n - 1, idx)));
  };

  const goTo = (idx: number) => {
    const el = trackRef.current;
    if (!el) return;
    const clamped = Math.max(0, Math.min(n - 1, idx));
    el.scrollTo({
      left: clamped * el.clientWidth,
      behavior: prefersReducedMotion() ? "auto" : "smooth",
    });
  };

  // Keyboard navigation (no visible controls): arrows move slides only while the
  // carousel itself is focused, so normal page scrolling is never hijacked.
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      goTo(active - 1);
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      goTo(active + 1);
    }
  };

  const ratio = frameRatio ?? 1;

  return (
    <div className="relative mt-3">
      <div
        ref={trackRef}
        onScroll={onScroll}
        onKeyDown={onKeyDown}
        tabIndex={0}
        role="group"
        aria-roledescription="carousel"
        aria-label={`Post images, ${n} total`}
        className="flex snap-x snap-mandatory overflow-x-auto overflow-y-hidden rounded-xl border border-border outline-none focus-visible:ring-2 focus-visible:ring-primary [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {media.map((m, i) => (
          <div key={i} className="relative w-full shrink-0 basis-full snap-start">
            <div className="relative w-full bg-background" style={{ aspectRatio: ratio }}>
              <Image
                src={m.src}
                alt={`${m.alt} (${i + 1} of ${n})`}
                fill
                sizes={SIZES}
                quality={QUALITY}
                onLoad={i === 0 ? onFirstLoad : undefined}
                className="object-cover"
              />
            </div>
          </div>
        ))}
      </div>

      {/* Position indicator: subtle dots for a few images, a thin bar for many.
          A small drop-shadow keeps them visible over light photos. */}
      {n <= 5 ? (
        <div className="pointer-events-none absolute bottom-2 left-1/2 flex -translate-x-1/2 items-center gap-1.5 [filter:drop-shadow(0_1px_1.5px_rgb(0_0_0/0.45))]">
          {media.map((_, i) => (
            <button
              key={i}
              type="button"
              aria-label={`Go to image ${i + 1}`}
              onClick={() => goTo(i)}
              className={`pointer-events-auto h-1.5 rounded-full transition-all ${
                i === active ? "w-4 bg-white" : "w-1.5 bg-white/70 hover:bg-white"
              }`}
            />
          ))}
        </div>
      ) : (
        <div className="pointer-events-none absolute bottom-2 left-1/2 h-1 w-16 -translate-x-1/2 overflow-hidden rounded-full bg-white/40 [filter:drop-shadow(0_1px_1.5px_rgb(0_0_0/0.45))]">
          <div
            className="h-full rounded-full bg-white transition-all"
            style={{ width: `${((active + 1) / n) * 100}%` }}
          />
        </div>
      )}
    </div>
  );
}
