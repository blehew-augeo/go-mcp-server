package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "github.com/denisenkom/go-mssqldb"
)

func initDatabase() *sql.DB {
	connStr := os.Getenv("MSSQL_CONNECTION_STRING")
	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		fmt.Println("Failed to open database: %w", err)
        return nil
	}
	
	if err := db.Ping(); err != nil {
		fmt.Println("Failed to connect to database: %w", err)
        db.Close()
        return nil
	}
	
	return db
}

func closeDB(db *sql.DB) {
	if db != nil {
		db.Close()
	}
}

func executeQuery(db *sql.DB, query string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database connection not available")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", err
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
			return "", err
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
	
	if len(allRows) == 0 {
		return "No data found", nil
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

	return output.String(), rows.Err()
}


func main() {
	db := initDatabase()
	defer closeDB(db)

	s := server.NewMCPServer("Encore MCP Server", "1.0.0")

	executeSQLTool := mcp.NewTool(
		"execute_sql",
		mcp.WithDescription("Execute SQL query on Microsoft SQL Server database"),
		mcp.WithString("query", mcp.Required(), mcp.Description("SQL query to execute")),
	)

	s.AddTool(executeSQLTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query parameter is required"), nil
		}

		result, err := executeQuery(db, query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to execute query: %v", err)), nil
		}

		return mcp.NewToolResultText(result), nil
	})

	
	if err := server.ServeStdio(s); err != nil {
		os.Exit(1)
	}
} 