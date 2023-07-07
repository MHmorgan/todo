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
	pattern = flag.String("p", "alpha", "pattern to use")

	re *regexp.Regexp
)

const USAGE = ` _            _       
| |_ ___   __| | ___  
| __/ _ \ / _' |/ _ \ 
| || (_) | (_| | (_) |
 \__\___/ \__,_|\___/ 

Usage: todo [-h] [-v] [-p pattern] [dir ...]

Search through files for TODOs. 

The directories to search for files are determined by, in order:
  1. The arguments passed to the program.
  2. The current git repository.
  3. The environment variable $TODO_PATH (colon separated list of directories).

A regex pattern is used to search through the files. The -p flag can be used to
select a pattern. The default pattern is "alpha".
If the value of -p is not a known pattern, it is used as the regex pattern
directly. This custom pattern MUST contain two capture groups. The first group
is the tag, the second group is the text.

Patterns:
  alpha
    	Tag with uppercase letters and @ prefix (e.g. @TODO).
  common
    	Common comment tags.
  todo
    	Only TODO tag.

Options:
`

func main() {
	flag.Parse()

	if *help {
		fmt.Print(USAGE)
		flag.PrintDefaults()
		os.Exit(0)
	}

	logFile := setupLogging()
	if logFile != nil {
		defer logFile.Close()
	}

	if p, ok := patterns[*pattern]; ok {
		re = regexp.MustCompile(p)
	} else {
		re = regexp.MustCompile(*pattern)
	}

	// FIND FILES

	var dirsWG sync.WaitGroup
	dirs := targetDirectories()
	debug("Target directories: %v", dirs)
	files := make(chan string, 42)
	for _, dir := range dirs {
		dirsWG.Add(1)
		go findFiles(dir, files, &dirsWG)
	}

	// SEARCH FILES

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

// -----------------------------------------------------------------------------
// FINDING FILES

func findFiles(path string, files chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	debug("Searching directory %q", path)

	filepath.Walk(path, func(path string, info fs.FileInfo, _ error) error {
		if info.Mode().IsRegular() {
			ext := filepath.Ext(path)
			if _, ok := ignoreFiles[ext]; !ok {
				files <- path
			}
		} else if info.IsDir() {
			for _, dir := range ignoreDirs {
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
// SEARCHING FILES

func searchFile(file string) *Result {
	debug("Scanning file %q", file)

	f, err := os.Open(file)
	if err != nil {
		warn("could not open file: %v", file)
	}
	defer f.Close()

	cnt := 1
	scanner := bufio.NewScanner(f)
	todos := make([]Todo, 0, 10)
	for scanner.Scan() {
		line := scanner.Text()
		if tag, text, ok := searchLine(line); ok {
			debug("Found TODO: %q", line)
			todos = append(todos, Todo{cnt, tag, text})
		}
		cnt++
	}

	if err := scanner.Err(); err != nil {
		warn("scanning file %q: %v", file, err)
	}

	if len(todos) > 0 {
		return &Result{file, todos}
	}
	return nil
}

func searchLine(line string) (tag, text string, ok bool) {
	switch m := re.FindStringSubmatch(line); len(m) {
	case 0:
		break
	case 3:
		tag = m[1]
		text = m[2]
		ok = true
	default:
		bail("invalid regex: %q", re)
	}
	return
}

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
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, sr.file); err == nil {
			name = "~/" + rel
		}
	}

	var s strings.Builder
	for _, todo := range sr.todos {
		fmt.Fprintf(&s, "%-7s %s:%d: %s\n", todo.tag, name, todo.line, todo.text)
	}
	return s.String()
}

// -----------------------------------------------------------------------------
// HELPERS

func bail(msg string, args ...any) {
	txt := "ERROR: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Print(txt)
	}
	log.Fatal(txt)
}

func warn(msg string, args ...any) {
	txt := "ERROR: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Print(txt)
	}
	log.Print(txt)
}

func debug(msg string, args ...any) {
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
		bail("could not get current working directory")
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
		bail("TODO_PATH not set")
	}
	return strings.Split(val, ":")
}

func setupLogging() *os.File {
	home, err := os.UserHomeDir()
	if err != nil {
		warn("could not get home directory")
		return nil
	}

	p := filepath.Join(home, ".todo.log")
	logFile, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		bail("could not open log file: %v", p)
	}
	log.SetOutput(logFile)

	return logFile
}
