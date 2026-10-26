import { expect, test } from "@playwright/test";

test("comparison URL survives reload and browser navigation", async ({ page }) => {
  const baseline = "a".repeat(32);
  const followup = "b".repeat(32);
  await page.goto(`/?view=compare&left=${baseline}&right=${followup}`);
  await page.reload();
  await expect(page).toHaveURL(`/?view=compare&left=${baseline}&right=${followup}`);
});
