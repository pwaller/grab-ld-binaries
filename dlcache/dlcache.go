package dlcache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const cacheMagic = "ld.so-1.7.0\x00"

// Load returns a *DLCache loaded from /etc/ld.so.cache.
func Load() (*DLCache, error) {

	fd, err := os.Open("/etc/ld.so.cache")
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	dc, err := ReadDLCache(fd)
	if err != nil {
		return nil, err
	}

	return dc, nil
}

// DLCache represents the contents of ld.so.cache.
type DLCache struct {
	FileEntries []fileEntry
}

// Lookup bisects the DLCache searching for library.
func (dc *DLCache) Lookup(library string) (string, bool) {
	if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
		paths := strings.Split(ldPath, ":")
		for _, path := range paths {
			maybePath := filepath.Join(path, library)
			if _, err := os.Stat(maybePath); err == nil {
				return maybePath, true
			}
		}
	}

	lo, hi := 0, len(dc.FileEntries)
	for lo < hi {
		mid := (lo + hi) / 2
		key := dc.FileEntries[mid].Key

		x := _dl_cache_libcmp(key, library)
		// log.Printf("lo, mid, hi, key = %d, %d, %d, %s, %d", lo, mid, hi, key, x)

		switch x {
		case 0:
			// case key == library:
			if dc.FileEntries[mid].Is64() {
				return dc.FileEntries[mid].Value, true
			}
			// Ignore wrong platform.
			hi = mid
		case -1:
			// case key < library:
			hi = mid
		case 1:
			// case key > library:
			lo = mid + 1
		}
	}

	for i, entry := range dc.FileEntries {
		if entry.Key == library {
			log.Printf("Found the slow way at %d: %q", i, entry.Key)

			for j := i - 10; j < i+10; j++ {
				log.Printf("  %d, %q", j, dc.FileEntries[j].Key)
			}
			return entry.Value, true
		}
	}

	return "", false
}

type _fileEntry struct {
	Flags      int32
	Key, Value uint32
}

// FileEntry represents a
type fileEntry struct {
	Flags      int
	Key, Value string
}

func (fe fileEntry) String() string {
	return fmt.Sprintf(
		"fileEntry{%x, %q, %q, is64=%t}",
		fe.Flags, fe.Key, fe.Value, fe.Is64(),
	)
}

func (fe fileEntry) Is64() bool {
	return (fe.Flags & 0x300) == 0x300
}

// ReadDLCache loads a DL Cache from r
func ReadDLCache(r io.Reader) (*DLCache, error) {

	magic := [len(cacheMagic)]byte{}

	var err error
	read := func(ptr interface{}) error {
		err = binary.Read(r, binary.LittleEndian, ptr)
		return err
	}

	read(&magic)
	if string(magic[:5]) != "ld.so" {
		return nil, fmt.Errorf("Magic does not start with ld.so.")
	}

	var nlibs uint32
	if err := read(&nlibs); err != nil {
		return nil, err
	}

	fileEntries := make([]_fileEntry, nlibs)
	for i := range fileEntries {
		if err := read(&fileEntries[i]); err != nil {
			return nil, err
		}
	}

	stringTable, err := ioutil.ReadAll(r)
	if err != nil {
		log.Fatal(err)
	}

	readString := func(index int) string {
		l := bytes.IndexByte(stringTable[index:], 0)
		return string(stringTable[index : index+l])
	}

	dlCache := &DLCache{}

	for _, entry := range fileEntries {
		dlCache.FileEntries = append(dlCache.FileEntries, fileEntry{
			Flags: int(entry.Flags),
			Key:   readString(int(entry.Key)),
			Value: readString(int(entry.Value)),
		})
	}
	//
	// for i, e := range dlCache.FileEntries {
	// 	log.Printf("%s", e)
	// 	if i > 100 {
	// 		break
	// 	}
	// }

	return dlCache, nil
}

func _dl_cache_libcmp(p1, p2 string) int {
	// log.Printf("Compare %q and %q", p1, p2)
	l := len(p1)
	if len(p2) < l {
		l = len(p2)
	}
	parseLeadingNum := func(s string) int {
		nonDigit := func(r rune) bool { return !unicode.IsDigit(r) }
		firstNonDigit := strings.IndexFunc(s, nonDigit)
		if firstNonDigit == -1 {
			// No numbers before end of string
			firstNonDigit = len(s)
		}
		n, err := strconv.ParseInt(s[:firstNonDigit], 10, 32)
		if err != nil {
			panic(err)
		}
		return int(n)
	}

	for i := 0; i < l; i++ {
		p1c, p2c := p1[i], p2[i]
		switch {
		case unicode.IsDigit(rune(p1c)) && unicode.IsDigit(rune(p2c)):
			// Must do a numerical compare.
			n1 := parseLeadingNum(p1[i:])
			n2 := parseLeadingNum(p2[i:])
			switch {
			case n1 < n2:
				return -1
			case n1 > n2:
				return 1
			}
		case unicode.IsDigit(rune(p1c)):
			return 1
		case unicode.IsDigit(rune(p2c)):
			return -1
		case p1c < p2c:
			return -1
		case p1c > p2c:
			return 1
		}
	}
	switch {
	case len(p1) < len(p2):
		return -1
	case len(p1) > len(p2):
		return 1
	}
	return 0
}

// _dl_cache_libcmp (const char *p1, const char *p2)
// {
//   while (*p1 != '\0')
//     {
//       if (*p1 >= '0' && *p1 <= '9')
//         {
//           if (*p2 >= '0' && *p2 <= '9')
//             {
// 	      /* Must compare this numerically.  */
// 	      int val1;
// 	      int val2;
//
// 	      val1 = *p1++ - '0';
// 	      val2 = *p2++ - '0';
// 	      while (*p1 >= '0' && *p1 <= '9')
// 	        val1 = val1 * 10 + *p1++ - '0';
// 	      while (*p2 >= '0' && *p2 <= '9')
// 	        val2 = val2 * 10 + *p2++ - '0';
// 	      if (val1 != val2)
// 		return val1 - val2;
// 	    }
// 	  else
//             return 1;
//         }
//       else if (*p2 >= '0' && *p2 <= '9')
//         return -1;
//       else if (*p1 != *p2)
//         return *p1 - *p2;
//       else
// 	{
// 	  ++p1;
// 	  ++p2;
// 	}
//     }
//   return *p1 - *p2;
// }
