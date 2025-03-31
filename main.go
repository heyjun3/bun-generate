package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func FetchTables(ctx context.Context, db *bun.DB) ([]string, error) {
	var tables []string

	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public';
	`

	err := db.NewRaw(query).Scan(ctx, &tables)
	if err != nil {
		return nil, err
	}

	return tables, nil
}

// FetchColumns retrieves column information for a given table
func FetchColumns(ctx context.Context, db *bun.DB, tableName string) ([]map[string]interface{}, error) {
	var columns []map[string]interface{}

	query := `
		SELECT column_name::text, data_type::text
		FROM information_schema.columns
		WHERE table_name = ?;
	`

	err := db.NewRaw(query, tableName).Scan(ctx, &columns)
	if err != nil {
		return nil, err
	}

	return columns, nil
}

// Refine MapColumnType to handle additional SQL data types
func MapColumnType(sqlType string) string {
	switch sqlType {
	case "integer":
		return "int"
	case "bigint":
		return "int64"
	case "text", "character varying":
		return "string"
	case "boolean":
		return "bool"
	case "timestamp without time zone", "date":
		return "time.Time"
	default:
		return "interface{}" // Fallback for unknown types
	}
}

// Refine decoding logic for column_name and data_type
func GenerateStruct(tableName string, columns []map[string]interface{}) string {
	structCode := fmt.Sprintf("type %s struct {\n", tableName)

	for _, column := range columns {
		fmt.Printf("Raw column data: %+v\n", column) // Debugging output
		var columnName, dataType string

		// Decode column_name
		if colName, ok := column["column_name"].([]uint8); ok {
			columnName = string(colName)
		} else if colName, ok := column["column_name"].(string); ok {
			columnName = colName
		} else {
			fmt.Printf("Unexpected column_name type: %T\n", column["column_name"])
		}

		// Decode data_type
		if colType, ok := column["data_type"].([]uint8); ok {
			dataType = MapColumnType(string(colType))
		} else if colType, ok := column["data_type"].(string); ok {
			dataType = MapColumnType(colType)
		} else {
			fmt.Printf("Unexpected data_type type: %T\n", column["data_type"])
		}

		// Clean up columnName to remove unexpected characters
		columnName = cleanString(columnName)

		// Add field to struct
		if columnName != "" && dataType != "" {
			structCode += fmt.Sprintf("\t%s %s `bun:\"%s\"`\n", columnName, dataType, columnName)
		}
	}

	structCode += "}\n"
	return structCode
}

// cleanString removes unexpected characters from a string
func cleanString(input string) string {
	return strings.TrimSpace(input)
}

// Save the generated struct to a file within a package directory
func SaveStructToFile(fileName, structCode string) error {
	packageDir := "bunmodels"
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		return err
	}

	packageDeclaration := "package bunmodels\n\n"
	fullCode := packageDeclaration + structCode
	filePath := fmt.Sprintf("%s/%s", packageDir, fileName)
	return os.WriteFile(filePath, []byte(fullCode), 0644)
}

func main() {
	// PostgreSQL connection string
	dsn := "postgres://postgres:postgres@localhost:5432/dbname?sslmode=disable"
	sqldb, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}

	// Create Bun DB instance
	db := bun.NewDB(sqldb, pgdialect.New())

	// Test connection
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		panic(err)
	}

	// Fetch table names
	tables, err := FetchTables(ctx, db)
	if err != nil {
		panic(err)
	}

	fmt.Println("Tables:", tables)

	for _, table := range tables {
		columns, err := FetchColumns(ctx, db, table)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Columns for table %s: %v\n", table, columns)
		structCode := GenerateStruct(table, columns)

		// Save the struct to a file
		fileName := fmt.Sprintf("%s_struct.go", table)
		if err := SaveStructToFile(fileName, structCode); err != nil {
			panic(err)
		}
		fmt.Printf("Struct for table %s saved to %s\n", table, fileName)
	}
}
