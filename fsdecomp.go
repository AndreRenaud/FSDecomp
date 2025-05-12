package fsdecomp

import (
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// Make sure DecompressFS implements fs.FS
var _ fs.FS = (*DecompressFS)(nil)
var _ fs.ReadDirFS = (*DecompressFS)(nil)

// DecompressFS wraps an io.FS and automatically decompresses files with known extensions
type DecompressFS struct {
	fs.FS
}

// New creates a new DecompressFS that wraps the provided filesystem
func New(fsys fs.FS) *DecompressFS {
	return &DecompressFS{FS: fsys}
}

// Open implements fs.FS.Open
func (dfs *DecompressFS) Open(name string) (fs.File, error) {
	// First try to open the file directly
	file, err := dfs.FS.Open(name)
	if err == nil {
		return file, nil
	}

	// If not found, try with compression extensions
	if errors.Is(err, fs.ErrNotExist) {
		// Try .gz
		gzFile, gzErr := dfs.FS.Open(name + ".gz")
		if gzErr == nil {
			return newGzipFile(gzFile)
		}

		// Try .bz2
		bz2File, bz2Err := dfs.FS.Open(name + ".bz2")
		if bz2Err == nil {
			return newBzip2File(bz2File)
		}

		// Try .zst
		zstFile, zstErr := dfs.FS.Open(name + ".zst")
		if zstErr == nil {
			return newZstdFile(zstFile)
		}

		// Try .lz4
		lz4File, lz4Err := dfs.FS.Open(name + ".lz4")
		if lz4Err == nil {
			return newLz4File(lz4File)
		}
	}

	// Original error if all attempts fail
	return nil, err
}

func (dfs *DecompressFS) ReadDir(name string) ([]fs.DirEntry, error) {
	// Custom implementation that filters/modifies directory entries
	entries, err := fs.ReadDir(dfs.FS, name)
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".bz2") || strings.HasSuffix(name, ".zst") || strings.HasSuffix(name, ".lz4") {
			// Modify the name to remove the compression extension
			newName := strings.TrimSuffix(name, ".gz")
			newName = strings.TrimSuffix(newName, ".bz2")
			newName = strings.TrimSuffix(newName, ".zst")
			newName = strings.TrimSuffix(newName, ".lz4")
			entries[i] = &fileInfoWrapper{
				FileInfo: info,
				name:     newName,
			}
		}
	}
	return entries, nil
}

// decompressFile implements fs.File for a decompressed reader
type decompressFile struct {
	reader     io.Reader
	closer     io.Closer
	info       fs.FileInfo
	originalFS fs.File
}

func (df *decompressFile) Stat() (fs.FileInfo, error) {
	if df.info != nil {
		return df.info, nil
	}
	return df.originalFS.Stat()
}

func (df *decompressFile) Read(p []byte) (int, error) {
	return df.reader.Read(p)
}

func (df *decompressFile) Close() error {
	return df.closer.Close()
}

// newGzipFile creates a decompressed file reader for gzip files
func newGzipFile(f fs.File) (fs.File, error) {
	gzReader, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, err
	}

	// Get the original file info
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	// Create custom FileInfo with the original name without the extension
	modifiedInfo := modifyFileInfo(info, strings.TrimSuffix(info.Name(), ".gz"))

	return &decompressFile{
		reader:     gzReader,
		closer:     multiCloser{gzReader, f},
		info:       modifiedInfo,
		originalFS: f,
	}, nil
}

// newBzip2File creates a decompressed file reader for bzip2 files
func newBzip2File(f fs.File) (fs.File, error) {
	bzReader := bzip2.NewReader(f)

	// Get the original file info
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	// Create custom FileInfo with the original name without the extension
	modifiedInfo := modifyFileInfo(info, strings.TrimSuffix(info.Name(), ".bz2"))

	return &decompressFile{
		reader:     bzReader,
		closer:     f,
		info:       modifiedInfo,
		originalFS: f,
	}, nil
}

// newZstdFile creates a decompressed file reader for zstd files
func newZstdFile(f fs.File) (fs.File, error) {
	zstReader, err := zstd.NewReader(f)
	if err != nil {
		f.Close()
		return nil, err
	}

	// Get the original file info
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	// Create custom FileInfo with the original name without the extension
	modifiedInfo := modifyFileInfo(info, strings.TrimSuffix(info.Name(), ".zst"))

	return &decompressFile{
		reader:     zstReader,
		closer:     f,
		info:       modifiedInfo,
		originalFS: f,
	}, nil
}

// newLz4File creates a decompressed file reader for lz4 files
func newLz4File(f fs.File) (fs.File, error) {
	lz4Reader := lz4.NewReader(f)

	// Get the original file info
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	// Create custom FileInfo with the original name without the extension
	modifiedInfo := modifyFileInfo(info, strings.TrimSuffix(info.Name(), ".lz4"))

	return &decompressFile{
		reader:     lz4Reader,
		closer:     f, // LZ4 reader doesn't need to be closed
		info:       modifiedInfo,
		originalFS: f,
	}, nil
}

// multiCloser helps close multiple resources
type multiCloser struct {
	c1, c2 io.Closer
}

func (mc multiCloser) Close() error {
	err1 := mc.c1.Close()
	err2 := mc.c2.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// fileInfoWrapper wraps an fs.FileInfo to modify its name
type fileInfoWrapper struct {
	fs.FileInfo
	name string
}

func (fiw fileInfoWrapper) Name() string {
	return fiw.name
}

func (fiw fileInfoWrapper) Info() (fs.FileInfo, error) {
	return fiw.FileInfo, nil
}

func (fiw fileInfoWrapper) Type() fs.FileMode {
	return fiw.FileInfo.Mode()
}

func (fiw fileInfoWrapper) ModTime() time.Time {
	return fiw.FileInfo.ModTime()
}

func modifyFileInfo(info fs.FileInfo, newName string) fs.FileInfo {
	return fileInfoWrapper{
		FileInfo: info,
		name:     newName,
	}
}
