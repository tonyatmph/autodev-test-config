package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"github.com/jackc/pgx/v5"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" { 
        dsn = "postgres://postgres:postgres@localhost:5432/postgres" 
    }
	
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil { log.Fatal(err) }
	defer conn.Close(context.Background())

	schema, err := os.ReadFile("migrations/001_init_catalog.sql")
	if err != nil { log.Fatal(err) }

	_, err = conn.Exec(context.Background(), string(schema))
	if err != nil { log.Fatal(err) }
	fmt.Println("Migration applied successfully.")
}
