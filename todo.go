package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	help = flag.Bool("h", false, "show help")
	// @TODO Support TODO_TAGS environment variable to override regex
	re   = regexp.MustCompile(`^\s+(//|#|/?\*)\s+(@[A-Z]+) (.*)`)
	home string
)

func init() {
	var err error
	home, err = os.UserHomeDir()
	if err != nil {
		bail("could not get home directory")
	}
}

const USAGE = `Usage: todo [-h] [command]

Look for TODOs in files.

@FIXME - Issue that needs to be fixed.q
@HACK  - A hack that needs to be replaced.
@TEMP  - Temporary solution that needs to be replaced.
@TODO  - Action item that needs to be done.
@XXX   - A note to the reader.

Options:
`

func main() {
	flag.Parse()

	if *help {
		fmt.Print(USAGE)
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Setup logging
	logPath := filepath.Join(home, ".todo.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		bail("could not open log file: %v", logPath)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// Find files to search
	var wg sync.WaitGroup
	dirs := targetDirectories()
	log.Printf("Target directories: %v", dirs)
	files := make(chan string, 42)
	for _, dir := range dirs {
		wg.Add(1)
		go findFiles(dir, files, &wg)
	}

	// Search files
	results := make(chan Result, 42)
	go func() {
		for file := range files {
			wg.Add(1)
			go searchFile(file, results, &wg)
		}
	}()

	go func() {
		wg.Wait()
		close(files)
		close(results)
	}()

	for r := range results {
		fmt.Print(r)
	}
}

func findFiles(path string, files chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("Searching directory %q", path)
	filepath.Walk(path, func(path string, info fs.FileInfo, _ error) error {
		if info.Mode().IsRegular() {
			files <- path
		}
		return nil
	})
}

// -----------------------------------------------------------------------------
// SEARCH

type Result struct {
	file  string
	todos []Todo
}

type Todo struct {
	line int
	tag  string
	text string
}

func (sr Result) String() string {
	name := sr.file
	if rel, err := filepath.Rel(home, sr.file); err == nil {
		name = "~/" + rel
	}

	var s strings.Builder
	for _, todo := range sr.todos {
		fmt.Fprintf(&s, "%s:%d: %s\n", name, todo.line, todo.text)
	}
	return s.String()
}

func searchFile(file string, res chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("Scanning file %q", file)

	f, err := os.Open(file)
	if err != nil {
		bail("could not open file: %v", file)
	}
	defer f.Close()

	cnt := 1
	scanner := bufio.NewScanner(f)
	todos := make([]Todo, 0, 10)
	for scanner.Scan() {
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); len(m) > 0 {
			log.Printf("Found todo: %q\n", m)
			todos = append(todos, Todo{cnt, m[2], m[3]})
		}
		cnt++
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error scanning file %q: %v", file, err)
	}

	if len(todos) > 0 {
		res <- Result{file, todos}
	}
}

// -----------------------------------------------------------------------------
// HELPERS

func bail(msg string, args ...any) {
	s := fmt.Sprintf(msg, args...)
	_, _ = fmt.Fprintln(os.Stderr, "ERROR: "+s)
	os.Exit(1)
}

func targetDirectories() []string {
	if root := flag.Arg(0); root != "" {
		return []string{root}
	}

	dir, err := os.Getwd()
	if err != nil {
		bail("could not get current working directory")
	}

	for dir != "" {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			return []string{dir}
		}
		dir, _ = path.Split(dir)
	}

	val, ok := os.LookupEnv("TODO_PATH")
	if !ok {
		bail("TODO_PATH not set")
	}
	return strings.Split(val, ":")
}
