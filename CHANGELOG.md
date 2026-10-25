# Changelog

## 0.1.0 (2026-10-25)


### Features

* **adapters:** add Claude Code adapter with settings merge ([#24](https://github.com/TomaszJanusz/kitbag/issues/24)) ([8ceb270](https://github.com/TomaszJanusz/kitbag/commit/8ceb2702cf24421ba2cf520f834631fe7a2279e0))
* **adapters:** add Codex adapter ([#22](https://github.com/TomaszJanusz/kitbag/issues/22)) ([959ae99](https://github.com/TomaszJanusz/kitbag/commit/959ae992c4d15175710f278bd1120f3cd9fe2def))
* **adapters:** add Gemini CLI and OpenCode ([#34](https://github.com/TomaszJanusz/kitbag/issues/34)) ([34e3210](https://github.com/TomaszJanusz/kitbag/commit/34e32103a8698ce489b04f7729d46118f1457265))
* **adapters:** add generic targets and files ([#23](https://github.com/TomaszJanusz/kitbag/issues/23)) ([255787b](https://github.com/TomaszJanusz/kitbag/commit/255787b84b694a49906f8f118be68d994db80028))
* **adapters:** add shared instruction rendering strategies ([#20](https://github.com/TomaszJanusz/kitbag/issues/20)) ([325795d](https://github.com/TomaszJanusz/kitbag/commit/325795d308be111a5ca454097618e43c530deca4))
* **api:** add local JSON-RPC API v1 with versioned contract ([#31](https://github.com/TomaszJanusz/kitbag/issues/31)) ([56da145](https://github.com/TomaszJanusz/kitbag/commit/56da1451aa3421a8b996fd3d82066bcc994941d5))
* bootstrap KitBag repository ([#2](https://github.com/TomaszJanusz/kitbag/issues/2)) ([06e4191](https://github.com/TomaszJanusz/kitbag/commit/06e41916dd2f6ce52c08748915e4fea19c19c977))
* **cli:** add adopt, unmanage and revert ([#28](https://github.com/TomaszJanusz/kitbag/issues/28)) ([58c0b20](https://github.com/TomaszJanusz/kitbag/commit/58c0b20089522fc0451589cc0169b55225267367))
* **cli:** add doctor command ([#26](https://github.com/TomaszJanusz/kitbag/issues/26)) ([7cbb6da](https://github.com/TomaszJanusz/kitbag/commit/7cbb6da9a25fe316301206ddb72dab9b6065a046))
* **cli:** add init wizard and scaffold ([#29](https://github.com/TomaszJanusz/kitbag/issues/29)) ([b6a9518](https://github.com/TomaszJanusz/kitbag/commit/b6a9518ba1e6232c6edeadb3dac320dac21195d3))
* **cli:** add sync, plan and status commands ([#25](https://github.com/TomaszJanusz/kitbag/issues/25)) ([5e632bc](https://github.com/TomaszJanusz/kitbag/commit/5e632bc49fa4fc3465304cc03c34b22354151f4c))
* **cli:** add validated config commands ([#30](https://github.com/TomaszJanusz/kitbag/issues/30)) ([253d189](https://github.com/TomaszJanusz/kitbag/commit/253d1892f4f7f2f669395fa71af96433bee694a3))
* **core:** add deterministic plan engine ([#19](https://github.com/TomaszJanusz/kitbag/issues/19)) ([238fe7c](https://github.com/TomaszJanusz/kitbag/commit/238fe7cffbb24c163a82a5e0540360ca8ce0a34b))
* **core:** add recoverable apply engine ([#21](https://github.com/TomaszJanusz/kitbag/issues/21)) ([de4adf2](https://github.com/TomaszJanusz/kitbag/commit/de4adf22453905bc55bdeab6f5985897b5dde7e0))
* **core:** add versioned state store with locking ([#16](https://github.com/TomaszJanusz/kitbag/issues/16)) ([cc0421d](https://github.com/TomaszJanusz/kitbag/commit/cc0421d8ee1b157b3bb7ebf2a5ffbc1b69ed6d23))
* **daemon:** add polling, drift watching and local API ([#33](https://github.com/TomaszJanusz/kitbag/issues/33)) ([d72cc5a](https://github.com/TomaszJanusz/kitbag/commit/d72cc5ae77ca097ece08d8fb36fa7c3b6ade9b0f))
* **doctor:** detect legacy hook markers in settings.json ([#37](https://github.com/TomaszJanusz/kitbag/issues/37)) ([3ae35a8](https://github.com/TomaszJanusz/kitbag/commit/3ae35a871831925d5f2ef549048dbbf21094562a))
* **manifest:** define kitbag.toml and config.toml schemas ([#10](https://github.com/TomaszJanusz/kitbag/issues/10)) ([365d555](https://github.com/TomaszJanusz/kitbag/commit/365d555e61276f87a660b9dfcf28fba8a68d64eb))
* **manifest:** parse and validate kitbag.toml ([#17](https://github.com/TomaszJanusz/kitbag/issues/17)) ([e2ff1ce](https://github.com/TomaszJanusz/kitbag/commit/e2ff1ce0832b3c749f93106555e65c2d903a634f))
* **platform:** add platform interfaces, unix, fake and windows stubs ([#11](https://github.com/TomaszJanusz/kitbag/issues/11)) ([7a7d791](https://github.com/TomaszJanusz/kitbag/commit/7a7d791a10d7d4a37b67217bd929216e4a28a058))
* **release:** add Homebrew, install script and Linux package channels ([#50](https://github.com/TomaszJanusz/kitbag/issues/50)) ([8dff2a6](https://github.com/TomaszJanusz/kitbag/commit/8dff2a6bf4eb5e3a9925744f62550e52903961b0))
* **service:** manage user background sync ([#45](https://github.com/TomaszJanusz/kitbag/issues/45)) ([1e8f5f7](https://github.com/TomaszJanusz/kitbag/commit/1e8f5f7f05e2ea54eefba3d57d9e19c87715acd1))
* **tui:** add kitbag tui with status, drift, settings and log tabs ([#51](https://github.com/TomaszJanusz/kitbag/issues/51)) ([690b574](https://github.com/TomaszJanusz/kitbag/commit/690b574d35a6db4fcf3fb89ce2eb1112f4ca4ff6))
* **ui:** add shared view-model layer ([#48](https://github.com/TomaszJanusz/kitbag/issues/48)) ([66582da](https://github.com/TomaszJanusz/kitbag/commit/66582daaa5250862ff87ff4d51e9ab422d3b56dc))
* **update:** guard versions and verify self-update ([#32](https://github.com/TomaszJanusz/kitbag/issues/32)) ([d1aa21b](https://github.com/TomaszJanusz/kitbag/commit/d1aa21b6a9626f738979f85eeff05a2df8d91704))
* **vcs:** add backend interface and system git backend ([#18](https://github.com/TomaszJanusz/kitbag/issues/18)) ([a90c7f5](https://github.com/TomaszJanusz/kitbag/commit/a90c7f5da9a3d3f3b93399e77e954a4d18595e92))
* **vcs:** add embedded go-git backend ([#27](https://github.com/TomaszJanusz/kitbag/issues/27)) ([a181f0a](https://github.com/TomaszJanusz/kitbag/commit/a181f0ae11c57934b5e91a31f3e18ec04009ac21))
* **vcs:** discover SSH credentials for the embedded backend ([#36](https://github.com/TomaszJanusz/kitbag/issues/36)) ([634b2fc](https://github.com/TomaszJanusz/kitbag/commit/634b2fc2031d6638058fb89de2e7c999916a666f))


### Bug Fixes

* **adopt:** replace legacy links in place without backups ([#35](https://github.com/TomaszJanusz/kitbag/issues/35)) ([4aa2666](https://github.com/TomaszJanusz/kitbag/commit/4aa266673ca3177715f6090816a5f235c80d2703))
* **hooks:** lint changed packages without platform-tagged failures ([#47](https://github.com/TomaszJanusz/kitbag/issues/47)) ([b349cc8](https://github.com/TomaszJanusz/kitbag/commit/b349cc8cd64e1461c53b76dcb106ff6680176374))
* **release:** pin the first release to 0.1.0 ([#54](https://github.com/TomaszJanusz/kitbag/issues/54)) ([d9bf7b9](https://github.com/TomaszJanusz/kitbag/commit/d9bf7b900d3a0e6e7c79d99eda9c5f0d34598dd2))
* **renovate:** skip mise input digest pinning ([#15](https://github.com/TomaszJanusz/kitbag/issues/15)) ([b37ef75](https://github.com/TomaszJanusz/kitbag/commit/b37ef75a3ffdaafdd5595aa1ced032f7685cbd90))
* **vcs:** skip default SSH keys when an IdentityFile is configured ([#40](https://github.com/TomaszJanusz/kitbag/issues/40)) ([60c0271](https://github.com/TomaszJanusz/kitbag/commit/60c0271b085c8093c9b9fc0ea67651eb201958b2))


### Performance Improvements

* **daemon:** cut idle RSS below the 20 MB budget ([#41](https://github.com/TomaszJanusz/kitbag/issues/41)) ([2550d7e](https://github.com/TomaszJanusz/kitbag/commit/2550d7ef8ec29be00ae619903c5817363e6c1ee4))

## Changelog

Release Please will populate this file from Conventional Commits when the
maintainer requests the first release proposal. No release has been published.
