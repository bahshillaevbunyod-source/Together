import type { TranslationKey } from "./en";

const es = {
  "navigation.home": "Inicio",
  "navigation.discover": "Descubrir",
  "navigation.messages": "Mensajes",
  "navigation.calls": "Llamadas",
  "navigation.groups": "Grupos",
  "navigation.explore": "Explorar",
  "navigation.events": "Eventos",
  "navigation.bookmarks": "Marcadores",
  "navigation.profile": "Perfil",
  "navigation.settings": "Ajustes",
  "navigation.primary": "Navegación principal",
  "header.tagline": "Personas diferentes. Un solo mundo.",
  "search.ariaLabel": "Buscar personas",
  "search.placeholder": "Buscar personas…",
  "search.shortcut": "Ctrl K",
  "search.searching": "Buscando…",
  "search.error": "No se pudo realizar la búsqueda.",
  "search.tryAgain": "Intentar de nuevo",
  "search.emptyPrefix": "No se encontraron personas para «",
  "search.emptySuffix": "».",
  "user.guest": "Invitado",
  "user.logout": "Cerrar sesión",
  "user.loggingOut": "Cerrando sesión…",
  "user.logoutError": "No se pudo cerrar sesión. Inténtalo de nuevo.",
} satisfies Record<TranslationKey, string>;

export default es;
