package main

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

func TestPortableDeterministicArchive(t *testing.T) {
	files := []entry{{Name: "council", Data: []byte("native fixture"), Mode: 0755}, {Name: "third-party/LICENSE", Data: []byte("notice"), Mode: 0644}}
	var first, second bytes.Buffer
	if err := writeArchive(&first, files); err != nil {
		t.Fatal(err)
	}
	if err := writeArchive(&second, files); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("identical input must produce identical archive bytes")
	}
	archive, err := zip.NewReader(bytes.NewReader(first.Bytes()), int64(first.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for i, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !bytes.Equal(data, files[i].Data) || uint32(file.Mode().Perm()) != files[i].Mode {
			t.Fatal("archive did not preserve file data or portable executable mode")
		}
	}
}

func TestRejectUnsafeArchiveEntries(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../private", "/private", "a/../../private", "a\\private", "C:/private", "a/", "a\x00b"} {
		if err := writeArchive(io.Discard, []entry{{Name: name, Mode: 0644}}); err == nil {
			t.Fatalf("accepted unsafe archive name %q", name)
		}
	}
	if err := writeArchive(io.Discard, []entry{{Name: "same", Mode: 0644}, {Name: "same", Mode: 0644}}); err == nil {
		t.Fatal("accepted duplicate archive names")
	}
	if err := writeArchive(io.Discard, []entry{{Name: "link", Mode: 0120777}}); err == nil {
		t.Fatal("accepted unsupported file mode")
	}
}
