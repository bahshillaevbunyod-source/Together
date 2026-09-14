"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { ApiError, getProfile, updateProfile } from "@/lib/api";

type Status = "loading" | "ready" | "error";

// Map backend failures to safe, friendly text (never surface raw error detail).
function friendlyProfileError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.status) {
      case 400:
        return "Please check the profile fields and try again.";
      case 401:
        return "Your session has expired. Please sign in again.";
      case 409:
        return "Some of those details are already in use.";
    }
  }
  return "Couldn’t update your profile. Try again.";
}

export default function EditProfilePage() {
  const router = useRouter();

  const [status, setStatus] = useState<Status>("loading");
  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [countryCode, setCountryCode] = useState("");
  const [city, setCity] = useState("");
  const [nativeLanguage, setNativeLanguage] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const inputClass =
    "h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getProfile(signal)
      .then((p) => {
        setDisplayName(p.displayName);
        setBio(p.bio ?? "");
        setCountryCode(p.countryCode ?? "");
        setCity(p.city ?? "");
        setNativeLanguage(p.nativeLanguage);
        setAvatarUrl(p.avatarUrl ?? "");
        setStatus("ready");
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const onSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving) return;

    const name = displayName.trim();
    const lang = nativeLanguage.trim();
    if (!name) {
      setError("Display name is required.");
      return;
    }
    if (lang.length < 2) {
      setError("Native language is required.");
      return;
    }

    const trimOrNull = (v: string) => (v.trim() === "" ? null : v.trim());

    setSaving(true);
    setError(null);
    try {
      await updateProfile({
        displayName: name,
        nativeLanguage: lang,
        bio: trimOrNull(bio),
        city: trimOrNull(city),
        avatarUrl: trimOrNull(avatarUrl),
        countryCode:
          countryCode.trim() === "" ? null : countryCode.trim().toUpperCase(),
      });
      router.push("/profile");
    } catch (err) {
      setError(friendlyProfileError(err)); // keep entered values on failure
    } finally {
      setSaving(false);
    }
  };

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">Loading profile…</p>
    );
  }

  if (status === "error") {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">Couldn’t load your profile.</p>
        <button
          type="button"
          onClick={() => load()}
          className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          Try again
        </button>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl">
      <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <h1 className="text-lg font-bold tracking-tight text-foreground">
          Edit profile
        </h1>

        <form onSubmit={onSave} className="mt-5 flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-foreground">
              Display name
            </span>
            <input
              className={inputClass}
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Your name"
            />
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-foreground">Bio</span>
            <textarea
              value={bio}
              onChange={(e) => setBio(e.target.value)}
              placeholder="A short bio"
              rows={3}
              className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            />
          </label>

          <div className="flex gap-4">
            <label className="flex flex-1 flex-col gap-1.5">
              <span className="text-sm font-medium text-foreground">City</span>
              <input
                className={inputClass}
                type="text"
                value={city}
                onChange={(e) => setCity(e.target.value)}
                placeholder="City"
              />
            </label>
            <label className="flex w-32 flex-col gap-1.5">
              <span className="text-sm font-medium text-foreground">
                Country
              </span>
              <input
                className={inputClass}
                type="text"
                value={countryCode}
                onChange={(e) => setCountryCode(e.target.value)}
                placeholder="US"
                maxLength={2}
              />
            </label>
          </div>

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-foreground">
              Native language
            </span>
            <input
              className={inputClass}
              type="text"
              value={nativeLanguage}
              onChange={(e) => setNativeLanguage(e.target.value)}
              placeholder="e.g. en"
            />
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-foreground">
              Avatar URL
            </span>
            <input
              className={inputClass}
              type="text"
              value={avatarUrl}
              onChange={(e) => setAvatarUrl(e.target.value)}
              placeholder="https://…"
            />
          </label>

          {error ? (
            <p className="text-sm text-red-500" role="alert">
              {error}
            </p>
          ) : null}

          <div className="mt-1 flex items-center gap-3">
            <button
              type="submit"
              disabled={saving}
              className="rounded-full bg-primary px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {saving ? "Saving…" : "Save"}
            </button>
            <Link
              href="/profile"
              className="rounded-full border border-border px-5 py-2 text-sm text-muted transition-colors hover:text-foreground"
            >
              Cancel
            </Link>
          </div>
        </form>
      </section>
    </div>
  );
}
