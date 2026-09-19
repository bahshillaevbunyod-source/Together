import { LanguageProvider } from "@/lib/language-context";

// Auth routes (e.g. /login) render before a user — and therefore before a
// platformLanguage — exists, so they use the English-fallback locale
// (platformLanguage=null, LTR), mirroring the language onboarding screen. The
// authenticated app subtree has its own LanguageProvider in (app)/layout with
// the user's real platformLanguage; the two never overlap, so there is no
// competing language state.
export default function AuthLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <LanguageProvider platformLanguage={null}>{children}</LanguageProvider>;
}
