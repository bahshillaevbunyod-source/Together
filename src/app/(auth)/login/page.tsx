"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";

import { ApiError, login, register } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

type Mode = "login" | "register";

export default function LoginPage() {
  const router = useRouter();
  const { refresh } = useAuth();

  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [nativeLanguage, setNativeLanguage] = useState("en");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const inputClass =
    "h-11 w-full rounded-lg border border-border bg-background px-4 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

  const switchMode = (next: Mode) => {
    setMode(next);
    setError(null);
  };

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submitting) return;

    // Minimal client-side validation; the backend is the source of truth.
    if (!email.trim() || !password) {
      setError("Email and password are required.");
      return;
    }
    if (mode === "register" && (!username.trim() || !displayName.trim())) {
      setError("Username and display name are required.");
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
      router.push("/");
    } catch (err) {
      // Show the backend's safe message, or a generic fallback.
      setError(err instanceof ApiError ? err.message : "Something went wrong.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-[calc(100vh-9rem)] items-center justify-center">
      <div className="w-full max-w-md rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <div className="mb-6 flex flex-col items-center gap-2 text-center">
          <Image
            src="/images/together-logo.png"
            alt="Together"
            width={48}
            height={48}
            className="h-12 w-12 object-contain"
          />
          <h1 className="text-lg font-bold tracking-tight text-foreground">
            {mode === "login" ? "Welcome back" : "Create your account"}
          </h1>
          <p className="text-sm text-muted">Different people. One world.</p>
        </div>

        {/* Mode switch */}
        <div className="mb-5 flex rounded-full border border-border bg-background p-1 text-sm">
          <button
            type="button"
            onClick={() => switchMode("login")}
            className={`flex-1 rounded-full py-1.5 font-medium transition-colors ${
              mode === "login"
                ? "bg-primary text-white"
                : "text-muted hover:text-foreground"
            }`}
          >
            Log in
          </button>
          <button
            type="button"
            onClick={() => switchMode("register")}
            className={`flex-1 rounded-full py-1.5 font-medium transition-colors ${
              mode === "register"
                ? "bg-primary text-white"
                : "text-muted hover:text-foreground"
            }`}
          >
            Sign up
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
                placeholder="Display name"
                autoComplete="name"
              />
              <input
                className={inputClass}
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="Username"
                autoComplete="username"
              />
            </>
          ) : null}

          <input
            className={inputClass}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Email"
            autoComplete="email"
          />
          <input
            className={inputClass}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Password"
            autoComplete={mode === "login" ? "current-password" : "new-password"}
          />

          {mode === "register" ? (
            <input
              className={inputClass}
              type="text"
              value={nativeLanguage}
              onChange={(e) => setNativeLanguage(e.target.value)}
              placeholder="Native language (e.g. en)"
            />
          ) : null}

          {error ? (
            <p className="text-sm text-red-500" role="alert">
              {error}
            </p>
          ) : null}

          <button
            type="submit"
            disabled={submitting}
            className="mt-1 h-11 rounded-full bg-primary text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
          >
            {submitting
              ? "Please wait…"
              : mode === "login"
                ? "Log in"
                : "Create account"}
          </button>
        </form>
      </div>
    </div>
  );
}
