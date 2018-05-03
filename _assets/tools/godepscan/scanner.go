package main

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// importsToFiles maps names of imports to a list of files importing them.
type importsToFiles map[string][]string

// scan retrieves go files and scans them for their imports.
func scan() error {
	var filenames []string
	for _, p := range paths {
		fs, err := findGoFiles(p)
		if err != nil {
			return err
		}
		filenames = append(filenames, fs...)
	}
	fmt.Printf("fonud %d files ...\n", len(filenames))
	intImports, extImports, err := scanGoFiles(filenames)
	if err != nil {
		return err
	}
	fmt.Printf("found %d internal and %d external imports ...\n", len(intImports), len(extImports))
	return nil
}

// findGoFiles retrieves the names of all go files in
// given directory and below.
func findGoFiles(dir string) ([]string, error) {
	if *verbose {
		fmt.Printf("scanning %q for go files ...\n", dir)
	}

	isGoFile := func(fi os.FileInfo) bool {
		name := fi.Name()
		return !fi.IsDir() && !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".go")
	}
	skipsContains := func(name string) bool {
		for _, s := range skips {
			if s == name {
				return true
			}
		}
		return false
	}
	skipDirectory := func(fi os.FileInfo) bool {
		return fi.IsDir() && skipsContains(fi.Name())
	}

	var filenames []string
	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", dir, err)
			return err
		}
		if skipDirectory(fi) {
			return filepath.SkipDir
		}
		if isGoFile(fi) {
			filenames = append(filenames, path)
		}
		return nil
	})
	return filenames, err
}

// scanGoFiles iterates over the given filenames, scan the files
// for imports and creates a mapping with imports as key and
// list of filenames importing it.
func scanGoFiles(filenames []string) (importsToFiles, importsToFiles, error) {
	intImports := importsToFiles{}
	extImports := importsToFiles{}
	for _, filename := range filenames {
		fileImports, err := scanGoFile(filename)
		if err != nil {
			return nil, nil, err
		}
		// Map imports to filenames.
		for _, fileImport := range fileImports {
			if strings.Contains(fileImport, ".") {
				extImports[fileImport] = append(extImports[fileImport], filename)
			} else {
				intImports[fileImport] = append(extImports[fileImport], filename)
			}
		}
	}
	return intImports, extImports, nil
}

// scanGoFile scans one go file for its imports.
func scanGoFile(filename string) ([]string, error) {
	if *verbose {
		fmt.Printf("scanning %q for imports ...\n", filename)
	}

	src, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	// Initialize the scanner.
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile(filename, fset.Base(), len(src))
	s.Init(file, src, nil, scanner.ScanComments)
	// Repeated calls to Scan yield the token sequence found in the input.
	var imports []string
	for {
		_, tok, _ := s.Scan()
		if tok == token.EOF {
			break
		} else if tok == token.IMPORT {
			blockImports, err := scanImports(s)
			if err != nil {
				return nil, err
			}
			imports = append(imports, blockImports...)
		}
	}
	return imports, nil
}

// scanImports scans one individual or a group of imports in a file.
func scanImports(s scanner.Scanner) ([]string, error) {
	var imports []string
	_, tok, lit := s.Scan()
	if tok == token.STRING {
		// Only one direct import.
		imp, err := strconv.Unquote(lit)
		if err != nil {
			return nil, err
		}
		imports = append(imports, imp)
	} else if tok == token.LPAREN {
		// Block of imports.
		for {
			_, tok, lit = s.Scan()
			if tok == token.STRING {
				imp, err := strconv.Unquote(lit)
				if err != nil {
					return nil, err
				}
				imports = append(imports, imp)
			} else if tok == token.RPAREN {
				break
			}
		}
	}
	return imports, nil
}
