import type { Locator } from "@playwright/test";

/** Select an item from the Radix Select used by the application. */
export async function chooseOption(
  trigger: Locator,
  optionName: string | RegExp,
) {
  await trigger.click();
  await trigger.page().getByRole("option", { name: optionName }).click();
}
