# Cursor rules (generated)

This repo’s Cursor rules live in `.cursor/rules/*.mdc`.

- `00-core.mdc`: always-on repo conventions (secrets, error wrapping, gofmt, etc.)
- `05-docs-and-language.mdc`: 简体中文回复 + 需求/设计等文档统一放 `docs/`
- `10-go-style.mdc`: Go coding/testing standards (`**/*.go`)
- `20-api-http.mdc`: API + middleware conventions (handlers, routing, responses)
- `30-db-gorm.mdc`: DB/Gorm repository conventions (`internal/db/**/*.go`)
- `90-project-structure.mdc`: 项目结构、分层与代码组织约定（常驻说明）