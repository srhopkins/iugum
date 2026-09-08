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
  /**
   * Where a visual baseline lives. Its own directory beside the suite, not
   * inside Playwright's default `__screenshots__`, so the recorded browser
   * version (`visual-baselines/browser.txt`) sits next to the images it
   * belongs to. Playwright still appends the platform, so a Linux run writes
   * its own set instead of failing against a Mac's.
   */
  snapshotPathTemplate:
    "{testDir}/visual-baselines/{arg}-{platform}{ext}",
  projects: [
    {
      /**
       * The behavioural rules. Every numbered rule EXCEPT 9, which needs a
       * different browser policy - see the `visual` project below.
       *
       * An explicit list of numbers rather than a character class. `[1-8]-`
       * was correct while 8 was the highest, and it silently stopped matching
       * when rule 10 arrived: `10-` has no digit-then-dash in it, so the file
       * was collected by no project at all and the rule ran nowhere. A regex
       * whose failure mode is "the test does not run" has to name its files.
       */
      name: "atomdown",
      testMatch: /\/(1|2|3|4|5|6|7|8|10|11|12)-.*\.test\.ts$/,
    },
    {
      /**
       * RULE 9 — VISUAL REGRESSION, in the one pinned environment.
       *
       * Steve's hard requirement (see `9-visual.test.ts` for the quote and the
       * whole list). What is pinned HERE rather than in the test:
       *
       *  - `channel: undefined`. Playwright's own bundled Chromium, at the
       *    version in `silverbullet/package-lock.json`. The `browserChannel()`
       *    fallback above deliberately does not apply: an installed Chrome
       *    auto-updates, and that is how two machines silently disagree about
       *    a pixel. `guardEnvironment` in the test refuses the run if the
       *    bundled build is missing rather than quietly using another one.
       *  - `headless: true`, stated rather than left to the default, so the
       *    guard has something to read. A headed run cannot produce or update
       *    a baseline.
       *  - A fixed viewport and `deviceScaleFactor: 1`, as the other project
       *    has, because these are pixel comparisons.
       *  - `reducedMotion` and a fixed timezone and locale, so a transition
       *    mid-capture or a date rendered in another region cannot move a
       *    pixel. `colorScheme` is forced per theme case inside the test.
       *  - `maxDiffPixels: 0`. Zero tolerance, on purpose: the escape from a
       *    failing baseline is to look at the diff and re-take it in one
       *    command, never to raise a threshold until the rule sees nothing.
       */
      name: "visual",
      testMatch: /9-visual\.test\.ts$/,
      use: {
        ...devices["Desktop Chrome"],
        channel: undefined,
        headless: true,
        viewport: { width: 1440, height: 900 },
        deviceScaleFactor: 1,
        reducedMotion: "reduce",
        timezoneId: "UTC",
        locale: "en-US",
        screenshot: "only-on-failure",
        trace: "retain-on-failure",
        video: "off",
        launchOptions: {
          args: ["--disable-dev-shm-usage", "--force-color-profile=srgb"],
        },
      },
      expect: {
        timeout: 20_000,
        toHaveScreenshot: {
          maxDiffPixels: 0,
          animations: "disabled",
          caret: "hide",
        },
      },
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
