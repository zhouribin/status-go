package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Configuration by flags.
var (
	paths []string
	skips []string

	skip    = flag.String("skip", "", "list of directories to skip (i.e. vendor), comma separated")
	verbose = flag.Bool("v", false, "enabling more output during processing")
)

// main parses the arguments and starts the scanning.
func main() {
	// Prepare configuration.
	flag.Usage = usage
	flag.Parse()
	paths = flag.Args()
	skips = strings.Split(*skip, ",")
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Start work.
	fmt.Printf("godepscan v0.1.0\n")
	if err := scan(); err != nil {
		fmt.Printf("error during scanning: %q\n", err)
	}
	fmt.Println("done.")
}

// usage prints help and exits the program.
func usage() {
	fmt.Fprintf(os.Stderr, "usage: godepscan [flags] [path ...]\n")
	flag.PrintDefaults()
	os.Exit(2)
}
