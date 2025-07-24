Setup:
   - open Powershell
   - run install script
   - create/edit mcp.json at C:\Users\<username>\.cursor\mcp.json
   - add the mcp server to the configuration:
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
   - restart Cursor
   - in Cursor Settings (CTRL+SHIFT+J) -> Tools & Integration
         - make sure the 'go-mcp-server' is listed, it should have 2 tools available
   - ask Cursor to 'use the execute_sql tool' to query the database, for example ask it how many tables are in the database

Tool for Debugging:
https://modelcontextprotocol.io/docs/tools/inspector