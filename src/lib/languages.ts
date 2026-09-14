/** A translation language option. `code` matches backend language codes. */
export interface Language {
  code: string;
  label: string;
}

/**
 * A small, practical starter set of languages for translation preferences.
 * Reusable by later translation UI. Not an exhaustive catalog.
 */
export const LANGUAGES: Language[] = [
  { code: "en", label: "English" },
  { code: "ru", label: "Russian" },
  { code: "uz", label: "Uzbek" },
  { code: "es", label: "Spanish" },
  { code: "fr", label: "French" },
  { code: "de", label: "German" },
  { code: "tr", label: "Turkish" },
  { code: "ar", label: "Arabic" },
  { code: "zh", label: "Chinese" },
  { code: "ja", label: "Japanese" },
  { code: "ko", label: "Korean" },
  { code: "pt", label: "Portuguese" },
  { code: "it", label: "Italian" },
];
