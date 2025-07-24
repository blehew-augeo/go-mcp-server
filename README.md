# go-mcp-server

A Model Context Protocol (MCP) server for executing SQL queries against Microsoft SQL Server databases.

## Installation

```powershell
iex (iwr -useb https://raw.githubusercontent.com/blehew-augeo/go-mcp-server/main/install.ps1).Content
```

## Configuration

Add to your Cursor MCP configuration at `C:\Users\<username>\.cursor\mcp.json`:

```json
{
  "mcpServers": {
    "go-mcp-server": {
      "command": "mcp-server.exe",
      "env": {
        "MSSQL_CONNECTION_STRING": "your-connection-string-here"
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