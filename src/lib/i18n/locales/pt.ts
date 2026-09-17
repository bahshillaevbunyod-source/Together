import type { TranslationKey } from "./en";

const pt = {
  "navigation.home": "Início",
  "navigation.discover": "Descobrir",
  "navigation.messages": "Mensagens",
  "navigation.calls": "Chamadas",
  "navigation.groups": "Grupos",
  "navigation.explore": "Explorar",
  "navigation.events": "Eventos",
  "navigation.bookmarks": "Favoritos",
  "navigation.profile": "Perfil",
  "navigation.settings": "Configurações",
  "navigation.primary": "Navegação principal",
  "header.tagline": "Pessoas diferentes. Um só mundo.",
  "search.ariaLabel": "Pesquisar pessoas",
  "search.placeholder": "Pesquisar pessoas…",
  "search.shortcut": "Ctrl K",
  "search.searching": "Pesquisando…",
  "search.error": "Não foi possível realizar a pesquisa.",
  "search.tryAgain": "Tentar novamente",
  "search.emptyPrefix": "Nenhuma pessoa encontrada para “",
  "search.emptySuffix": "”.",
  "user.guest": "Visitante",
  "user.logout": "Sair",
  "user.loggingOut": "Saindo…",
  "user.logoutError": "Não foi possível sair. Tente novamente.",
} satisfies Record<TranslationKey, string>;

export default pt;
