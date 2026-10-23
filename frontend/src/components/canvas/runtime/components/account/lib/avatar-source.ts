const INVALID_AVATAR_CHARACTERS = /[\s\\]|\p{Cc}/u;

export function avatarSource(value?: string | null): string | undefined {
  if (!value || value.length > 1024 || INVALID_AVATAR_CHARACTERS.test(value))
    return undefined;

  if (value.startsWith("/") && !value.startsWith("//")) return value;
  if (!/^https:\/\//i.test(value)) return undefined;

  try {
    const url = new URL(value);
    return url.hostname && !url.username && !url.password ? value : undefined;
  } catch {
    return undefined;
  }
}
