# Go Datastar Web Application - Agent Guidelines

## Build & Test Commands
- `make build` - Build the full application (runs templ generate, tailwindcss build, go build)
- `make test` - Run all tests with `go test ./... -v`
- `go test ./path/to/package -v` - Run single test package
- `make run` - Run the application directly
- `make watch` - Live reload with Air (installs if needed)
- `templ generate` - Generate Go code from .templ files
- `tailwindcss -i tailwind.css -o internal/server/assets/css/output.css` - Build CSS

## Code Style Guidelines
- **Imports**: Group stdlib, then third-party, then local imports (no blank line between groups)
- **Formatting**: Use standard `go fmt` conventions
- **Types**: Use explicit types in struct definitions and function signatures
- **Naming**: CamelCase for exported, PascalCase for types, snake_case for unexported
- **Error Handling**: Use explicit error returns, avoid panic except in main()
- **Logging**: Use structured logging with slog, JSON output format
- **HTTP Handlers**: Implement http.Handler interface with method checking
- **Middleware**: Use alice chain for middleware composition
- **Context**: Pass context through request chain for request tracing
- **Templ**: Use .templ files for HTML templates with Go component patterns