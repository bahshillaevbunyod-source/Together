import type { TranslationKey } from "./en";

const zh = {
  "navigation.home": "首页",
  "navigation.discover": "发现",
  "navigation.messages": "消息",
  "navigation.calls": "通话",
  "navigation.groups": "群组",
  "navigation.explore": "探索",
  "navigation.events": "活动",
  "navigation.bookmarks": "书签",
  "navigation.profile": "个人资料",
  "navigation.settings": "设置",
  "navigation.primary": "主导航",
  "header.tagline": "不同的人，同一个世界。",
  "search.ariaLabel": "搜索用户",
  "search.placeholder": "搜索用户…",
  "search.shortcut": "Ctrl K",
  "search.searching": "搜索中…",
  "search.error": "无法完成搜索。",
  "search.tryAgain": "重试",
  "search.emptyPrefix": "未找到与“",
  "search.emptySuffix": "”相关的用户。",
  "user.guest": "访客",
  "user.logout": "退出登录",
  "user.loggingOut": "正在退出…",
  "user.logoutError": "退出登录失败。请重试。",
} satisfies Record<TranslationKey, string>;

export default zh;
