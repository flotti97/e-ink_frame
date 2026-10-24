// Command library lists, adds and removes Images in an Image library.
//
//	library [-dir path] list
//	library [-dir path] add <image.jpg|image.png>...
//	library [-dir path] rm <id>...
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flotti97/e-ink_frame/library"
)

func main() {
	dir := flag.String("dir", "data", "library directory")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: library [-dir path] list | add <image>... | rm <id>...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	lib, err := library.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "library:", err)
		os.Exit(1)
	}

	cmd, args := flag.Arg(0), flag.Args()[1:]
	failed := false
	switch cmd {
	case "list":
		entries, err := lib.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "library:", err)
			os.Exit(1)
		}
		for _, e := range entries {
			fmt.Printf("%s  %s  %s\n", e.ID, e.Added.Local().Format("2006-01-02 15:04"), e.Name)
		}
	case "add":
		for _, path := range args {
			if err := add(lib, path); err != nil {
				fmt.Fprintln(os.Stderr, "library:", err)
				failed = true
			}
		}
	case "rm":
		for _, id := range args {
			if err := lib.Delete(id); err != nil {
				fmt.Fprintln(os.Stderr, "library:", err)
				failed = true
			}
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
	if failed {
		os.Exit(1)
	}
}

func add(lib *library.Library, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	e, err := lib.Add(filepath.Base(path), f)
	if err != nil {
		return err
	}
	fmt.Printf("%s  %s\n", e.ID, e.Name)
	return nil
}
