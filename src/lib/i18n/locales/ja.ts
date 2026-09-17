import type { TranslationKey } from "./en";

const ja = {
  "navigation.home": "ホーム",
  "navigation.discover": "見つける",
  "navigation.messages": "メッセージ",
  "navigation.calls": "通話",
  "navigation.groups": "グループ",
  "navigation.explore": "探索",
  "navigation.events": "イベント",
  "navigation.bookmarks": "ブックマーク",
  "navigation.profile": "プロフィール",
  "navigation.settings": "設定",
  "navigation.primary": "メインナビゲーション",
  "header.tagline": "いろいろな人。ひとつの世界。",
  "search.ariaLabel": "人を検索",
  "search.placeholder": "人を検索…",
  "search.shortcut": "Ctrl K",
  "search.searching": "検索中…",
  "search.error": "検索を実行できませんでした。",
  "search.tryAgain": "再試行",
  "search.emptyPrefix": "「",
  "search.emptySuffix": "」に一致する人は見つかりませんでした。",
  "user.guest": "ゲスト",
  "user.logout": "ログアウト",
  "user.loggingOut": "ログアウト中…",
  "user.logoutError": "ログアウトできませんでした。もう一度お試しください。",
} satisfies Record<TranslationKey, string>;

export default ja;
