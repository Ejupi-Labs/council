# Council design identity

This is the design reference for the Council rebuild. The user requires the Ejupi Labs visual style and icons, with all Council interface text in English. The verified facts below identify existing assets and their authority; the proposed journeys describe how Council can apply them. This document does not copy artwork or website implementation into Council.

## Verified sources

The company canonical record owns exact shared palette and wording. The brand repository owns approved identity artwork. The public icon repository owns the generic interface icon family. The website is an implemented product reference, rather than a requirement to reproduce its desktop metaphor.

| Repository | Reviewed commit | Relevant exact paths |
| --- | --- | --- |
| [Ejupi-Labs/ejupilabs-company](https://github.com/Ejupi-Labs/ejupilabs-company/tree/14d8c63c942041287049598eb844daa11a8c7e57) | `14d8c63c942041287049598eb844daa11a8c7e57` | `docs/vibe/15-canon-and-evolution.md`, `05-visual-language.md`, `06-voice-and-language.md`, `08-experience-and-interaction.md` |
| [Ejupi-Labs/ejupilabs-brand](https://github.com/Ejupi-Labs/ejupilabs-brand/tree/0a523bf4d5902f476e5e8081f87fb53f7fe38c00) | `0a523bf4d5902f476e5e8081f87fb53f7fe38c00` | `assets/brand/brand-tokens.json`, `assets/brand/presets/`, `assets/brand/fonts/SOURCE.txt`, `assets/brand/fonts/OFL.txt` |
| [Ejupi-Labs/ejupilabs-icons](https://github.com/Ejupi-Labs/ejupilabs-icons/tree/c37702fc04da7a114e5eea4416d621bd7723c74d) | `c37702fc04da7a114e5eea4416d621bd7723c74d` | `README.md`, `LICENSE`, `manifest.json`, `mono/`, `mono-dark/`, `duotone/`, `sprite-mono.svg`, `sprite-mono-dark.svg`, `sprite-duotone.svg` |
| [Ejupi-Labs/ejupilabs-website](https://github.com/Ejupi-Labs/ejupilabs-website/tree/5bf6d585b38208c66dd4383bda971d7bf81f7791) | `5bf6d585b38208c66dd4383bda971d7bf81f7791` | `docs/website/03-visual-system.md`, `04-components-and-app-screens.md`, `06-accessibility-and-responsive-behaviour.md`, `src/styles/global.css`, `src/styles/startup.css` |

These four sibling checkouts are available locally under `C:/Users/ejupi/Documents/Projects/`. The company, brand and website repositories are private; the generic icon repository is public. The selected company documents, website references, brand tokens and logo were clean during discovery. A locally modified brand README was corroborated against the connected repository rather than treated as the sole authority.

The company corpus is version 1.8. The supplied brand token file separately records schema 3, asset version `2026-10-24.5`, brand pack `2.3.0`, and an older corpus snapshot `1.5`. That snapshot does not replace the current company corpus. The local token file's Git blob `8524191624c13bc9cba5bf5e80f456ac29c64982` matches the connected repository.

## Verified identity and palette

The accessible company name is **Ejupi Labs**. Its wordmark is lowercase **ejupi labs**, with a visible space and custom vector lettering. It is not a name typed in Manrope. The symbol has four planes; preserve the approved geometry, proportions and placement. The invitation/signature is separate fixed vector artwork, not an application heading font.

The [canonical record](https://github.com/Ejupi-Labs/ejupilabs-company/blob/14d8c63c942041287049598eb844daa11a8c7e57/docs/vibe/15-canon-and-evolution.md) and [brand tokens](https://github.com/Ejupi-Labs/ejupilabs-brand/blob/0a523bf4d5902f476e5e8081f87fb53f7fe38c00/assets/brand/brand-tokens.json) agree on these exact values:

| Token | Value |
| --- | --- |
| Night | `#17132B` |
| Room | `#25213B` |
| Chalk | `#F5F2FF` |
| Pear | `#DFFF5E` |
| Lilac | `#B8A5FF` |
| Rose | `#F5B1CF` |
| Cobalt | `#6176FF` |
| Muted | `#BCB4D1` |
| Error | `#FF8797` |

Approved appearances are `dark-colour` (Lilac/Pear on Night), `dark-mono` (Chalk on Night) and `light-mono` (Night on Chalk). The colour wordmark uses Lilac for `ejupi` and Pear for `labs`.

## Recommended existing artwork

Use the approved **dark-colour** company logo as the initial Council attribution reference. It is already publicly used: on 5 November 2026, [the official website logo](https://www.ejupilabs.com/media/brand/dark-colour/logo-horizontal.svg) returned HTTP 200 with `image/svg+xml`, 5,624 bytes, and SHA-256 `d697a045d059366aaeafe7c2a9a8f5f4a589c0c698e5e7c32757d5b1c8e81c7a`. This exactly matches `ejupilabs-brand/assets/brand/presets/dark-colour/logos/logo-horizontal.svg`. The website source also uses appearance-matched horizontal logos and symbols. This verifies the asset's public use, not a rendered review of the whole live website.

| Use | Source path in `ejupilabs-brand` | SHA-256 where checked |
| --- | --- | --- |
| Company attribution | `assets/brand/presets/dark-colour/logos/logo-horizontal.svg` | `d697a045d059366aaeafe7c2a9a8f5f4a589c0c698e5e7c32757d5b1c8e81c7a` |
| Compact company mark | `assets/brand/presets/dark-colour/logos/symbol.svg` | `87bb971fa3b8e5f484d149b93b60fff068c6bdd161635845fe22f9ac740f7bf9` |
| Light appearance attribution | `assets/brand/presets/light-mono/logos/logo-horizontal.svg` | `4a26dacdf641c25eb3e1d664617594769b2bdb807fc410b3a5185b814de8f8fd` |
| Appearance-matched web marks | `assets/brand/presets/{appearance}/web/favicon.svg`, `app-icon.svg`, `maskable-icon.svg` | Available; select and pin exact files during implementation. |

The prepared symbol is suitable for compact chrome. Token starting minimums are 240 CSS px for the horizontal logo, 180 for stacked, 160 for wordmark and 24 for symbol; preserve margins and check actual legibility. Do not squeeze a full logo into a small control.

No approved Council-specific application identity was found in the reviewed inventories. Existing company attribution is available without inventing one. A later deliberate Council app mark can be a separate design decision rather than borrowing another product's app tile.

## Verified icons, typography and geometry

Use the public Ejupi Labs generic icon family, release **1.3.1**: 72 icons in eight families, with 216 standalone SVGs and three sprites. The supplied geometry uses a `0 0 48 48` viewBox, transparent backgrounds, filled shapes and consistent optical margins. `mono` uses `currentColor`; `mono-dark` uses Chalk; `duotone` uses Lilac/Pear. Useful existing names include `users`, `plug`, `network`, `pipeline`, `clock`, `check`, `warning`, `file-text`, `terminal`, `edit`, `plus`, `settings` and `shield`.

Use uniform scaling and the supplied margins. Inline SVG or a sprite can inherit `currentColor`; a plain SVG `<img>` does not inherit the surrounding document's text colour. Give meaningful controls accessible names and hide decorative glyphs from assistive reading. The reviewed icon board confirms the supplied filled family; unrelated thin icons or emoji would change the visual language.

Recommend the actual website's system reading stack for Council: `-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif`. It avoids an unnecessary font download and follows the implemented product reference. Brand guidance reserves **Manrope 500** for supporting captions, labels and copy, while product reading typography is a separate choice. The outlined wordmark does not require loading a font. If Manrope is selected later, the original variable file and its source/license are available at `assets/brand/fonts/`.

The website uses a default **12 px** surface radius and documented spacing steps of **4, 8, 12, 16, 20, 24, 32 and 48 px**. Its extra UI neutrals are product tokens, rather than new canonical brand colours. Its Mac-like menu bar, dock, windows, games and entrance animation are website-specific treatments; Council can express the same identity with a direct work interface.

Company interaction guidance favours a clear route to useful work, visible important choices, understandable feedback, control/stop/undo and optional deeper exploration. Voice is warm, direct and concrete. Application labels should name real actions and state instead of repeating brand slogans.

## Licensing and provenance during implementation

The user's request already authorizes using Ejupi Labs style and icons for Council. No additional generic permission gate is introduced by this reference.

The [generic icon package is MIT](https://github.com/Ejupi-Labs/ejupilabs-icons/blob/c37702fc04da7a114e5eea4416d621bd7723c74d/LICENSE), copyright 2026 Ejupi Labs. Preserve that copyright and permission notice with redistributed icons. Company logos, trademarks, private app identities, brand documents and website source do **not** acquire MIT licensing from the icon package. Record company artwork as Ejupi Labs identity assets with its actual provenance; do not relabel it as an open icon asset.

The supplied Manrope font has **SIL Open Font License 1.1**, with its original source recorded as Google Fonts. Retain `OFL.txt` if the font is vendored. Using outlined logo artwork does not require shipping the font.

When vendoring selected assets, record repository, commit, original path, file hash, appearance and retained notice. Use the smallest chosen set. The website's historical `runtime-assets.lock.json` pins its own `vendor/runtime/1.0.0` bundle; it is not Council's lock and does not justify copying the website source or all private app artwork. No source or asset has been copied by this documentation work.

## Proposed Council journeys

These are **Council product suggestions**, separate from verified brand facts. Feature specifications should turn the selected journeys into acceptance criteria before implementation. They are not claims that any rebuilt UI or provider capability already exists.

1. **Compose a task.** Start with one clear task field, workspace selection and context attachment. Make Auto the primary path, using the saved task policy. Keep the effective mode and permissions visible, with an optional task override instead of a mandatory read/write choice on every task. Reveal per-agent model, effort and other tuning when relevant rather than presenting a wall of settings.
2. **See available agents.** Name configured agents and their observed readiness, with a concise reason and a setup action where needed. Finding an executable does not prove authentication, model access or quota. Keep unavailable and unknown states truthful; use Preferences for recurring setup.
3. **Start and inspect the plan.** Start under the saved task policy without requiring plan confirmation on every task. Make a concise plan with participants, dependent work, effective permissions and the explicit call bound available to inspect as work proceeds. Keep task overrides accessible. Describe provider-specific permission behaviour accurately; do not imply a universal OS sandbox.
4. **Follow a task.** Use a readable timeline with participant ownership and distinct completed, failed and cancelled states. A preflight failure with zero agent calls must look different from a failed agent step. Keep Stop visible while active. Show resume only for a supported checkpoint, with its actual scope.
5. **Inspect result, evidence and usage.** Make the answer, retained steps/artifacts/checks and reported usage easy to find for that task. Display unknown tokens, cost or quota as “Not reported”; separate estimates and configured information from measured usage or actual receipts. The saved task policy may authorize verified automatic local integration; review-only or manual acceptance retains the patch/worktree for review. Always report the actual integration status. External publication occurs only when explicitly included in the task scope.
6. **Return to work.** Keep primary navigation to Tasks, Agents and Preferences, with recent useful work easy to reopen. The first screen should help start or continue work without an introduction or desktop simulation.

Suggested visual application is Night/Room surfaces, Chalk reading text, restrained Lilac details and a Pear primary action with Night text. Suggested icon mappings are `edit`/`plus` for composing, `users`/`plug` for agents, `pipeline` for steps, `clock` for active/waiting, `check`/`warning` for outcomes, `file-text` for evidence and `settings` for preferences. These mappings and semantic state colours are product choices, not brand canon.

The UI feature should verify keyboard navigation, visible focus, labelled controls, non-colour state cues, reduced motion, readable contrast and suitable coarse-pointer targets (the website reference uses at least 44 px). Brand reuse is not an accessibility certification. Keep long results and evidence reading calmer than the surrounding controls, and test the actual task flows at small widths.
