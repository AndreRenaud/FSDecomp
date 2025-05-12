package fsdecomp

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4"
)

// TestDecompressFS tests the functionality of DecompressFS
func TestDecompressFS(t *testing.T) {
	// Create test data
	normalContent := "This is a normal file"
	gzipContent := "This is gzipped content"
	bzip2Content := "This is bzip2 content"
	zstdContent := "This is zstd content"
	lz4Content := "This is lz4 content"

	// Create a test MapFS with various file types
	testFS := fstest.MapFS{
		"normal.txt": &fstest.MapFile{
			Data: []byte(normalContent),
		},
		"compressed.txt.gz": &fstest.MapFile{
			Data: createGzipData(t, gzipContent),
		},
		"archive.txt.bz2": &fstest.MapFile{
			Data: createBzip2Data(t, bzip2Content),
		},
		"file-zstd.txt.zst": &fstest.MapFile{
			Data: createZstdData(t, zstdContent),
		},
		"file-lz4.txt.lz4": &fstest.MapFile{
			Data: createLz4Data(t, lz4Content),
		},
	}

	// Create the DecompressFS wrapper
	dfs := DecompressFS{testFS}

	// Test cases
	tests := []struct {
		name          string
		path          string
		expectedData  string
		expectedError bool
	}{
		{
			name:         "Regular file",
			path:         "normal.txt",
			expectedData: normalContent,
		},
		{
			name:         "Gzipped file - transparent access",
			path:         "compressed.txt",
			expectedData: gzipContent,
		},
		{
			name:         "Bzip2 file - transparent access",
			path:         "archive.txt",
			expectedData: bzip2Content,
		},
		{
			name:         "Zstd file - transparent access",
			path:         "file-zstd.txt",
			expectedData: zstdContent,
		},
		{
			name:         "Lz4 file - transparent access",
			path:         "file-lz4.txt",
			expectedData: lz4Content,
		},
		{
			name:          "Non-existent file",
			path:          "doesnotexist.txt",
			expectedError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, err := dfs.Open(tc.path)
			if tc.expectedError {
				if err == nil {
					t.Fatalf("Expected error opening %s, got nil", tc.path)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error opening %s: %v", tc.path, err)
			}
			defer file.Close()

			// Check file content
			data, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("Error reading file: %v", err)
			}

			if string(data) != tc.expectedData {
				t.Errorf("Expected content %q, got %q", tc.expectedData, string(data))
			}

			// Check file info
			info, err := file.Stat()
			if err != nil {
				t.Fatalf("Error getting file info: %v", err)
			}

			// Check that the extension was removed for compressed files
			if strings.HasSuffix(tc.path, ".gz") || strings.HasSuffix(tc.path, ".bz2") {
				if strings.HasSuffix(info.Name(), ".gz") || strings.HasSuffix(info.Name(), ".bz2") {
					t.Errorf("File name %s should not have compression extension", info.Name())
				}
			}
		})
	}
}

// Helper to create gzip test data
func createGzipData(t *testing.T, content string) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, err := gzw.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write gzip data: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}
	return buf.Bytes()
}

// Helper to create bzip2 test data
func createBzip2Data(t *testing.T, content string) []byte {
	var buf bytes.Buffer
	bw, err := bzip2.NewWriter(&buf, nil)
	if err != nil {
		t.Fatalf("Failed to create bzip2 writer: %v", err)
	}
	if _, err := bw.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write bzip2 data: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("Failed to close bzip2 writer: %v", err)
	}
	return buf.Bytes()
}

// Helper to create zstd test data
func createZstdData(t *testing.T, content string) []byte {
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("Failed to create zstd writer: %v", err)
	}
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write zstd data: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close zstd writer: %v", err)
	}
	return buf.Bytes()
}

// Helper to create lz4 test data
func createLz4Data(t *testing.T, content string) []byte {
	var buf bytes.Buffer
	lw := lz4.NewWriter(&buf)
	if _, err := lw.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write lz4 data: %v", err)
	}
	if err := lw.Close(); err != nil {
		t.Fatalf("Failed to close lz4 writer: %v", err)
	}
	return buf.Bytes()
}

// TestReadDirectoryFails ensures that directory-related operations work properly
func TestReadDirectoryFails(t *testing.T) {
	testFS := fstest.MapFS{
		"dir/file.txt": &fstest.MapFile{
			Data: []byte("content"),
		},
	}

	dfs := DecompressFS{testFS}

	// Try to open a directory
	_, err := dfs.Open("dir")
	if err != nil {
		// This should pass through the underlying fs's behavior for directories
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Unexpected error when opening directory: %v", err)
		}
	}
}
