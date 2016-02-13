package main

import (
	"archive/tar"
	"debug/elf"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sort"

	"github.com/mattn/go-isatty"
	"github.com/pwaller/grab-ld-binaries/dlcache"
)

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		log.Fatal("usage: grab-binaries <filename>")
	}

	filename := args[0]

	dc, err := dlcache.Load()
	if err != nil {
		log.Fatalf("Failed to load ld.so.cache: %v", err)
	}

	filename, err = resolveBinary(dc, filename)
	if err != nil {
		log.Fatalf("resolveBinary %q: %v", filename, err)
	}

	imports, err := recursiveImports(dc, filename)
	if err != nil {
		log.Fatal(err)
	}

	paths := []string{filename}
	for _, lib := range sortedSet(imports) {
		path, ok := dc.Lookup(lib)
		if ok {
			log.Println(lib, "=>", path)
			paths = append(paths, path)
		} else {
			log.Println(lib, "(not found)")
		}
	}

	out := io.Writer(os.Stdout)
	if isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(os.Stderr)
		log.Printf("Not writing tar file to terminal.")
		log.Printf("Use `| cat` if you really want it.")
		fmt.Fprintln(os.Stderr)
		out = ioutil.Discard
	}

	total := writeTar(out, paths)
	log.Printf("Total: %.2f MiB", float64(total)/1024/1024)
}

// writeTar writes `paths` to `out` as a tar stream and returns the total bytes
// read from disk.
func writeTar(out io.Writer, paths []string) int64 {
	tf := tar.NewWriter(out)
	defer tf.Close()

	var total int64
	for _, path := range paths {
		fi, err := os.Stat(path)
		if err != nil {
			log.Fatal(err)
		}

		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			log.Fatal(err)
		}

		err = tf.WriteHeader(hdr)
		if err != nil {
			log.Fatal(err)
		}

		func() {
			fd, err := os.Open(path)
			if err != nil {
				log.Fatal(err)
			}
			defer fd.Close()

			n, err := io.Copy(tf, fd)
			if err != nil {
				log.Fatal(err)
			}
			total += n
		}()
	}
	return total
}

// sortedSet takes a string set and returns it as a sorted slice.
func sortedSet(set map[string]struct{}) []string {
	var out []string
	for element := range set {
		out = append(out, element)
	}
	sort.Strings(out)
	return out
}

// recursiveImports returns the set of all imports for a given filename.
func recursiveImports(
	dc *dlcache.DLCache, filename string,
) (
	map[string]struct{}, error,
) {
	seen := map[string]struct{}{}
	imports := map[string]struct{}{}

	var depth int

	var visit func(filename string) error
	visit = func(filename string) error {

		// log.Println(strings.Repeat(" ", depth), "import", filename)

		if _, ok := seen[filename]; ok {
			// Stop, since it has been seen before
			return nil
		}
		seen[filename] = struct{}{}

		importedLibs, err := readImports(dc, filename)
		if err != nil {
			return err
		}

		for _, dep := range importedLibs {

			imports[dep] = struct{}{}
			depth++
			err := visit(dep)
			depth--
			if err != nil {
				return err
			}
		}
		return nil
	}

	return imports, visit(filename)
}

// readImports returns the imports of one ELF file, with a lookup into the
// ld.so.cache if needed.
func readImports(dc *dlcache.DLCache, filename string) ([]string, error) {

	fd, err := os.Open(filename)
	if os.IsNotExist(err) {
		// Lookup the path from the dl cache.
		filename, ok := dc.Lookup(filename)
		if ok {
			fd, err = os.Open(filename)
		}
	}
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	elf, err := elf.NewFile(fd)
	if err != nil {
		return nil, err
	}
	defer elf.Close()

	importedLibs, err := elf.ImportedLibraries()
	if err != nil {
		return nil, err
	}
	return importedLibs, nil
}

// resolveBinary looks up "filename" in the $PATH and in the ld.so.cache.
func resolveBinary(dc *dlcache.DLCache, filename string) (string, error) {
	var (
		err error
		fn  string
	)

	if _, err = os.Stat(filename); os.IsNotExist(err) {
		// Try looking for executables in $PATH.
		if fn, err = exec.LookPath(filename); err == nil {
			filename = fn
		} else if err != nil {
			// Try looking in the ld.so.cache.
			if fn, ok := dc.Lookup(filename); ok {
				log.Printf("Resolved %q to %q", filename, fn)
				filename = fn
			} else {
				err = fmt.Errorf("Unable to locate %q", filename)
			}
		}
	}
	return filename, err
}
