// Export one user_feedback.logs blob as pretty JSON.
//
// Run from the repo root:
//
//	go run ./scripts/export-feedback.go --db prod --list
//	go run ./scripts/export-feedback.go --db test --id 31
//
// --db prod reads SUPABASE_DB_URL from scripts/gcp_config.sh.
// --db test reads it from scripts/local_config.sh.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var dbURLAssignment = regexp.MustCompile(`^\s*(?:export\s+)?SUPABASE_DB_URL\s*=\s*(.*\S)\s*$`)

func main() {
	dbName := flag.String("db", "", "database to read: prod or test")
	list := flag.Bool("list", false, "list feedback rows (id, date, logs size, feedback snippet)")
	id := flag.Int("id", 0, "feedback row id to export")
	flag.Parse()

	if err := run(*dbName, *list, *id); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(dbName string, list bool, id int) error {
	if dbName != "prod" && dbName != "test" {
		return fmt.Errorf("--db must be prod or test")
	}
	if list == (id != 0) {
		return fmt.Errorf("pass exactly one of --list or --id")
	}

	url, err := loadDBURL(dbName)
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect to database: %s", redact(err.Error(), url))
	}
	defer conn.Close(ctx)

	if list {
		return listFeedback(ctx, conn)
	}
	return exportFeedback(ctx, conn, id)
}

func loadDBURL(dbName string) (string, error) {
	name := "gcp_config.sh"
	if dbName == "test" {
		name = "local_config.sh"
	}
	path, err := findConfig(name)
	if err != nil {
		return "", err
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var url string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := dbURLAssignment.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		parsed, err := unquoteShell(m[1])
		if err != nil {
			return "", fmt.Errorf("parse SUPABASE_DB_URL in %s: %w", path, err)
		}
		url = parsed
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if url == "" {
		return "", fmt.Errorf("SUPABASE_DB_URL is not set in %s", path)
	}
	return url, nil
}

func findConfig(name string) (string, error) {
	candidates := []string{
		filepath.Join("scripts", name),
		name,
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("config %s not found (run from the repo root)", name)
}

func unquoteShell(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty value")
	}
	if strings.HasPrefix(s, `"`) {
		v, err := strconv.Unquote(s)
		if err != nil {
			return "", err
		}
		return v, nil
	}
	if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") && len(s) >= 2 {
		return s[1 : len(s)-1], nil
	}
	return s, nil
}

func listFeedback(ctx context.Context, conn *pgx.Conn) error {
	rows, err := conn.Query(ctx, `
		SELECT id, created_date, coalesce(length(logs), 0), coalesce(feedback, '')
		FROM user_feedback
		ORDER BY created_date DESC, id DESC`)
	if err != nil {
		return fmt.Errorf("list feedback: %w", err)
	}
	defer rows.Close()

	fmt.Printf("%-8s  %-25s  %10s  %s\n", "id", "created", "logs_bytes", "feedback")
	count := 0
	for rows.Next() {
		var (
			id       int
			created  time.Time
			logsSize int
			feedback string
		)
		if err := rows.Scan(&id, &created, &logsSize, &feedback); err != nil {
			return fmt.Errorf("scan feedback row: %w", err)
		}
		fmt.Printf("%-8d  %-25s  %10d  %s\n",
			id,
			created.In(time.Local).Format("2006-01-02 15:04:05 MST"),
			logsSize,
			snippet(feedback, 60),
		)
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list feedback: %w", err)
	}
	if count == 0 {
		fmt.Println("(no feedback rows)")
	}
	return nil
}

func exportFeedback(ctx context.Context, conn *pgx.Conn, id int) error {
	var created time.Time
	var logs *string
	err := conn.QueryRow(ctx, `
		SELECT created_date, logs
		FROM user_feedback
		WHERE id = $1`, id).Scan(&created, &logs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("feedback id %d not found", id)
		}
		return fmt.Errorf("load feedback id %d: %w", id, err)
	}
	if logs == nil || strings.TrimSpace(*logs) == "" {
		return fmt.Errorf("feedback id %d has no logs", id)
	}

	value, err := decodeLogs(*logs)
	if err != nil {
		return fmt.Errorf("feedback id %d logs are not JSON: %w", id, err)
	}

	name := exportFilename(created, id)
	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	abs, err := filepath.Abs(name)
	if err != nil {
		abs = name
	}
	fmt.Println(abs)

	textFiles, err := writeConsoleLogs(value, name)
	if err != nil {
		return err
	}
	for _, textFile := range textFiles {
		abs, err := filepath.Abs(textFile)
		if err != nil {
			abs = textFile
		}
		fmt.Println(abs)
	}
	return nil
}

// writeConsoleLogs writes one .txt per day in the logs object. Each file sits
// next to the JSON and uses its stem, for example 2026-10-19-2026-10-17.txt.
// Days that are not strings are skipped. A payload without a logs object
// writes no text files.
func writeConsoleLogs(value any, jsonName string) ([]string, error) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, nil
	}
	rawLogs, ok := obj["logs"]
	if !ok {
		return nil, nil
	}
	days, ok := rawLogs.(map[string]any)
	if !ok {
		return nil, nil
	}

	keys := make([]string, 0, len(days))
	for day := range days {
		keys = append(keys, day)
	}
	sort.Strings(keys)

	stem := strings.TrimSuffix(jsonName, filepath.Ext(jsonName))
	written := make([]string, 0, len(keys))
	for _, day := range keys {
		text, ok := days[day].(string)
		if !ok {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		name := stem + "-" + safeFilePart(day) + ".txt"
		if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", name, err)
		}
		written = append(written, name)
	}
	return written, nil
}

func safeFilePart(day string) string {
	day = strings.TrimSpace(day)
	if day == "" || day == "." || day == ".." || strings.ContainsAny(day, `/\`) {
		return "log"
	}
	return day
}

// decodeLogs parses the stored logs text. If the value is a JSON string whose
// contents are themselves JSON, unwrap that string once.
func decodeLogs(raw string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &value); err != nil {
		return nil, err
	}
	text, ok := value.(string)
	if !ok {
		return value, nil
	}
	var inner any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &inner); err != nil {
		return value, nil
	}
	return inner, nil
}

func exportFilename(created time.Time, id int) string {
	date := created.In(time.Local).Format("2006-01-02")
	name := date + ".json"
	if _, err := os.Stat(name); err == nil {
		return fmt.Sprintf("%s-%d.json", date, id)
	}
	return name
}

func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "-"
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func redact(msg, secret string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "<db>")
}
