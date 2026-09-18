#!/usr/bin/env node
/**
 * i18n coverage validator for Together's global platform-language dictionaries.
 *
 * Static, dependency-free check that complements `tsc --noEmit` (which enforces
 * exact key typing via `satisfies Record<TranslationKey, string>`). It parses
 * the source files textually and proves:
 *   1. the full Google canonical target set is accounted for
 *   2. every canonical platform language has exactly one dictionary
 *   3. every dictionary belongs to a supported platform language
 *   4. exact key parity with English
 *   5. no empty strings (and no corruption / brand leakage)
 *   6. no supported platform language resolves to the English fallback
 *   7. aliases / variants resolve correctly
 *   8. RTL metadata is correct
 *   9. an unknown / invalid locale safely falls back to English
 *  10. no target code is unaccounted for
 *
 * Exits non-zero on any failure.
 */
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const localesDir = join(root, "src/lib/i18n/locales");
const languagesFile = join(root, "src/lib/languages.ts");
const indexFile = join(root, "src/lib/i18n/index.ts");

const failures = [];
const fail = (m) => failures.push(m);

// The verified full Google Cloud Translation v2/NMT target set (198 codes).
const GOOGLE_TARGETS = "ab ace ach af sq alz am ar hy as awa ay az ban bm ba eu btx bts bbc be bem bn bew bho bik bs br bg bua yue ca ceb ny zh-CN zh zh-TW cv co crh hr cs da din dv doi dov nl dz en eo et ee fj fil tl fi fr fr-FR fr-CA fy ff gaa gl lg ka de el gn gu ht cnh ha haw iw he hil hi hmn hu hrx is ig ilo id ga it ja jw jv kn pam kk km cgg rw ktu gom ko kri ku ckb ky lo ltg la lv lij li ln lt lmo luo lb mk mai mak mg ms ms-Arab ml mt mi mr chm mni-Mtei min lus mn my nr new ne nso no nus oc or om pag pap ps fa pl pt pt-PT pt-BR pa pa-Arab qu rom ro rn ru sm sg sa gd sr st crs shn sn scn szl sd si sk sl so es su sw ss sv tg ta tt te tet th ti ts tn tr tk ak uk ur ug uz vi cy xh yi yo yua zu".split(" ");
// True aliases (same target under an alternate code) that must NOT have their own dictionary.
const TRUE_ALIASES = { iw: "he", jw: "jv", tl: "fil", "zh-CN": "zh", "fr-FR": "fr", "pt-PT": "pt" };
const EXPECTED_RTL = ["ar","he","fa","ur","ckb","ps","sd","ug","yi","dv","ms-Arab","pa-Arab"];

// --- catalog ---
const languagesSrc = readFileSync(languagesFile, "utf8");
const catalogCodes = [...languagesSrc.matchAll(/code:\s*"([^"]+)"/g)].map((m) => m[1]);
const catalogSet = new Set(catalogCodes);
if (catalogCodes.length !== catalogSet.size) fail("languages.ts contains duplicate codes");

// --- dictionaries ---
const localeFiles = readdirSync(localesDir).filter((f) => f.endsWith(".ts"));
const dicts = new Map();
function parseDict(code, src) {
  const keys = new Set();
  const entries = [];
  for (const line of src.split("\n")) {
    const m = line.match(/^\s*"([^"]+)":\s*("(?:[^"\\]|\\.)*"),?\s*$/);
    if (!m) continue;
    let value;
    try { value = JSON.parse(m[2]); } catch { fail(`${code}: bad value for "${m[1]}"`); continue; }
    keys.add(m[1]);
    entries.push([m[1], value]);
  }
  return { keys, entries };
}
for (const file of localeFiles) {
  const code = file.replace(/\.ts$/, "");
  dicts.set(code, parseDict(code, readFileSync(join(localesDir, file), "utf8")));
}
const enDict = dicts.get("en");
if (!enDict) { fail("missing en.ts (source of truth)"); report(); }
const enKeys = enDict.keys;

// --- 2. every catalog language has a dictionary; 3. every dictionary is in the catalog ---
for (const code of catalogCodes) if (!dicts.has(code)) fail(`[2] catalog "${code}" has no dictionary`);
for (const code of dicts.keys()) if (!catalogSet.has(code)) fail(`[3] dictionary "${code}" not in languages.ts`);

// --- 4. exact English key parity; no missing/extra ---
for (const [code, dict] of dicts) {
  if (code === "en") continue;
  for (const k of enKeys) if (!dict.keys.has(k)) fail(`[4] ${code}: missing key "${k}"`);
  for (const k of dict.keys) if (!enKeys.has(k)) fail(`[4] ${code}: extra key "${k}"`);
}

// --- 5. no empty / corrupt / brand-leaking strings ---
for (const [code, dict] of dicts) {
  for (const [k, v] of dict.entries) {
    if (typeof v !== "string" || v.trim() === "") fail(`[5] ${code}: empty "${k}"`);
    else if (v !== v.trim()) fail(`[5] ${code}: whitespace-padded "${k}"`);
    else if (/�/.test(v)) fail(`[5] ${code}: corruption (U+FFFD) in "${k}"`);
    // The product brand is the literal token "Together" and must stay identical
    // in every locale. Allow only the exact standalone brand word; any other
    // "Together" occurrence (glued/partial — i.e. leakage or corruption) still fails.
    else if (/Together/.test(v.replace(/\bTogether\b/g, ""))) fail(`[5] ${code}: brand "Together" present in "${k}"`);
  }
}

// --- resolver replica (kept in lockstep with src/lib/i18n/index.ts) ---
const localeCodes = new Set(dicts.keys());
const lowerToLocale = new Map([...localeCodes].map((c) => [c.toLowerCase(), c]));
const ALIASES = {
  iw: "he", jw: "jv", tl: "fil", in: "id", nb: "no", nn: "no",
  "zh-cn": "zh", "zh-hans": "zh", "zh-sg": "zh",
  "zh-tw": "zh-TW", "zh-hk": "zh-TW", "zh-hant": "zh-TW",
  "fr-fr": "fr", "pt-pt": "pt", "pt-br": "pt-BR",
};
function resolveLocale(language) {
  const n = language?.trim().toLowerCase();
  if (!n) return "en";
  if (lowerToLocale.has(n)) return lowerToLocale.get(n);
  if (ALIASES[n]) return ALIASES[n];
  const base = n.split(/[-_]/)[0];
  if (base && base !== n) {
    if (ALIASES[base]) return ALIASES[base];
    if (lowerToLocale.has(base)) return lowerToLocale.get(base);
  }
  return "en";
}
const indexSrc = readFileSync(indexFile, "utf8");
for (const a of Object.keys(ALIASES)) {
  const re = new RegExp(`["']?${a.replace(/[-/\\^$*+?.()|[\]{}]/g, "\\$&")}["']?\\s*:`);
  if (!re.test(indexSrc)) fail(`[7] alias "${a}" in validator not present in index.ts (drift)`);
}

// --- 1 & 10. full Google target set accounted for; nothing unaccounted; no true-alias dict ---
for (const t of GOOGLE_TARGETS) {
  const r = resolveLocale(t);
  const isEnglish = t.toLowerCase() === "en";
  if (isEnglish) {
    if (r !== "en") fail(`[1] target "en" resolved to "${r}"`);
  } else if (r === "en") {
    fail(`[10] target "${t}" is UNACCOUNTED (resolves to English fallback)`);
  }
  if (t in TRUE_ALIASES && dicts.has(t)) fail(`[1] true alias "${t}" must not have its own dictionary`);
}
// coverage math: canonical dicts + true aliases == full target set
const canonicalCount = GOOGLE_TARGETS.filter((t) => !(t in TRUE_ALIASES)).length;
if (canonicalCount !== dicts.size)
  fail(`[1] canonical target count ${canonicalCount} != dictionaries ${dicts.size}`);

// --- 6. no supported platform language resolves to English (except en) ---
for (const code of catalogCodes) {
  const r = resolveLocale(code);
  if (r !== code) fail(`[6] resolveLocale("${code}") = "${r}", expected "${code}"`);
  if (r === "en" && code !== "en") fail(`[6] supported language "${code}" uses English fallback`);
}

// --- 7. alias / variant resolution ---
for (const [alias, target] of Object.entries(TRUE_ALIASES)) {
  const r = resolveLocale(alias);
  if (r !== target) fail(`[7] alias "${alias}" resolved to "${r}", expected "${target}"`);
}
for (const [a, want] of [["zh-Hant","zh-TW"],["zh-HK","zh-TW"],["es-MX","es"],["en-GB","en"],["pt-BR","pt-BR"]]) {
  const r = resolveLocale(a);
  if (r !== want) fail(`[7] "${a}" resolved to "${r}", expected "${want}"`);
}

// --- 8. RTL metadata ---
const rtlMatch = indexSrc.match(/RTL_LOCALES\s*=\s*new Set<Locale>\(\[([^\]]*)\]\)/);
if (!rtlMatch) fail("[8] RTL_LOCALES not found in index.ts");
else {
  const rtl = [...rtlMatch[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]);
  for (const c of rtl) if (!localeCodes.has(c)) fail(`[8] RTL locale "${c}" has no dictionary`);
  for (const c of EXPECTED_RTL) if (!rtl.includes(c)) fail(`[8] expected RTL locale "${c}" missing`);
  for (const c of rtl) if (!EXPECTED_RTL.includes(c)) fail(`[8] unexpected RTL locale "${c}"`);
}
// QA_REVIEW_LOCALES must be a subset of shipped locales (they are still fully shipped).
const qaMatch = indexSrc.match(/QA_REVIEW_LOCALES\s*=\s*new Set<Locale>\(\[([\s\S]*?)\]\)/);
let qaCount = 0;
if (!qaMatch) fail("[8] QA_REVIEW_LOCALES not found in index.ts");
else {
  const qa = [...qaMatch[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]);
  qaCount = qa.length;
  for (const c of qa) if (!localeCodes.has(c)) fail(`[8] QA locale "${c}" has no dictionary`);
}

// --- 9. unknown / invalid -> English ---
for (const u of ["", "xx", "xx-ZZ", "klingon", "zz-Latn-ZZ", "123", null]) {
  if (resolveLocale(u) !== "en") fail(`[9] resolveLocale(${JSON.stringify(u)}) did not fall back to en`);
}

report();

function report() {
  const keyCount = enKeys ? enKeys.size : 0;
  if (failures.length === 0) {
    console.log(
      `i18n OK — ${dicts.size} dictionaries, ${keyCount} keys each, catalog ${catalogCodes.length}, ` +
      `Google targets ${GOOGLE_TARGETS.length} (canonical ${canonicalCount} + aliases ${Object.keys(TRUE_ALIASES).length}), ` +
      `RTL ${EXPECTED_RTL.length}, QA ${qaCount}. All 10 checks passed.`,
    );
    process.exit(0);
  }
  console.error(`i18n VALIDATION FAILED (${failures.length}):`);
  for (const f of failures.slice(0, 60)) console.error("  - " + f);
  if (failures.length > 60) console.error(`  … and ${failures.length - 60} more`);
  process.exit(1);
}
