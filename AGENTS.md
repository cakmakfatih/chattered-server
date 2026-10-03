# Project Instructions

## Git operations

- Do not perform any Git operation unless the user explicitly asks for it. This includes read-only inspection commands such as `git status`, `git log`, and `git diff`, as well as `fetch`, `checkout`, `branch`, `merge`, `commit`, `push`, and similar operations.
- Commit only when the user explicitly asks. Never commit or push without explicit user authorization.
- When Git work is authorized, follow Git Flow principles.
- Write all commit messages in English.

## Language and naming

- Write all documentation in English, including README files and other documentation files.
- Use English for identifiers and names, including variables, functions, types, files, and resources. Follow established project and language conventions for casing and formatting.
- Use Turkish only for user-facing language inside the application. Turkish does not apply to identifiers, technical names, documentation, or commit messages.

## Code quality

- Apply Clean Code Architecture naming principles: names should communicate meaning and responsibility while following the project's existing language and naming conventions.
- Name functions clearly for the action they perform and the responsibility they own.
- Keep functions short, focused, understandable, and limited to a coherent responsibility. Use judgment rather than enforcing a rigid line-count limit.
- Avoid excessive context and unrelated responsibilities within a function.

## Go project layout

- For Go projects created or changed in this workspace, follow the patterns from `golang-standards/project-layout`.
- Place executable entry points under `cmd/<app>` and private application code under `internal/`.
- Add optional directories such as `pkg`, `api`, or `deployments` only when the project needs them.
