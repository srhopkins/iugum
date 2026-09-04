/**
 * Playwright config for the Atomdown front-end suite.
 *
 * Separate from `silverbullet/playwright.config.ts` on purpose: that file lives
 * in the vendored subtree, which CONTRIBUTING.md decision 6 keeps in upstream
 * form. This config reuses that tree's installed `@playwright/test` and its
 * browser download through the `node_modules` symlink beside this file, so
 * there is one Playwright and one Chromium on the machine, not two.
 *
 * Settings that differ from upstream's, and why:
 *
 * - `workers: 1`. Every test boots its own server on its own free port against
 *   its own temp space, so they are isolated enough to parallelise. They are
 *   not parallelised because these are *geometry* tests: four Chromium
 *   instances competing for one machine's CPU make layout settle late, and a
 *   late layout is a wrong number rather than a slow one.
 * - A fixed 1440x900 viewport. `full` width is `min(1600px, 96%)`, so the
 *   viewport is part of the measurement. Letting it vary would make the same
 *   assertion mean different things on two machines.
 * - `deviceScaleFactor: 1`. Rect arithmetic on a 2x display rounds
 *   differently, and this suite compares pixels.
 */

import { chromium, defineConfig, devices } from "@playwright/test";
import { existsSync } from "node:fs";

/**
 * Which browser to drive.
 *
 * Prefer Playwright's own pinned Chromium: it is a fixed build, so a pixel
 * measured here means the same thing on the next machine. If it has not been
 * downloaded (`npx playwright install` in `silverbullet/`, or `make setup`),
 * fall back to the Google Chrome already installed on the box rather than
 * failing the gate over a missing 150MB download. The fallback is the less
 * hermetic of the two — Chrome auto-updates — so the runner prints which one
 * it used, and `ATOMDOWN_FE_CHANNEL` forces either.
 */
function browserChannel(): string | undefined {
  if (process.env.ATOMDOWN_FE_CHANNEL) {
    return process.env.ATOMDOWN_FE_CHANNEL === "chromium"
      ? undefined
      : process.env.ATOMDOWN_FE_CHANNEL;
  }
  try {
    if (existsSync(chromium.executablePath())) return undefined;
  } catch {
    // No pinned build registered at all.
  }
  return "chrome";
}

export const CHANNEL = browserChannel();

export default defineConfig({
  testDir: ".",
  // Booting a server, loading 82 atoms and walking a 16-cell matrix is not
  // fast. The suite is a pre-push gate, not a watch-mode test.
  timeout: 180_000,
  expect: { timeout: 20_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: process.env.CI ? [["list"], ["github"]] : [["list"]],
  outputDir: "../../scratchpad/atomdown-fe-out/playwright",
  use: {
    ...devices["Desktop Chrome"],
    channel: CHANNEL,
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "off",
    launchOptions: {
      // A small /dev/shm crashes the chromium renderer under CI.
      args: ["--disable-dev-shm-usage"],
    },
  },
  projects: [
    {
      name: "atomdown",
      testMatch: /\d-.*\.test\.ts$/,
    },
    {
      // The negative control: these tests reintroduce real defects and assert
      // that the rules REPORT them. Its own project because it is supposed to
      // see violations, and mixing it into the gate would read as failures.
      name: "defects",
      testMatch: /defects\.test\.ts$/,
    },
    {
      // A scaffolding check, not a rule: it prints what the two views actually
      // render so the six rules can be written against reality. Run it with
      // `--project=probe`; the gate never does.
      name: "probe",
      testMatch: /probe\.test\.ts$/,
    },
  ],
});
