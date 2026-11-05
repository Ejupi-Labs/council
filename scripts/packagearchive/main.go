// Command packagearchive writes an explicitly supplied file list to a portable ZIP.
// The development wrapper supplies file contents, so this helper never walks the tree.
package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

type entry struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
	Mode uint32 `json:"mode"`
}

func writeArchive(destination io.Writer, entries []entry) error {
	seen := map[string]bool{}
	for _, item := range entries {
		if (item.Name == "." || item.Name == "..") || path.Clean(item.Name) != item.Name || path.IsAbs(item.Name) ||
			strings.ContainsAny(item.Name, "\\:\x00") || strings.HasPrefix(item.Name, "../") ||
			strings.HasSuffix(item.Name, "/") || seen[item.Name] || (item.Mode != 0644 && item.Mode != 0755) {
			return errors.New("invalid or duplicate archive entry")
		}
		seen[item.Name] = true
	}
	if len(entries) == 0 {
		return errors.New("archive must contain files")
	}
	writer := zip.NewWriter(destination)
	for _, item := range entries {
		header := &zip.FileHeader{Name: item.Name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		header.SetMode(os.FileMode(item.Mode))
		file, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := file.Write(item.Data); err != nil {
			return err
		}
	}
	return writer.Close()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to create native preview archive:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return errors.New("expected one output filename")
	}
	var entries []entry
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected one entry list")
	}
	output, err := os.OpenFile(os.Args[1], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = writeArchive(output, entries)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}
