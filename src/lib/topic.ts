const maxTopicSlugCharacters = 100;
const allowedTopicCharacter = /^[\p{L}\p{N}\p{M}_]$/u;

export type TopicTextSegment = {
  text: string;
  slug?: string;
};

/**
 * Mirrors the backend topic canonicalization: Unicode NFC, lowercase, and the
 * exact hashtag alphabet persisted by post creation.
 */
export function canonicalTopicSlug(raw: string): string | null {
  if (!raw) return null;

  for (const character of raw) {
    if (!allowedTopicCharacter.test(character)) return null;
  }

  const slug = raw.normalize("NFC").toLowerCase();
  if (!slug || Array.from(slug).length > maxTopicSlugCharacters) return null;
  return slug;
}

/**
 * Splits post text into ordinary text and valid hashtag segments without
 * altering the original display text. Invalid or oversized tags stay ordinary
 * text, matching the backend extractor's skip behavior.
 */
export function splitTopicText(content: string): TopicTextSegment[] {
  const characters = Array.from(content);
  const segments: TopicTextSegment[] = [];
  let textStart = 0;

  for (let index = 0; index < characters.length; index += 1) {
    if (characters[index] !== "#") continue;

    let end = index + 1;
    while (end < characters.length && allowedTopicCharacter.test(characters[end])) {
      end += 1;
    }

    const slug = canonicalTopicSlug(characters.slice(index + 1, end).join(""));
    if (!slug) continue;

    if (textStart < index) {
      segments.push({ text: characters.slice(textStart, index).join("") });
    }
    segments.push({ text: characters.slice(index, end).join(""), slug });
    textStart = end;
    index = end - 1;
  }

  if (textStart < characters.length || segments.length === 0) {
    segments.push({ text: characters.slice(textStart).join("") });
  }

  return segments;
}
