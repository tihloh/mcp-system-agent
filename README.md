# mcp-system-agent

Cross-platform MCP system agent for securely managing Windows and Linux hosts through AI assistants such as Gemini CLI.

## Status

Early development (`v0.1.0`). The first tool is a controlled command runner that executes approved programs directly without invoking PowerShell, CMD, Bash, or another shell.

## Why

The goal is to install one small agent on a Windows or Linux host and expose safe system-management tools over MCP.

```text
Gemini / MCP client
        |
        v
 mcp-system-agent
        |
   +----+----+
   |         |
 Windows    Linux
   |         |
 Docker     Docker
 Git        Git
 system     system
```

## Current MCP tool

### `run_command`

Inputs:

- `program` - executable name, for example `docker`, `git`, `hostname`, or `whoami`
- `args` - arguments passed directly to that executable
- `cwd` - optional working directory; it must be inside an allowed directory

Example conceptual call:

```json
{
  "program": "docker",
  "args": ["ps"]
}
```

No shell is inserted between the MCP client and the executable. The agent uses Go's direct process execution API.

## Security model

`v0.1.0` intentionally starts restricted:

- programs must be explicitly allowlisted
- executable paths are rejected; only executable names are accepted
- PowerShell, CMD, Bash and similar interpreters are not enabled by default
- optional working directories must be inside configured allowed roots
- commands have a configurable timeout
- local `config.json` is ignored by Git

This is still experimental software. Do not expose it directly to the public Internet or run it with administrator/root privileges unless you understand the risks.

## Requirements

- Go 1.24+ to build from source
- an MCP client such as Gemini CLI

## Build

```bash
git clone https://github.com/tihloh/mcp-system-agent.git
cd mcp-system-agent
go mod download
go build -o mcp-system-agent .
```

Windows PowerShell:

```powershell
go mod download
go build -o mcp-system-agent.exe .
```

## Configuration

Copy the example configuration:

Windows:

```powershell
Copy-Item config.example.json config.json
```

Linux:

```bash
cp config.example.json config.json
```

Then edit `config.json` for the machine where the agent runs.

Example:

```json
{
  "allowed_programs": [
    "hostname",
    "whoami",
    "docker",
    "git"
  ],
  "allowed_directories": [
    "D:\\Projects",
    "/srv/apps"
  ],
  "timeout_seconds": 30
}
```

You may also point to a different config file with:

```text
MCP_SYSTEM_AGENT_CONFIG=/path/to/config.json
```

If no config file exists, the built-in program allowlist is used, working-directory selection is disabled, and the timeout defaults to 30 seconds.

## Gemini CLI

After building the agent, register it as a local stdio MCP server.

Windows example:

```powershell
gemini.cmd mcp add --scope user mcp-system-agent "C:\\path\\to\\mcp-system-agent.exe"
```

Linux example:

```bash
gemini mcp add --scope user mcp-system-agent /path/to/mcp-system-agent
```

Check the connection:

```bash
gemini mcp list
```

Then start Gemini and try a harmless request such as:

```text
Use mcp-system-agent to tell me the hostname of this computer.
```

## Cross-platform builds

GitHub Actions builds:

- Windows AMD64
- Windows ARM64
- Linux AMD64
- Linux ARM64

## Planned tools

Future versions may add dedicated MCP tools such as:

- `system_info`
- `docker_ps`
- `docker_logs`
- `git_status`
- `read_file`
- `write_file`
- `service_status`
- `restart_service`

Dedicated tools will allow tighter permissions than relying on a generic command runner for every operation.

## License

MIT
