package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "github.com/denisenkom/go-mssqldb"
)

type DatabaseManager struct {
	mu             sync.RWMutex
	db             *sql.DB
	lastConnString string
}

func NewDatabaseManager() *DatabaseManager {
	return &DatabaseManager{}
}

func (dm *DatabaseManager) getConnection() (*sql.DB, error) {
	dm.mu.RLock()
	currentConnString := os.Getenv("MSSQL_CONNECTION_STRING")
	
	if dm.db != nil && dm.lastConnString == currentConnString {
		db := dm.db
		dm.mu.RUnlock()
		return db, nil
	}
	dm.mu.RUnlock()

	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.db != nil && dm.lastConnString == currentConnString {
		return dm.db, nil
	}

	if dm.db != nil {
		dm.db.Close()
		dm.db = nil
	}

	if currentConnString == "" {
		dm.lastConnString = ""
		return nil, fmt.Errorf("MSSQL_CONNECTION_STRING environment variable is not set")
	}

	db, err := sql.Open("sqlserver", currentConnString)
	if err != nil {
		dm.lastConnString = currentConnString
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		dm.lastConnString = currentConnString
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	dm.db = db
	dm.lastConnString = currentConnString
	return db, nil
}

func (dm *DatabaseManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	if dm.db != nil {
		dm.db.Close()
		dm.db = nil
	}
}

func executeQuery(dm *DatabaseManager, query string) (string, error) {
	db, err := dm.getConnection()
	if err != nil {
		return "", fmt.Errorf("database connection unavailable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("query execution failed: %v", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("failed to get column information: %v", err)
	}

	var output strings.Builder
	
	columnWidths := make([]int, len(columns))
	for i, col := range columns {
		columnWidths[i] = len(col)
	}
	
	var allRows [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return "", fmt.Errorf("failed to scan row: %v", err)
		}

		var rowValues []string
		for i := range columns {
			val := ""
			if v := values[i]; v != nil {
				if b, ok := v.([]byte); ok {
					val = string(b)
				} else {
					val = fmt.Sprintf("%v", v)
				}
			}
			rowValues = append(rowValues, val)
			if len(val) > columnWidths[i] {
				columnWidths[i] = len(val)
			}
		}
		allRows = append(allRows, rowValues)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error during row iteration: %v", err)
	}
	
	if len(allRows) == 0 {
		return "Query executed successfully. No rows returned.", nil
	}
	
	for i, col := range columns {
		output.WriteString(col)
		output.WriteString(strings.Repeat(" ", columnWidths[i]-len(col)+2))
	}
	output.WriteString("\n")
	
	for i, width := range columnWidths {
		output.WriteString(strings.Repeat("-", width))
		if i < len(columnWidths)-1 {
			output.WriteString("  ")
		}
	}
	output.WriteString("\n")
	
	for _, row := range allRows {
		for i, val := range row {
			output.WriteString(val)
			output.WriteString(strings.Repeat(" ", columnWidths[i]-len(val)+2))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

func main() {
	dm := NewDatabaseManager()
	defer dm.Close()

	s := server.NewMCPServer("SQL Server MCP", "1.0.0")

	executeSQLTool := mcp.NewTool(
		"execute_sql",
		mcp.WithDescription("Execute SQL query on Microsoft SQL Server database"),
		mcp.WithString("query", mcp.Required(), mcp.Description("SQL query to execute")),
	)

	s.AddTool(executeSQLTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("Missing required 'query' parameter"), nil
		}

		result, err := executeQuery(dm, query)
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
		}

		return mcp.NewToolResultText(result), nil
	})

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
} 