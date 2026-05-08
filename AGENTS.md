# Repository Guidelines

## Project Structure & Module Organization

This is a Go workspace managed by `go.work` with three modules:

- `serialport/`: shared serial-port configuration, discovery, prompting, and open logic.
- `tri-detector/`: CLI tool for reading drone detector serial data, with `client/`, `handler/`, `parser/`, and `ui/` packages.
- `gpio-controller/`: CLI tool for Linux sysfs GPIO output control, with `gpio/` hardware logic and `ui/` prompts.

Tests live beside package code as `*_test.go`, currently in `gpio-controller/gpio/`, `tri-detector/handler/`, and `tri-detector/parser/`.

## Build, Test, and Development Commands

Run commands from the repository root. Because the root is a workspace, use explicit module patterns instead of `go test ./...`.

```bash
go build ./gpio-controller/... ./serialport/... ./tri-detector/...
```

Builds all workspace packages.

```bash
go test ./gpio-controller/... ./serialport/... ./tri-detector/...
```

Runs all unit tests across the three modules.

```bash
go run ./tri-detector -port /dev/tty.usbserial-XXXX -baud 115200
go run ./gpio-controller 96
```

Runs the detector parser or GPIO controller locally. Omit flags/arguments to use interactive prompts where supported.

## Coding Style & Naming Conventions

Use Go 1.25.x and format all Go changes with `gofmt -w`. Keep package names short and lowercase (`parser`, `handler`, `gpio`). Exported symbols should use standard Go PascalCase, while local helpers should stay camelCase. Existing CLI prompts, comments, and user-facing errors are primarily Chinese; keep new user-facing messages consistent with that style.

## Testing Guidelines

Use Go’s standard `testing` package. Add table-driven tests for parsers, protocol edge cases, and hardware abstractions where file or port operations can be stubbed. Name tests `TestXxx` and keep fixtures local to the package. For GPIO behavior, avoid requiring real `/sys/class/gpio`; follow the existing pattern of replaceable filesystem functions.

## Commit & Pull Request Guidelines

Recent history uses Conventional Commit-style prefixes such as `fix:` and `refactor:`. Use short, imperative subjects, for example `fix: harden gpio cleanup path`. Pull requests should describe the behavior change, list test commands run, and note hardware or OS assumptions. Include terminal output or screenshots when changing interactive CLI flows.

## 提交与合并请求
提交信息保持简短、祈使句，可带作用域，例如 `fix(tcp): 修复状态查询超时`、`chore(portal): 添加 Air 本地热重载配置`。一次提交尽量只聚焦一个模块或一个功能。PR 需要说明影响模块、行为或配置变化、已执行命令，并为 Wails 或其他 UI 改动附上截图；如有关联问题单，请一并链接。

Codex 执行提交时，commit message 必须使用中文并符合规范格式：`type(scope): 中文祈使句描述`。`type` 优先使用 `feat`、`fix`、`refactor`、`chore`、`docs`、`test`、`build`、`ci`，`scope` 使用小写英文模块名或目录名，例如 `directed-strike`、`offline-map`、`updater`、`portal`；没有明确模块时可省略 scope，例如 `chore: 整理构建配置`。除专有名词、协议名、模块名外，描述部分不要使用英文；用户只说“提交代码”且未指定 message 时，Codex 必须根据本次改动自动生成符合该规则的中文 commit message。

## Security & Configuration Tips

Do not commit machine-specific serial device names, credentials, or generated local binaries unless they are intentional release artifacts. GPIO access usually requires Linux permissions; document required privileges when changing setup or runtime behavior.
# Project Instructions for AI Code Assistant with gopls-mcp

## Context
You are an AI programming assistant helping users with Go code. You have access to gopls-mcp tools for semantic code analysis.

## CRITICAL PROHIBITIONS (NEVER DO THIS)
1. NEVER use `go_search` for text content (comments, strings, TODOs). Use `Grep` tool.
2. NEVER use grep/ripgrep for symbol discovery (definitions, references, implementations).
3. NEVER fall back from exclusive capabilities (see Tool Selection Guide).

<!-- Marker: AUTO-GEN-START -->
## Tool Selection Guide

### Code relationships (Exclusive Capabilities - NO FALLBACK)
| Task | Tool |
|------|------|
| Find interface implementations | go_implementation |
| Trace call relationships | go_get_call_hierarchy |
| Find symbol references | go_symbol_references |
| Jump to definition | go_definition |
| Analyze dependencies | go_get_dependency_graph |
| Preview renaming | go_dryrun_rename_symbol |

### Code exploration (Enhanced Capabilities - FALLBACK ALLOWED)
| Task | Tool | Fallback after 3 failures |
|------|------|---------------------------|
| List package symbols | go_list_package_symbols | Glob + Read |
| List module packages | go_list_module_packages | find |
| Analyze workspace | go_analyze_workspace | Manual exploration |
| Quick project overview | go_get_started | Read README + go.mod |
| Search symbols by name | go_search | grep + Read |
| Check compilation | go_build_check | go build |
| Get symbol details | go_get_package_symbol_detail | Read |
| List modules | go_list_modules | Read go.mod |
<!-- Marker: AUTO-GEN-END -->

## Integration Workflow
1. **Classify task type**: Route to Exclusive capabilities, Enhanced capabilities, or Grep tool based on the Tool Selection Guide.
2. **Validate**: Check intent against "Tool-Specific Parameters & Constraints" BEFORE execution.
3. **Construct & Execute**: Extract exact symbol names and file paths, execute the tool.
4. **Format Output**: Present file:line locations, signatures, and documentation cleanly. Do not dump raw JSON.

## Tool-Specific Parameters & Constraints

* **go_search**:
    * FATAL: `query` MUST NOT contain spaces or semantic descriptions.
    * Must be symbol names only (single token). Correct: `query="ParseInt"`.
    * Does NOT search comments or documentation.
* **go_implementation**:
    * Only for interfaces and types. STRICTLY PROHIBITED for functions.
* **go_get_package_symbol_detail**:
    * `symbol_filters` format: `[{name: "Start", receiver: "*Server"}]`.
    * `receiver` requires exact string match (`"*Server"` != `"Server"`).
* **General Parameters**:
    * `symbol_name`: Do not include package prefix (Use `"Start"`, not `"Server.Start"`).
    * `context_file`: Obtain strictly from the current file being analyzed.

## Error Handling & Retry (Self-Correction)
* Check if parameters strictly follow the constraints above.
* Try a shorter/simpler symbol name.
* Re-analyze code context before retrying.

## Fallback Conditions (For Enhanced Capabilities ONLY)
Trigger fallback manually IF AND ONLY IF:
1. 3 consecutive tool failures.
2. Timeout exceeds 30 seconds.
3. Empty result returned when code existence is absolutely certain.
*Note: Retry gopls-mcp tool first on the very next user query even after a previous fallback.*
