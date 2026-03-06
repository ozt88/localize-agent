# Project Workspace Layout

Shared engine code stays in `workflow/`.

Each translation project lives in `projects/<project-name>/`:

- `project.json`: project profile consumed by `--project`
- `context/`: project-specific context/rules
- `source/`: project input source files
- `output/`: project output/checkpoint/db/export artifacts
- `cmd/`: project-local run scripts

Examples:

```powershell
go run ./workflow/cmd/go-translate --project esoteric-ebb
go run ./workflow/cmd/go-evaluate  --project esoteric-ebb
```

Override any value via CLI flags when needed.
