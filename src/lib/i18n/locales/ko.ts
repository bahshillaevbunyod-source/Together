import type { TranslationKey } from "./en";

const ko = {
  "navigation.home": "홈",
  "navigation.discover": "발견",
  "navigation.messages": "메시지",
  "navigation.calls": "통화",
  "navigation.groups": "그룹",
  "navigation.explore": "탐색",
  "navigation.events": "이벤트",
  "navigation.bookmarks": "북마크",
  "navigation.profile": "프로필",
  "navigation.settings": "설정",
  "navigation.primary": "기본 탐색",
  "header.tagline": "서로 다른 사람들. 하나의 세상.",
  "search.ariaLabel": "사람 검색",
  "search.placeholder": "사람 검색…",
  "search.shortcut": "Ctrl K",
  "search.searching": "검색 중…",
  "search.error": "검색을 실행할 수 없습니다.",
  "search.tryAgain": "다시 시도",
  "search.emptyPrefix": "‘",
  "search.emptySuffix": "’에 해당하는 사람을 찾을 수 없습니다.",
  "user.guest": "게스트",
  "user.logout": "로그아웃",
  "user.loggingOut": "로그아웃 중…",
  "user.logoutError": "로그아웃할 수 없습니다. 다시 시도해 주세요.",
} satisfies Record<TranslationKey, string>;

export default ko;
