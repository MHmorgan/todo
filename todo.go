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
	help    = flag.Bool("h", false, "show help")
	verbose = flag.Bool("v", false, "verbose output")

	// @TODO Support TODO_TAGS environment variable to override regex
	re = regexp.MustCompile(`^\s+(//|#|/?\*)\s+(@[A-Z]+) (.*)`)

	home string

	igoreDirs = []string{
		".git",
		".idea",
		".vscode",
		".gradle",
		"venv",
	}
)

func init() {
	var err error
	home, err = os.UserHomeDir()
	if err != nil {
		fatalf("could not get home directory")
	}
}

const USAGE = `Usage: todo [-h] [-v] [command]

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
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fatalf("could not open log file: %v", logPath)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	// Find files to search
	var dirsWG sync.WaitGroup
	dirs := targetDirectories()
	logf("Target directories: %v", dirs)
	files := make(chan string, 42)
	for _, dir := range dirs {
		dirsWG.Add(1)
		go findFiles(dir, files, &dirsWG)
	}

	// Search files
	var filesWG sync.WaitGroup
	results := make(chan Result, 42)
	for i := 0; i < 7; i++ {
		filesWG.Add(1)
		go func() {
			for file := range files {
				if res := searchFile(file); res != nil {
					results <- *res
				}
			}
			filesWG.Done()
		}()
	}

	go func() {
		dirsWG.Wait()
		close(files)
	}()

	go func() {
		filesWG.Wait()
		close(results)
	}()

	for r := range results {
		fmt.Print(r)
	}
}

func findFiles(path string, files chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	logf("Searching directory %q", path)
	filepath.Walk(path, func(path string, info fs.FileInfo, _ error) error {
		if info.Mode().IsRegular() {
			files <- path
		} else if info.IsDir() {
			for _, dir := range igoreDirs {
				if m, err := filepath.Match(dir, info.Name()); err != nil {
					panic(err)
				} else if m {
					return filepath.SkipDir
				}
			}
		}
		return nil
	})
}

// -----------------------------------------------------------------------------
// FILE SEARCH

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
		fmt.Fprintf(&s, "%-7s %s:%d: %s\n", todo.tag, name, todo.line, todo.text)
	}
	return s.String()
}

func searchFile(file string) *Result {
	logf("Scanning file %q", file)

	f, err := os.Open(file)
	if err != nil {
		errorf("could not open file: %v", file)
	}
	defer f.Close()

	cnt := 1
	scanner := bufio.NewScanner(f)
	todos := make([]Todo, 0, 10)
	for scanner.Scan() {
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); len(m) > 0 {
			logf("Found todo: %q\n", m)
			todos = append(todos, Todo{cnt, m[2], m[3]})
		}
		cnt++
	}

	if err := scanner.Err(); err != nil {
		errorf("scanning file %q: %v", file, err)
	}

	if len(todos) > 0 {
		return &Result{file, todos}
	}
	return nil
}

// -----------------------------------------------------------------------------
// HELPERS

func fatalf(msg string, args ...any) {
	txt := "ERROR: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Print(txt)
	}
	log.Fatal(txt)
}

func errorf(msg string, args ...any) {
	txt := "ERROR: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Print(txt)
	}
	log.Print(txt)
}

func logf(msg string, args ...any) {
	txt := fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Print(txt)
	}
	log.Print(txt)
}

func targetDirectories() []string {
	if root := flag.Arg(0); root != "" {
		return []string{root}
	}

	dir, err := os.Getwd()
	if err != nil {
		fatalf("could not get current working directory")
	}

	for dir != "" {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			return []string{dir}
		}
		dir, _ = path.Split(dir)
		dir = strings.TrimSuffix(dir, "/")
	}

	val, ok := os.LookupEnv("TODO_PATH")
	if !ok {
		fatalf("TODO_PATH not set")
	}
	return strings.Split(val, ":")
}
