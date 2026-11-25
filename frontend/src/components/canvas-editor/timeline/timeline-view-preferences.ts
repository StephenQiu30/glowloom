// Optional display preference only; never store jobs, media, drafts or identity here.
const JIANYING_REMINDER_KEY = "glowloom:timeline:jianying:skip-reminder";

export function skipJianyingReminder(): boolean {
  try {
    return window.localStorage.getItem(JIANYING_REMINDER_KEY) === "true";
  } catch {
    return false;
  }
}

export function setSkipJianyingReminder(skip: boolean): void {
  try {
    if (skip === true)
      window.localStorage.setItem(JIANYING_REMINDER_KEY, "true");
    else window.localStorage.removeItem(JIANYING_REMINDER_KEY);
  } catch {
    // A blocked preference store must not prevent a native export.
  }
}
