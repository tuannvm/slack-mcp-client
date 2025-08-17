## [1.4.2](https://github.com/tuannvm/slack-mcp-client/compare/v1.4.1...v1.4.2) (2025-05-03)

## [1.4.1](https://github.com/tuannvm/slack-mcp-client/compare/v1.4.0...v1.4.1) (2025-04-25)

# [1.4.0](https://github.com/tuannvm/slack-mcp-client/compare/v1.3.0...v1.4.0) (2025-04-24)


### Features

* **slack:** add fallback to plain text for Block Kit errors ([1260fb8](https://github.com/tuannvm/slack-mcp-client/commit/1260fb8ee72a83783d1edf7752de93e5813736ec))
* **slack:** add message formatting and detection capabilities ([7dbeda3](https://github.com/tuannvm/slack-mcp-client/commit/7dbeda33bd014eb7183dd77f1d1d72517b3648b9))

# [1.3.0](https://github.com/tuannvm/slack-mcp-client/compare/v1.2.0...v1.3.0) (2025-04-24)


### Bug Fixes

* **logging:** handle MCP client close errors in main and Slack client functions ([fedf9c4](https://github.com/tuannvm/slack-mcp-client/commit/fedf9c4e8932692fe8712192085dec20501d3cf5))


### Features

* **handlers:** add LLM gateway handler with provider registry ([6aaf626](https://github.com/tuannvm/slack-mcp-client/commit/6aaf6267a173448fdada7fe4b2626c7f0dc92236))
* **langchain:** integrate LangChain provider for LLM operations ([fe187c2](https://github.com/tuannvm/slack-mcp-client/commit/fe187c2672cc1160f4d07f89ef75ff2441cc4593))
* **logging:** enhance logging with key-value pairs ([2c8a269](https://github.com/tuannvm/slack-mcp-client/commit/2c8a2695113719d05125b8fcca8406311e32032a))
* **mcp-client:** support command-based configuration for stdio mode ([9732e11](https://github.com/tuannvm/slack-mcp-client/commit/9732e117d7b6ec06fb475fa786910f6314714526))
* **slack-mcp-client:** make MCP clients optional, add godotenv package ([bb60c35](https://github.com/tuannvm/slack-mcp-client/commit/bb60c35c5cc087f43cd652b6e6344ebdd92ad2c2))

# [1.2.0](https://github.com/tuannvm/slack-mcp-client/compare/v1.1.3...v1.2.0) (2025-04-23)


### Features

* **errors:** add unified error handling system ([1d7d2d3](https://github.com/tuannvm/slack-mcp-client/commit/1d7d2d38ca424f878d756f022612a142c7698f6f))
* **langchain:** add support for LangChain LLM integration ([a4f3f5c](https://github.com/tuannvm/slack-mcp-client/commit/a4f3f5c563f9a3d63e5fbdaff0a0224731b82a22))

## [1.1.3](https://github.com/tuannvm/slack-mcp-client/compare/v1.1.2...v1.1.3) (2025-04-17)

## [1.1.2](https://github.com/tuannvm/slack-mcp-client/compare/v1.1.1...v1.1.2) (2025-04-17)

## [1.1.1](https://github.com/tuannvm/slack-mcp-client/compare/v1.1.0...v1.1.1) (2025-04-17)

# [1.1.0](https://github.com/tuannvm/slack-mcp-client/compare/v1.0.0...v1.1.0) (2025-04-16)


### Features

* **gh-pages:** add new layout and head customization for Jekyll site ([4ba7b07](https://github.com/tuannvm/slack-mcp-client/commit/4ba7b076e5d5fb6d5a1d2f920e0ab19a3ad95c7d))

# 1.0.0 (2025-04-16)


### Features

* add helm chart, update readme ([017a2cb](https://github.com/tuannvm/slack-mcp-client/commit/017a2cb9f0c49eea0c9c927a5bcabb50714dc4bf))
* add initial version of error handling and logging system ([0714f6c](https://github.com/tuannvm/slack-mcp-client/commit/0714f6cb552ce0c8862f361364679f9ce33c12b1))
* **bridge:** add LLM-MCP bridge for tool call detection and execution ([cdd7bae](https://github.com/tuannvm/slack-mcp-client/commit/cdd7bae45d10b5c2d252f9ae4c5730ada616c96c))
* **build:** update GoReleaser and add config files for CI integration ([d1b2e1e](https://github.com/tuannvm/slack-mcp-client/commit/d1b2e1ee95fa14c8afef404222ed5191c7cf4334))
* **config:** add support for new schema with command and args ([97bbf28](https://github.com/tuannvm/slack-mcp-client/commit/97bbf28f8deea0708257d058a1c990f4af1a4be5))
* **llm-mcp:** enhance tool call detection and logging ([1e9e0cc](https://github.com/tuannvm/slack-mcp-client/commit/1e9e0cc4b7aeb8d0a360a230388a6225f2157685))
* **logging:** integrate custom logger in slack-mcp-client ([afce8f9](https://github.com/tuannvm/slack-mcp-client/commit/afce8f9bf965b6a92aa793f4bcb4e979a62ed45f))
* **mcp-client:** add configurable transport modes and logging ([b794e25](https://github.com/tuannvm/slack-mcp-client/commit/b794e253c99797f0dc145d84bd4bb288365bf539))
* **mcp-server:** implement MCP server with Ollama tool integration ([a00cd99](https://github.com/tuannvm/slack-mcp-client/commit/a00cd99b8e9b9afce24ae18113a0ca4b9d007501))
* **openai:** add OpenAI tool support and configuration in LLM bridge ([91d9b25](https://github.com/tuannvm/slack-mcp-client/commit/91d9b25970276a7dc30d0e36c6f06555a874c45d))
* **slack-mcp-client:** add LLM provider support for OpenAI and Ollama ([b501ca0](https://github.com/tuannvm/slack-mcp-client/commit/b501ca0ed282272542ecff8185fbc74ce7f7e3ea))
* **slack-mcp-client:** initialize Slack MCP client project ([9a45980](https://github.com/tuannvm/slack-mcp-client/commit/9a4598012c378c0dd702a2f9c2689eaa15e1ffd0))
* Update readme, add helm chart build ([f4d7406](https://github.com/tuannvm/slack-mcp-client/commit/f4d7406b57ad8587ca2591b131840a08f14ed498))

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial setup of CI/CD pipeline
- Basic project structure
- Support for `httpHeaders` field for authentication

## [0.1.0] - YYYY-MM-DD
### Added
- Initial release
