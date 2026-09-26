"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";

import { ApiError, login, register } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useLanguage } from "@/lib/language-context";

type Mode = "login" | "register";

export default function LoginPage() {
  const router = useRouter();
  const { refresh } = useAuth();
  const { t } = useLanguage();

  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [nativeLanguage, setNativeLanguage] = useState("en");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const inputClass =
    // 16px text on phones prevents iOS Safari from zooming into focused inputs.
    "h-12 w-full min-w-0 rounded-xl border border-border bg-background px-4 text-base text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 sm:h-11 sm:text-sm";

  const switchMode = (next: Mode) => {
    setMode(next);
    setError(null);
  };

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submitting) return;

    // Minimal client-side validation; the backend is the source of truth.
    if (!email.trim() || !password) {
      setError(t("auth.emailPasswordRequired"));
      return;
    }
    if (mode === "register" && (!username.trim() || !displayName.trim())) {
      setError(t("auth.usernameDisplayNameRequired"));
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      if (mode === "login") {
        await login(email.trim(), password);
      } else {
        await register({
          email: email.trim(),
          username: username.trim(),
          displayName: displayName.trim(),
          nativeLanguage: nativeLanguage.trim() || "en",
          password,
        });
      }
      await refresh();
      if (mode === "register") {
        router.replace("/language");
      } else {
        router.push("/");
      }
    } catch (err) {
      // Show the backend's safe message, or a generic fallback.
      setError(err instanceof ApiError ? err.message : t("auth.genericError"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center bg-background pb-[max(2rem,env(safe-area-inset-bottom))] pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))] pt-[max(2rem,env(safe-area-inset-top))]">
      <div className="w-full max-w-md rounded-3xl border border-border bg-surface p-5 shadow-sm sm:p-8">
        <div className="mb-6 flex flex-col items-center gap-2 text-center">
          <Image
            src="/images/together-logo.png"
            alt="Together"
            width={48}
            height={48}
            className="h-12 w-12 object-contain"
          />
          <h1 className="text-xl font-bold tracking-tight text-foreground">
            {mode === "login"
              ? t("auth.welcomeBack")
              : t("auth.createAccountHeading")}
          </h1>
          <p className="text-sm text-muted">{t("header.tagline")}</p>
        </div>

        {/* Mode switch */}
        <div className="mb-5 flex rounded-full border border-border bg-background p-1 text-sm">
          <button
            type="button"
            onClick={() => switchMode("login")}
            className={`h-10 flex-1 rounded-full font-medium transition-colors ${
              mode === "login"
                ? "bg-primary text-white"
                : "text-muted hover:text-foreground"
            }`}
          >
            {t("auth.login")}
          </button>
          <button
            type="button"
            onClick={() => switchMode("register")}
            className={`h-10 flex-1 rounded-full font-medium transition-colors ${
              mode === "register"
                ? "bg-primary text-white"
                : "text-muted hover:text-foreground"
            }`}
          >
            {t("auth.signUp")}
          </button>
        </div>

        <form onSubmit={onSubmit} className="flex flex-col gap-3">
          {mode === "register" ? (
            <>
              <input
                className={inputClass}
                type="text"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder={t("edit.displayNameLabel")}
                autoComplete="name"
                enterKeyHint="next"
              />
              <input
                className={inputClass}
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={t("auth.username")}
                autoComplete="username"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                enterKeyHint="next"
              />
            </>
          ) : null}

          <input
            className={inputClass}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder={t("auth.email")}
            autoComplete="email"
            inputMode="email"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            enterKeyHint="next"
          />
          <input
            className={inputClass}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={t("auth.password")}
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            enterKeyHint={mode === "login" ? "go" : "next"}
          />

          {mode === "register" ? (
            <input
              className={inputClass}
              type="text"
              value={nativeLanguage}
              onChange={(e) => setNativeLanguage(e.target.value)}
              placeholder={t("auth.nativeLanguagePlaceholder")}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              enterKeyHint="go"
            />
          ) : null}

          {error ? (
            <p className="break-words text-sm text-red-500" role="alert">
              {error}
            </p>
          ) : null}

          <button
            type="submit"
            disabled={submitting}
            className="mt-1 h-12 rounded-full bg-primary text-base font-medium sm:h-11 sm:text-sm text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
          >
            {submitting
              ? t("auth.pleaseWait")
              : mode === "login"
                ? t("auth.login")
                : t("auth.createAccount")}
          </button>
        </form>
      </div>
    </div>
  );
}
