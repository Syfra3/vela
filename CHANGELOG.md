# Changelog

## [1.3.1](https://github.com/Syfra3/vela/compare/v1.3.0...v1.3.1) (2026-04-18)


### Bug Fixes

* **graph:** support windows build by abstracting file locking ([0062c59](https://github.com/Syfra3/vela/commit/0062c590a29f79d99adc3b848ef8beac1a1e1b97))

## [1.3.0](https://github.com/Syfra3/vela/compare/v1.2.0...v1.3.0) (2026-04-18)


### Features

* **tui:** add projects and uninstall screens, doctor integration checks, graph lock ([#15](https://github.com/Syfra3/vela/issues/15)) ([4467cd3](https://github.com/Syfra3/vela/commit/4467cd3fb5f62d961ef1ea9efa579d52a9294dcc))

## [1.2.0](https://github.com/Syfra3/vela/compare/v1.1.4...v1.2.0) (2026-04-17)


### Features

* **graph:** persist bench history and enrich ancora metadata ([#13](https://github.com/Syfra3/vela/issues/13)) ([8d952ad](https://github.com/Syfra3/vela/commit/8d952ad1b85d48668c161c0200895192fb9de0ec))

## [1.1.4](https://github.com/Syfra3/vela/compare/v1.1.3...v1.1.4) (2026-04-16)


### Bug Fixes

* use macos-14 runners for darwin builds ([a8e2e92](https://github.com/Syfra3/vela/commit/a8e2e92187ab2c0917d2f0e3a0294e4a094bfed6))

## [1.1.3](https://github.com/Syfra3/vela/compare/v1.1.2...v1.1.3) (2026-04-16)


### Bug Fixes

* remove duplicate runs-on in matrix strategy ([0865114](https://github.com/Syfra3/vela/commit/08651149e94f7380a6117d090886bd0a5437bced))
* use ubuntu for all builds with Zig cross-compilation ([d929bee](https://github.com/Syfra3/vela/commit/d929bee873e76cc77f47e5850f771360f58470d5))

## [1.1.2](https://github.com/Syfra3/vela/compare/v1.1.1...v1.1.2) (2026-04-16)


### Bug Fixes

* use macos-latest instead of deprecated macos-13 runner ([8ce906f](https://github.com/Syfra3/vela/commit/8ce906fc7d8a76debe552988b7647fb64fc059f0))

## [1.1.1](https://github.com/Syfra3/vela/compare/v1.1.0...v1.1.1) (2026-04-16)


### Bug Fixes

* **detect:** add missing Collect and EnsureVelignore functions ([cb8903e](https://github.com/Syfra3/vela/commit/cb8903e1cd5669115c9c6470fd1a73abe0b9a76a))

## [1.1.0](https://github.com/Syfra3/vela/compare/v1.0.1...v1.1.0) (2026-04-16)


### Features

* **detect:** add recursive file walker with gitignore and tech-aware defaults ([02701d6](https://github.com/Syfra3/vela/commit/02701d6625a8cfef3787e8725bd5547b17918d34))
* **detect:** recursive file walker with gitignore and tech-aware defaults ([f6dcf06](https://github.com/Syfra3/vela/commit/f6dcf06a9b8203912fe05344ee0b2ac503fad7d5))


### Bug Fixes

* release-please build binary ([275db45](https://github.com/Syfra3/vela/commit/275db45ead03e00eed03933c4fc8cdfe754fdd11))

## [1.0.1](https://github.com/Syfra3/vela/compare/v1.0.0...v1.0.1) (2026-04-16)


### Bug Fixes

* **ci:** use zig cc for CGO cross-compilation ([cf79b3b](https://github.com/Syfra3/vela/commit/cf79b3b0e3ab434eb8543739d2a6e265594d0021))

## 1.0.0 (2026-04-16)


### Features

* add detailed system requirements checks display to setup wizard ([e5057ca](https://github.com/Syfra3/vela/commit/e5057caaad1a3c79bc05bc3340be060e10331c29))
* add model selection framework and Syfra ecosystem integration docs ([bf4d22f](https://github.com/Syfra3/vela/commit/bf4d22feef536ffea5bfb154a2a032faed10e469))
* ancora integration with real-time IPC sync, knowledge graph reconciliation, and release pipeline ([56916a1](https://github.com/Syfra3/vela/commit/56916a140125eb74e5f0c0658a14b5896bacdfe8))
* ancora integration, real-time IPC sync, knowledge graph, and release pipeline ([dddbcd1](https://github.com/Syfra3/vela/commit/dddbcd1f49eb2329b743696ce44a4c562c00ca6d))
* bootstrap Vela project structure ([3d3a384](https://github.com/Syfra3/vela/commit/3d3a38400650845bb9296e13bd6ce2bf2f990207))
* implement Phase 0 PoC — file detect, tree-sitter Go AST, gonum graph, JSON export, TUI, CLI ([1853461](https://github.com/Syfra3/vela/commit/1853461e3fe1fab4bf7fbfb7f89c0e46e2e5c166))
* implement Phase 1 — Python/TS AST, LLM providers, doc/PDF extraction, chunking, cache, config, doctor ([a7c2465](https://github.com/Syfra3/vela/commit/a7c246565c6208f3ab8343bd180395e873b5feba))
* implement Phase 2 — Leiden clustering, god nodes, surprise edges, GRAPH_REPORT.md, HTML, Obsidian ([bc01928](https://github.com/Syfra3/vela/commit/bc01928901004e3b8692c29c8e4a9bab7ea0b76d))
* implement Phase 3 — full Bubbletea TUI with worker pool, query mode, CLI path/explain/query commands ([0d13195](https://github.com/Syfra3/vela/commit/0d1319596d434f2b2090bfcab5bc3a55946aa96f))
* implement Phase 4 — watch mode, MCP server, Neo4j export, git hooks ([4faff65](https://github.com/Syfra3/vela/commit/4faff65585553d3abdda132bdb3ddbc399b1de77))
* implement TUI integration with install wizard, extract, query, doctor, and config screens ([58e6ed9](https://github.com/Syfra3/vela/commit/58e6ed9100a764c280da7c38d0b074e4c41f3a28))
* rebuild setup wizard with comprehensive Ollama dependency management ([b05e116](https://github.com/Syfra3/vela/commit/b05e116a8476327d5346406939b6afc2b2847e54))
* unify TUI dashboard layout and rebuild setup wizard with linear step-by-step flow ([af6bb21](https://github.com/Syfra3/vela/commit/af6bb21804d024a5497424f71c0564e9ce09b25c))


### Bug Fixes

* **wizard:** auto-run system check with hardware detection and show install progress ([ede454b](https://github.com/Syfra3/vela/commit/ede454bc1a6dd5bc53b52328d7c409b92102baa9))
