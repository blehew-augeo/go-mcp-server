# go-mcp-server

A Model Context Protocol (MCP) server for executing SQL queries against Microsoft SQL Server databases.

> ⚠️ **Security Warning**: This tool can execute any SQL query you provide. Use with caution and ensure proper database permissions.

## Installation

1. Download the latest `mcp-server.exe` from the [releases page](https://github.com/blehew-augeo/go-mcp-server/releases/latest)

2. **Option A: Simple (no PATH setup)**
   - Place `mcp-server.exe` anywhere you like (e.g., `C:\tools\mcp-server.exe`)

3. **Option B: Add to PATH (recommended)**
   - Create a directory and move the executable:
     ```powershell
     New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\Programs\mcp-server"
     Move-Item "mcp-server.exe" "$env:LOCALAPPDATA\Programs\mcp-server\mcp-server.exe"
     ```
   - Add to your PATH:
     ```powershell
     $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
     $newPath = "$currentPath;$env:LOCALAPPDATA\Programs\mcp-server"
     [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
     ```
   - Restart your terminal

## Configuration

Add to your Cursor MCP configuration at `C:\Users\<username>\.cursor\mcp.json`:

**If you chose Option A (no PATH):**
```json
{
  "mcpServers": {
    "go-mcp-server": {
      "command": "C:\\path\\to\\your\\mcp-server.exe",
      "env": {
        "MSSQL_CONNECTION_STRING": "server=localhost\\SQLEXPRESS;database=YourDatabase;trusted_connection=true;encrypt=false;trustservercertificate=true;"
      }
    }
  }
}
```

**If you chose Option B (added to PATH):**
```json
{
  "mcpServers": {
    "go-mcp-server": {
      "command": "mcp-server.exe",
      "env": {
        "MSSQL_CONNECTION_STRING": "server=localhost\\SQLEXPRESS;database=YourDatabase;trusted_connection=true;encrypt=false;trustservercertificate=true;"
      }
    }
  }
}
```

Restart Cursor and the server will be available with SQL execution tools.

## Usage

Ask Cursor to use the `execute_sql` tool to query your database:
- "How many tables are in the database?"
- "Show me the top 10 rows from the users table"

## Development

```bash
go build -o mcp-server.exe
```