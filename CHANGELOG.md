# Changelog

## [0.2.2](https://github.com/vertracloud/sdk-api-go/compare/v0.2.1...v0.2.2) (2026-10-18)


### Bug Fixes

* **apps:** support async app creation with building status and WaitUntilReady ([53bc8f8](https://github.com/vertracloud/sdk-api-go/commit/53bc8f88de74394651e6528455bfbb79a9f0bc24))

## [0.2.1](https://github.com/vertracloud/sdk-api-go/compare/v0.2.0...v0.2.1) (2026-10-15)


### Features

* **blob:** add RenameOnConflict to Put and Copy ([196985d](https://github.com/vertracloud/sdk-api-go/commit/196985d585a621ae672ab42b1e66e1a4020c926a))

## [0.2.0](https://github.com/vertracloud/sdk-api-go/compare/v0.1.5...v0.2.0) (2026-10-15)


### Features

* add Blob Storage client ([c2426d4](https://github.com/vertracloud/sdk-api-go/commit/c2426d47f4c6cb4e9f40d035ae43024006636348))
* add ExpiresAt to Pix payment and order status ([d5f2594](https://github.com/vertracloud/sdk-api-go/commit/d5f25940708cf0fc28cb24d104f3766530e56996))
* web_content on blob custom domains ([4c9f77d](https://github.com/vertracloud/sdk-api-go/commit/4c9f77ddea03f3d71b4b7620454cc931124aecf2))

## [0.1.5](https://github.com/vertracloud/sdk-api-go/compare/v0.1.4...v0.1.5) (2026-10-12)


### Features

* **apps:** add FirstStartedAt to Release ([f03bf54](https://github.com/vertracloud/sdk-api-go/commit/f03bf54f611a3d2549b5e0d05304b5298bd5cfca))
* **apps:** release history and upload message ([eebe958](https://github.com/vertracloud/sdk-api-go/commit/eebe9581c47cfb5fa64fec043ce5b260b6e41c2b))
* **apps:** rollback preview, rollback and deploy settings ([db5c72f](https://github.com/vertracloud/sdk-api-go/commit/db5c72fe2e0b30f6bf3c5443023fc240badb148c))

## [0.1.4](https://github.com/vertracloud/sdk-api-go/compare/v0.1.3...v0.1.4) (2026-10-10)


* release 0.1.4 ([c7eceff](https://github.com/vertracloud/sdk-api-go/commit/c7eceffb6f381f360d76a4586ead4698a4549977))


### Features

* **apps:** add destination path option to file upload ([378f470](https://github.com/vertracloud/sdk-api-go/commit/378f4704c59b8cb4e43474661d233c98db676745))

## [0.1.3](https://github.com/vertracloud/sdk-api-go/compare/v0.1.2...v0.1.3) (2026-10-05)


* release 0.1.3 ([1276150](https://github.com/vertracloud/sdk-api-go/commit/12761506fdd85276d86c2a94a836e53d057a1708))


### Features

* **billing:** billing details, card provider and new order fields ([d042196](https://github.com/vertracloud/sdk-api-go/commit/d04219681791737bb88f475708437b92a278d9a1))

## [0.1.2](https://github.com/vertracloud/sdk-api-go/compare/v0.1.1...v0.1.2) (2026-09-30)


### ⚠ BREAKING CHANGES

* remove AutoRestart field from the application contract

* release 0.1.2 ([cc4a136](https://github.com/vertracloud/sdk-api-go/commit/cc4a136fd7639ca238520860466ff8247c68fa44))


### Features

* remove AutoRestart field from the application contract ([a27ebd1](https://github.com/vertracloud/sdk-api-go/commit/a27ebd1adb708f49ed06c8527a0b38c2de14ae06))

## [0.1.1](https://github.com/vertracloud/sdk-api-go/compare/v0.1.0...v0.1.1) (2026-09-23)


### Bug Fixes

* drop resource-organization segment from folder and favorite routes ([f73fcbf](https://github.com/vertracloud/sdk-api-go/commit/f73fcbf40bb0132b1e711344264850aa11bff8db))

## 0.1.0 (2026-09-23)


### Features

* add request helpers and nullable fields ([aac9b01](https://github.com/vertracloud/sdk-api-go/commit/aac9b018dbb8d1aa48eca0ef317bb3ef0aa712ab))
* add VertraClient with apps, databases, snapshots, account, workspaces, billing and status services ([707fe6c](https://github.com/vertracloud/sdk-api-go/commit/707fe6c8c1674edd844fdda802e2f33b15cd49a2))
* **rest:** add HTTP client with API key auth and typed errors ([4f69b2c](https://github.com/vertracloud/sdk-api-go/commit/4f69b2c48964c56cf1a7a086f820da1cf022aeef))
* **rest:** add realtime log streaming over SSE ([c52cba1](https://github.com/vertracloud/sdk-api-go/commit/c52cba14870dfddd1cc288614d92b18eb3ee9b55))
