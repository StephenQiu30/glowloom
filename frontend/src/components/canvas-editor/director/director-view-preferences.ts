// A display-only onboarding preference. No scene, asset, identity, draft or save state is stored here.
const HELP_KEY = "glowloom.director.help.v1";
export function directorHelpSeen() {
  try {
    return window.localStorage.getItem(HELP_KEY) === "seen";
  } catch {
    return false;
  }
}
export function markDirectorHelpSeen() {
  try {
    window.localStorage.setItem(HELP_KEY, "seen");
  } catch {
    /* Optional view preference. */
  }
}
